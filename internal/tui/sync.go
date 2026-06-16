package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/embed"
	"github.com/hackastak/repog/internal/sync"
)

// syncView is the Sync/Embed tab: it triggers the GitHub→DB ingest and the
// embedding pipeline and renders their progress live. Both pipelines already
// expose buffered event channels (sync.IngestRepos, embed.RunEmbedPipeline);
// this view drains them one event at a time via a re-issued wait command — the
// same streaming pattern the Ask view uses for tokens. It is a thin presentation
// layer over internal/sync and internal/embed: no business logic is duplicated,
// and failures surface as UI state rather than os.Exit (unlike commands/*.go).
//
// A run keeps progressing even if the user switches tabs: the pipeline events
// implement routedMsg (see view.go), so the root model delivers them here to
// re-issue the wait command regardless of which tab is active.
type syncView struct {
	app *appContext

	spinner  spinner.Model
	viewport viewport.Model

	phase   syncPhase
	running bool
	chain   bool // when true, automatically embed after a sync completes (the "a" flow)
	hasRun  bool // at least one operation has finished this session

	// Active typed event channels; nil when no operation of that kind is in
	// flight. gen invalidates stale events from a superseded run.
	syncCh  <-chan sync.IngestEvent
	embedCh <-chan embed.EmbedEvent
	gen     int

	// Live counters mirrored into the status line.
	syncSynced, syncSkipped, syncErrors        int
	embedded, embSkipped, embErrored, embTotal int
	embBatch, embBatchTotal                    int

	lines       []string // accumulated progress log
	lastContent string   // cache so we only SetContent on change
	err         error    // last fatal precondition error (not configured, provider, etc.)
}

// syncPhase tracks which pipeline, if any, is currently running.
type syncPhase int

const (
	phaseIdle syncPhase = iota
	phaseSyncing
	phaseEmbedding
)

// maxLogLines bounds the in-memory log so a large sync (hundreds of repos)
// can't grow it without limit; only the most recent lines are retained.
const maxLogLines = 500

func newSyncView(app *appContext) *syncView {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = helpStyle
	return &syncView{app: app, spinner: sp}
}

// Init is a no-op: the view starts idle and only does work once the user
// triggers an operation, so re-entering the tab must not disturb a run.
func (v *syncView) Init() tea.Cmd { return nil }

func (v *syncView) Update(msg tea.Msg) (view, tea.Cmd) {
	switch msg := msg.(type) {
	case syncStartedMsg:
		if msg.gen != v.gen {
			return v, nil
		}
		v.syncCh = msg.ch
		return v, waitForSyncEvent(v.syncCh, v.gen)

	case embedStartedMsg:
		if msg.gen != v.gen {
			return v, nil
		}
		v.embedCh = msg.ch
		return v, waitForEmbedEvent(v.embedCh, v.gen)

	case syncEventMsg:
		if msg.gen != v.gen {
			return v, nil
		}
		return v.handleSyncEvent(msg)

	case embedEventMsg:
		if msg.gen != v.gen {
			return v, nil
		}
		return v.handleEmbedEvent(msg)

	case opErrMsg:
		if msg.gen != v.gen {
			return v, nil
		}
		v.running = false
		v.phase = phaseIdle
		v.chain = false
		v.err = msg.err
		v.appendLine(errStyle.Render("✗ " + msg.err.Error()))
		return v, nil

	case spinner.TickMsg:
		if !v.running {
			return v, nil
		}
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		return v, cmd

	case tea.KeyMsg:
		// While an operation runs, the trigger keys are inert; other keys still
		// scroll the log so the user can review progress.
		if !v.running {
			switch msg.String() {
			case "s":
				return v, v.beginSync(false)
			case "e":
				return v, v.beginEmbed(true)
			case "a":
				return v, v.beginSync(true)
			}
		}
		var cmd tea.Cmd
		v.viewport, cmd = v.viewport.Update(msg)
		return v, cmd
	}

	return v, nil
}

func (v *syncView) View(width, height int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Sync / Embed"))
	b.WriteString("\n\n")
	b.WriteString(v.statusLine())
	b.WriteString("\n\n")

	// Title + status take four rows (each with a trailing blank); the rest is
	// the scrollable log. A running op follows the bottom; an idle one scrolls.
	v.viewport.Width = width
	v.viewport.Height = max(height-4, 1)
	content := v.renderLog()
	if content != v.lastContent {
		atBottom := v.viewport.AtBottom()
		v.viewport.SetContent(content)
		v.lastContent = content
		if v.running && atBottom {
			v.viewport.GotoBottom()
		}
	}
	b.WriteString(v.viewport.View())

	return b.String()
}

func (v *syncView) HelpKeys() string {
	if v.running {
		return "running… ↑/↓ scroll"
	}
	return "s sync · e embed · a sync+embed · ↑/↓ scroll"
}

// statusLine renders the single dynamic line above the log: a spinner with live
// counters while running, an idle prompt otherwise.
func (v *syncView) statusLine() string {
	switch {
	case v.running && v.phase == phaseSyncing:
		return v.spinner.View() + helpStyle.Render(fmt.Sprintf(
			" Syncing… %d synced · %d skipped · %d errors",
			v.syncSynced, v.syncSkipped, v.syncErrors))
	case v.running && v.phase == phaseEmbedding:
		batch := ""
		if v.embBatchTotal > 0 {
			batch = fmt.Sprintf(" batch %d/%d ·", v.embBatch, v.embBatchTotal)
		}
		return v.spinner.View() + helpStyle.Render(fmt.Sprintf(
			" Embedding…%s %d/%d chunks · %d errors",
			batch, v.embedded, v.embTotal, v.embErrored))
	case v.err != nil:
		return errStyle.Render("Error: " + v.err.Error())
	case v.hasRun:
		return okStyle.Render("Done.") + helpStyle.Render("  Press s/e/a to run again.")
	default:
		return helpStyle.Render("Press s to sync, e to embed, or a to sync then embed.")
	}
}

func (v *syncView) renderLog() string {
	if len(v.lines) == 0 {
		return helpStyle.Render("No activity yet.")
	}
	return strings.Join(v.lines, "\n")
}

// appendLine adds one log line, trimming the oldest once the cap is hit.
func (v *syncView) appendLine(s string) {
	v.lines = append(v.lines, s)
	if len(v.lines) > maxLogLines {
		v.lines = v.lines[len(v.lines)-maxLogLines:]
	}
}

// beginSync starts the ingest pipeline. chain carries the "embed afterwards"
// intent (the "a" flow) so finishSync knows whether to continue into embed.
func (v *syncView) beginSync(chain bool) tea.Cmd {
	v.gen++
	v.phase = phaseSyncing
	v.running = true
	v.chain = chain
	v.err = nil
	v.syncSynced, v.syncSkipped, v.syncErrors = 0, 0, 0
	if chain {
		v.appendLine(titleStyle.Render("▶ Sync + embed started"))
	} else {
		v.appendLine(titleStyle.Render("▶ Sync started"))
	}
	return tea.Batch(v.spinner.Tick, runSyncCmd(v.app, v.gen))
}

// beginEmbed starts the embedding pipeline. issueTick is true when embed is
// launched standalone (the "e" flow) and must start its own spinner ticker;
// when chained after a sync the existing ticker is still running, so a second
// Tick would double it.
func (v *syncView) beginEmbed(issueTick bool) tea.Cmd {
	v.gen++
	v.phase = phaseEmbedding
	v.running = true
	v.chain = false
	v.err = nil
	v.embedded, v.embSkipped, v.embErrored, v.embTotal = 0, 0, 0, 0
	v.embBatch, v.embBatchTotal = 0, 0
	v.appendLine(titleStyle.Render("▶ Embed started"))
	if issueTick {
		return tea.Batch(v.spinner.Tick, runEmbedCmd(v.app, v.gen))
	}
	return runEmbedCmd(v.app, v.gen)
}

func (v *syncView) handleSyncEvent(msg syncEventMsg) (view, tea.Cmd) {
	if msg.closed {
		// The producer closed without a terminal "done" event — finalize anyway
		// so the view never gets stuck in the running state.
		return v.finishSync()
	}
	switch ev := msg.ev; ev.Type {
	case "repo":
		v.syncSynced++
		label := "new"
		if ev.Status == "updated" {
			label = "updated"
		}
		v.appendLine(okStyle.Render("✓ ") + fmt.Sprintf("%-8s %s", label, ev.Repo))
	case "skip":
		// Skips are common (unchanged repos); track the count but keep them out
		// of the log so the interesting events stay visible.
		v.syncSkipped++
	case "error":
		v.syncErrors++
		v.appendLine(errStyle.Render("✗ ") + ev.Repo + helpStyle.Render(" ("+ev.Reason+")"))
	case "done":
		// The done event carries authoritative totals.
		v.syncSynced, v.syncSkipped, v.syncErrors = ev.Total, ev.Skipped, ev.Errors
		v.appendLine(bannerSync(ev))
		return v.finishSync()
	}
	return v, waitForSyncEvent(v.syncCh, v.gen)
}

func (v *syncView) finishSync() (view, tea.Cmd) {
	v.syncCh = nil
	if v.chain {
		// Continue into embed on the same spinner ticker.
		return v, v.beginEmbed(false)
	}
	v.running = false
	v.phase = phaseIdle
	v.hasRun = true
	return v, nil
}

func (v *syncView) handleEmbedEvent(msg embedEventMsg) (view, tea.Cmd) {
	if msg.closed {
		return v.finishEmbed()
	}
	switch ev := msg.ev; ev.Type {
	case "batch":
		v.embedded = ev.ChunksEmbedded
		v.embSkipped = ev.ChunksSkipped
		v.embErrored = ev.ChunksErrored
		v.embTotal = ev.TotalChunks
		v.embBatch = ev.BatchIndex
		v.embBatchTotal = ev.BatchTotal
		for _, e := range dedupeStrings(ev.Errors) {
			v.appendLine(errStyle.Render("  error: ") + e)
		}
	case "repo_skip":
		// Counts arrive via batch/done events; no per-repo log noise.
	case "error":
		v.embErrored++
		v.appendLine(errStyle.Render("✗ ") + ev.RepoFullName)
	case "done":
		v.embedded = ev.ChunksEmbedded
		v.embSkipped = ev.ChunksSkipped
		v.embErrored = ev.ChunksErrored
		v.embTotal = ev.TotalChunks
		v.appendLine(bannerEmbed(ev))
		return v.finishEmbed()
	}
	return v, waitForEmbedEvent(v.embedCh, v.gen)
}

func (v *syncView) finishEmbed() (view, tea.Cmd) {
	v.embedCh = nil
	v.running = false
	v.phase = phaseIdle
	v.chain = false
	v.hasRun = true
	return v, nil
}

// --- messages & commands ---------------------------------------------------

// syncStartedMsg / embedStartedMsg hand a freshly-opened pipeline channel back
// to Update, which kicks off the per-event wait loop. opErrMsg reports a
// precondition failure (no config, missing PAT, provider build error) before any
// channel exists.
type syncStartedMsg struct {
	gen int
	ch  <-chan sync.IngestEvent
}

type embedStartedMsg struct {
	gen int
	ch  <-chan embed.EmbedEvent
}

type opErrMsg struct {
	gen int
	err error
}

// syncEventMsg / embedEventMsg carry one pipeline event (or a closed signal) to
// Update. gen guards against events from a superseded run.
type syncEventMsg struct {
	gen    int
	ev     sync.IngestEvent
	closed bool
}

type embedEventMsg struct {
	gen    int
	ev     embed.EmbedEvent
	closed bool
}

// targetTab routes every Sync/Embed pipeline message to this view so a run that
// the user started keeps streaming after they switch tabs (see routedMsg).
func (syncStartedMsg) targetTab() tab  { return tabSync }
func (embedStartedMsg) targetTab() tab { return tabSync }
func (syncEventMsg) targetTab() tab    { return tabSync }
func (embedEventMsg) targetTab() tab   { return tabSync }
func (opErrMsg) targetTab() tab        { return tabSync }

// waitForSyncEvent blocks on the ingest channel and returns the next event.
// Each event re-issues this command from Update, draining the channel in order;
// a closed channel yields a final closed=true message.
func waitForSyncEvent(ch <-chan sync.IngestEvent, gen int) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return syncEventMsg{gen: gen, closed: true}
		}
		return syncEventMsg{gen: gen, ev: ev}
	}
}

func waitForEmbedEvent(ch <-chan embed.EmbedEvent, gen int) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return embedEventMsg{gen: gen, closed: true}
		}
		return embedEventMsg{gen: gen, ev: ev}
	}
}

// runSyncCmd validates preconditions on the command goroutine, then opens the
// ingest pipeline and hands its channel back. It mirrors commands/sync.go's
// defaults (owned + starred, chunk size derived from the embedding model) so the
// TUI and CLI produce identical data.
func runSyncCmd(app *appContext, gen int) tea.Cmd {
	return func() tea.Msg {
		database := app.db()
		if database == nil {
			return opErrMsg{gen: gen, err: fmt.Errorf("not configured — open Settings to finish setup")}
		}
		pat, err := config.GetGitHubPAT()
		if err != nil {
			return opErrMsg{gen: gen, err: fmt.Errorf("GitHub token unavailable: %w", err)}
		}
		// The embedding provider only supplies the token budget for chunk sizing
		// here; building it now also surfaces a misconfiguration before any
		// network calls.
		embedProvider, err := app.embedProvider()
		if err != nil {
			return opErrMsg{gen: gen, err: err}
		}
		maxChunkSize := sync.CalculateMaxCharsFromTokens(embedProvider.MaxTokens())

		ch := sync.IngestRepos(context.Background(), sync.IngestOptions{
			IncludeOwned:   true,
			IncludeStarred: true,
			MaxChunkSize:   maxChunkSize,
			DB:             database,
			GitHubPAT:      pat,
		})
		return syncStartedMsg{gen: gen, ch: ch}
	}
}

// runEmbedCmd validates preconditions, then opens the embedding pipeline and
// hands its channel back. Defaults mirror commands/embed.go (file-tree chunks
// included; the provider's batch size).
func runEmbedCmd(app *appContext, gen int) tea.Cmd {
	return func() tea.Msg {
		database := app.db()
		if database == nil {
			return opErrMsg{gen: gen, err: fmt.Errorf("not configured — open Settings to finish setup")}
		}
		embedProvider, err := app.embedProvider()
		if err != nil {
			return opErrMsg{gen: gen, err: err}
		}
		ch := embed.RunEmbedPipeline(context.Background(), embed.EmbedOptions{
			IncludeFileTree:   true,
			DB:                database,
			EmbeddingProvider: embedProvider,
		})
		return embedStartedMsg{gen: gen, ch: ch}
	}
}

// bannerSync / bannerEmbed render the terminal summary line, tinted red when the
// run reported errors (matching the CLI's completion message).
func bannerSync(ev sync.IngestEvent) string {
	style := okStyle
	if ev.Errors > 0 {
		style = errStyle
	}
	return style.Render(fmt.Sprintf("✓ Sync complete — %d synced, %d skipped, %d errors",
		ev.Total, ev.Skipped, ev.Errors))
}

func bannerEmbed(ev embed.EmbedEvent) string {
	style := okStyle
	if ev.ChunksErrored > 0 {
		style = errStyle
	}
	return style.Render(fmt.Sprintf("✓ Embedding complete — %d embedded, %d skipped, %d errors",
		ev.ChunksEmbedded, ev.ChunksSkipped, ev.ChunksErrored))
}

// dedupeStrings preserves order while dropping repeats — embed batches often
// report the same provider error for every chunk in the batch.
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
