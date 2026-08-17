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

	// Suspend/resume state (ADR-011). paused means a run was halted with "p" and
	// can be continued with "r"; pausedPhase/pausedChain remember what to resume,
	// and resuming (set on "r", consumed by the next begin*) tells begin* to carry
	// counters forward instead of zeroing them.
	paused      bool
	pausedPhase syncPhase
	pausedChain bool
	resuming    bool

	// Active typed event channels; nil when no operation of that kind is in
	// flight. gen invalidates stale events from a superseded run.
	syncCh  <-chan sync.IngestEvent
	embedCh <-chan embed.EmbedEvent
	gen     int

	// Live counters mirrored into the status line.
	syncSynced, syncSkipped, syncErrors        int
	embedded, embSkipped, embErrored, embTotal int
	embBatch, embBatchTotal                    int

	// Carried progress across a suspend (ADR-011), so resume shows a continuous
	// count rather than restarting from zero. repoOutcome remembers each repo's
	// last tallied outcome so a resumed sync's re-emitted events don't double-count
	// — and so a repo that errored before the suspend and succeeds on resume moves
	// from the error tally to the synced one instead of being stuck as an error.
	// For embed the resumed pipeline reports only un-embedded chunks, so a batch
	// baseline is added to the run's cumulative counts; embCarrying marks a run
	// that is continuing a suspended embed.
	repoOutcome                                                map[string]syncOutcome
	embCarrying                                                bool
	baseEmbedded, baseEmbErrored, baseEmbSkipped, baseEmbTotal int

	lines       []string // accumulated progress log
	lastContent string   // cache so we only SetContent on change
	err         error    // last fatal precondition error (not configured, provider, etc.)

	// streamCtx/cancel drive the in-flight run. cancel tears down the sync/embed
	// producer goroutine — aborting its HTTP requests and open SQLite write — when
	// the app quits mid-run; nil when idle. See cancelStream.
	streamCtx context.Context
	cancel    context.CancelFunc
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

// releaseStream cancels the in-flight sync/embed run (if any) and clears its
// context, so the producer goroutine unwinds and its HTTP requests and open
// transaction are torn down. A harmless no-op reset once the run has finished.
func (v *syncView) releaseStream() {
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
	v.streamCtx = nil
}

// cancelStream implements streamCanceler; the root model calls it on quit (and
// when the views are rebuilt) so a running sync or embed doesn't strand its
// producer goroutine, HTTP requests, and SQLite write when the program exits.
// Quit is therefore a hard cancel: nothing about a suspended run is persisted,
// so the next launch starts fresh (ADR-011).
func (v *syncView) cancelStream() { v.releaseStream() }

// suspend halts the running op but keeps a resume point (ADR-011). It cancels
// the context (tearing the producer down exactly as quit does), remembers what
// to resume, snapshots the embed baseline so resume can continue the count, and
// bumps gen so any event already in flight from the stopped run is discarded.
func (v *syncView) suspend() {
	v.pausedPhase = v.phase
	v.pausedChain = v.chain
	if v.phase == phaseEmbedding {
		// The resumed pipeline re-reports only un-embedded chunks, so carry the
		// current cumulative totals as the baseline resume adds onto.
		v.baseEmbedded = v.embedded
		v.baseEmbErrored = v.embErrored
		v.baseEmbSkipped = v.embSkipped
		v.baseEmbTotal = v.embTotal
	}
	v.releaseStream()
	v.syncCh = nil
	v.embedCh = nil
	v.gen++
	v.running = false
	v.paused = true
	v.appendLine(warnStyle.Render("⏸ Paused"))
}

// resume continues a suspended op. It sets resuming so the next begin* carries
// counters forward rather than zeroing them, then re-issues the pipeline the run
// was in (a chained sync carries its embed-afterwards intent).
func (v *syncView) resume() tea.Cmd {
	v.resuming = true
	v.paused = false
	v.appendLine(titleStyle.Render("▶ Resumed"))
	if v.pausedPhase == phaseEmbedding {
		return v.beginEmbed(true)
	}
	return v.beginSync(v.pausedChain)
}

// cancelOp stops a running op (or drops a suspended one) and returns to idle,
// abandoning any resume point. Work already committed to the DB stays — cancel
// never rolls back embeddings or synced repos already written (ADR-011).
func (v *syncView) cancelOp() {
	v.releaseStream()
	v.syncCh = nil
	v.embedCh = nil
	v.gen++
	v.running = false
	v.paused = false
	v.resuming = false
	v.embCarrying = false
	v.phase = phaseIdle
	v.chain = false
	v.appendLine(errStyle.Render("✗ Cancelled"))
}

func (v *syncView) Update(msg tea.Msg) (view, tea.Cmd) {
	switch msg := msg.(type) {
	case syncStartedMsg:
		if msg.gen != v.gen {
			return v, nil
		}
		v.syncCh = msg.ch
		return v, waitForSyncEvent(v.streamCtx, v.syncCh, v.gen)

	case embedStartedMsg:
		if msg.gen != v.gen {
			return v, nil
		}
		v.embedCh = msg.ch
		return v, waitForEmbedEvent(v.streamCtx, v.embedCh, v.gen)

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
		v.releaseStream()
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
		switch msg.String() {
		case "p":
			// Suspend a running op; keep a resume point (ADR-011).
			if v.running {
				v.suspend()
				return v, nil
			}
		case "r":
			// Resume a suspended op, carrying its counters forward.
			if v.paused {
				return v, v.resume()
			}
		case "c":
			// Cancel — stop and abandon the run (or drop a suspended one).
			if v.running || v.paused {
				v.cancelOp()
				return v, nil
			}
		case "s":
			if !v.running && !v.paused {
				return v, v.beginSync(false)
			}
		case "e":
			if !v.running && !v.paused {
				return v, v.beginEmbed(true)
			}
		case "a":
			if !v.running && !v.paused {
				return v, v.beginSync(true)
			}
		}
		// Any other key (and inert triggers) scrolls the log so the user can
		// review progress mid-run or while paused.
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
	switch {
	case v.running:
		return "p pause · c cancel · ↑/↓ scroll"
	case v.paused:
		return "r resume · c cancel · ↑/↓ scroll"
	default:
		return "s sync · e embed · a sync+embed · ↑/↓ scroll"
	}
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
	case v.paused && v.pausedPhase == phaseSyncing:
		return warnStyle.Render(fmt.Sprintf(
			"⏸ Paused — %d synced · %d skipped · %d errors",
			v.syncSynced, v.syncSkipped, v.syncErrors)) +
			helpStyle.Render("  Press r to resume or c to cancel.")
	case v.paused:
		return warnStyle.Render(fmt.Sprintf(
			"⏸ Paused — %d/%d chunks · %d errors",
			v.embedded, v.embTotal, v.embErrored)) +
			helpStyle.Render("  Press r to resume or c to cancel.")
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

// syncOutcome is the terminal result tallied for a repo during a sync, tracked
// per repo so a resumed run can reconcile re-emitted events (see recordOutcome).
type syncOutcome int

const (
	outcomeNone    syncOutcome = iota // not yet tallied
	outcomeSynced                     // counted in syncSynced
	outcomeSkipped                    // counted in syncSkipped
	outcomeError                      // counted in syncErrors
)

// recordOutcome folds a repo's terminal outcome into the running tallies and
// reports whether the caller should surface it (log a line) as a fresh event.
//
// It exists for resume: the re-run re-emits every repo, so a repo already tallied
// with the same outcome must not be counted again. Crucially it also handles a
// changed outcome — a repo that errored before the suspend and succeeds on resume
// is moved from syncErrors to syncSynced rather than being stuck as an error.
// A fresh run starts with an empty map; an empty repo name is never deduped.
func (v *syncView) recordOutcome(repo string, outcome syncOutcome) bool {
	counter := func(o syncOutcome) *int {
		switch o {
		case outcomeSynced:
			return &v.syncSynced
		case outcomeSkipped:
			return &v.syncSkipped
		case outcomeError:
			return &v.syncErrors
		default:
			return nil
		}
	}
	apply := func(prev syncOutcome) bool {
		if prev == outcome {
			return false // already tallied with this outcome
		}
		// A prior success/skip is sticky: this repo's real work was already
		// accounted for by the operation, so a resume re-emitting it (a completed
		// repo re-appears as an unchanged "skip") must not move or re-count it.
		// Only a prior error is provisional — resume retries it and reclassifies
		// once it succeeds, decrementing the stale error before the new tally.
		if prev == outcomeSynced || prev == outcomeSkipped {
			return false
		}
		if c := counter(prev); c != nil { // prev is outcomeError
			*c--
		}
		if c := counter(outcome); c != nil {
			*c++
		}
		return true
	}

	if repo == "" {
		return apply(outcomeNone) // can't dedupe an unnamed repo; always fresh
	}
	if v.repoOutcome == nil {
		v.repoOutcome = make(map[string]syncOutcome)
	}
	// Only record the outcome we actually tallied. When a sticky prior wins, the
	// map must keep reflecting the counted outcome, not the one we ignored.
	if changed := apply(v.repoOutcome[repo]); changed {
		v.repoOutcome[repo] = outcome
		return true
	}
	return false
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
	// Release any prior run's context (a no-op from a keypress, since triggers are
	// inert while running; live only on the chain hand-off from a finished sync).
	v.releaseStream()
	resuming := v.resuming
	v.resuming = false
	v.gen++
	v.phase = phaseSyncing
	v.running = true
	v.chain = chain
	v.err = nil
	// On a fresh run reset the tallies and the per-repo outcome map; on resume keep
	// both so already-tallied repos (re-emitted on the re-run) aren't double-counted.
	if !resuming {
		v.syncSynced, v.syncSkipped, v.syncErrors = 0, 0, 0
		v.repoOutcome = make(map[string]syncOutcome)
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.streamCtx = ctx
	v.cancel = cancel
	switch {
	case resuming:
		// "▶ Resumed" was already logged by resume().
	case chain:
		v.appendLine(titleStyle.Render("▶ Sync + embed started"))
	default:
		v.appendLine(titleStyle.Render("▶ Sync started"))
	}
	return tea.Batch(v.spinner.Tick, runSyncCmd(v.app, v.gen, ctx))
}

// beginEmbed starts the embedding pipeline. issueTick is true when embed is
// launched standalone (the "e" flow) and must start its own spinner ticker;
// when chained after a sync the existing ticker is still running, so a second
// Tick would double it.
func (v *syncView) beginEmbed(issueTick bool) tea.Cmd {
	// On the chain hand-off this cancels the just-finished sync's context (a
	// no-op — its producer has already closed) before opening a fresh one.
	v.releaseStream()
	resuming := v.resuming
	v.resuming = false
	v.gen++
	v.phase = phaseEmbedding
	v.running = true
	v.chain = false
	v.err = nil
	// A resume carries the pre-suspend totals (snapshotted into base* by suspend);
	// handleEmbedEvent renders base + the resumed run's cumulative counts. A fresh
	// run zeroes everything, including the baseline.
	v.embCarrying = resuming
	if !resuming {
		v.baseEmbedded, v.baseEmbErrored, v.baseEmbSkipped, v.baseEmbTotal = 0, 0, 0, 0
		v.embedded, v.embSkipped, v.embErrored, v.embTotal = 0, 0, 0, 0
	}
	v.embBatch, v.embBatchTotal = 0, 0
	ctx, cancel := context.WithCancel(context.Background())
	v.streamCtx = ctx
	v.cancel = cancel
	if !resuming {
		// "▶ Resumed" was already logged by resume().
		v.appendLine(titleStyle.Render("▶ Embed started"))
	}
	if issueTick {
		return tea.Batch(v.spinner.Tick, runEmbedCmd(v.app, v.gen, ctx))
	}
	return runEmbedCmd(v.app, v.gen, ctx)
}

func (v *syncView) handleSyncEvent(msg syncEventMsg) (view, tea.Cmd) {
	if msg.closed {
		// The producer closed without a terminal "done" event — finalize anyway
		// so the view never gets stuck in the running state.
		return v.finishSync()
	}
	switch ev := msg.ev; ev.Type {
	case "repo":
		if !v.recordOutcome(ev.Repo, outcomeSynced) {
			break // already tallied as synced (e.g. re-emitted after a resume)
		}
		label := "new"
		if ev.Status == "updated" {
			label = "updated"
		}
		v.appendLine(okStyle.Render("✓ ") + fmt.Sprintf("%-8s %s", label, ev.Repo))
	case "skip":
		// Skips are common (unchanged repos); track the count but keep them out
		// of the log so the interesting events stay visible. A resumed run re-emits
		// already-synced repos as skips — recordOutcome keeps them from double-counting.
		v.recordOutcome(ev.Repo, outcomeSkipped)
	case "error":
		if !v.recordOutcome(ev.Repo, outcomeError) {
			break
		}
		v.appendLine(errStyle.Render("✗ ") + ev.Repo + helpStyle.Render(" ("+ev.Reason+")"))
	case "done":
		// Counts are derived incrementally (and deduped across a resume), so the
		// done event's per-run totals must not overwrite them; it only finalizes.
		v.appendLine(bannerSync(v.syncSynced, v.syncSkipped, v.syncErrors))
		return v.finishSync()
	}
	return v, waitForSyncEvent(v.streamCtx, v.syncCh, v.gen)
}

func (v *syncView) finishSync() (view, tea.Cmd) {
	v.syncCh = nil
	if v.chain {
		// Continue into embed on the same spinner ticker; beginEmbed swaps in a
		// fresh context for the embed phase.
		return v, v.beginEmbed(false)
	}
	v.releaseStream()
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
		v.applyEmbedCounts(ev)
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
		v.applyEmbedCounts(ev)
		v.appendLine(bannerEmbed(v.embedded, v.embSkipped, v.embErrored))
		return v.finishEmbed()
	}
	return v, waitForEmbedEvent(v.streamCtx, v.embedCh, v.gen)
}

// applyEmbedCounts folds a batch/done event's cumulative counts into the live
// tallies. On a fresh run the event's totals are authoritative. On a resumed run
// (embCarrying) the pipeline re-reports only the un-embedded chunks, so its
// embedded/errored counts are added onto the pre-suspend baseline, and the total
// and skipped counts are held at their pre-suspend values so the scope of the run
// stays continuous rather than shrinking to just the remainder.
func (v *syncView) applyEmbedCounts(ev embed.EmbedEvent) {
	v.embedded = v.baseEmbedded + ev.ChunksEmbedded
	v.embErrored = v.baseEmbErrored + ev.ChunksErrored
	if v.embCarrying {
		v.embTotal = v.baseEmbTotal
		v.embSkipped = v.baseEmbSkipped
	} else {
		v.embTotal = ev.TotalChunks
		v.embSkipped = ev.ChunksSkipped
	}
}

func (v *syncView) finishEmbed() (view, tea.Cmd) {
	v.embedCh = nil
	v.releaseStream()
	v.running = false
	v.paused = false
	v.resuming = false
	v.embCarrying = false
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
// a closed channel yields a final closed=true message. The read races ctx.Done()
// so quitting mid-run unblocks this reader goroutine (returning a nil, no-op
// message) instead of parking on a channel the cancelled producer has stopped
// feeding.
func waitForSyncEvent(ctx context.Context, ch <-chan sync.IngestEvent, gen int) tea.Cmd {
	if ctx == nil || ch == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case ev, ok := <-ch:
			if !ok {
				return syncEventMsg{gen: gen, closed: true}
			}
			return syncEventMsg{gen: gen, ev: ev}
		case <-ctx.Done():
			return nil
		}
	}
}

func waitForEmbedEvent(ctx context.Context, ch <-chan embed.EmbedEvent, gen int) tea.Cmd {
	if ctx == nil || ch == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case ev, ok := <-ch:
			if !ok {
				return embedEventMsg{gen: gen, closed: true}
			}
			return embedEventMsg{gen: gen, ev: ev}
		case <-ctx.Done():
			return nil
		}
	}
}

// runSyncCmd validates preconditions on the command goroutine, then opens the
// ingest pipeline and hands its channel back. It mirrors commands/sync.go's
// defaults (owned + starred, chunk size derived from the embedding model) so the
// TUI and CLI produce identical data.
func runSyncCmd(app *appContext, gen int, ctx context.Context) tea.Cmd {
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

		ch := sync.IngestRepos(ctx, sync.IngestOptions{
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
func runEmbedCmd(app *appContext, gen int, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		database := app.db()
		if database == nil {
			return opErrMsg{gen: gen, err: fmt.Errorf("not configured — open Settings to finish setup")}
		}
		embedProvider, err := app.embedProvider()
		if err != nil {
			return opErrMsg{gen: gen, err: err}
		}
		ch := embed.RunEmbedPipeline(ctx, embed.EmbedOptions{
			IncludeFileTree:   true,
			DB:                database,
			EmbeddingProvider: embedProvider,
		})
		return embedStartedMsg{gen: gen, ch: ch}
	}
}

// bannerSync / bannerEmbed render the terminal summary line, tinted red when the
// run reported errors (matching the CLI's completion message).
func bannerSync(synced, skipped, errors int) string {
	style := okStyle
	if errors > 0 {
		style = errStyle
	}
	return style.Render(fmt.Sprintf("✓ Sync complete — %d synced, %d skipped, %d errors",
		synced, skipped, errors))
}

func bannerEmbed(embedded, skipped, errored int) string {
	style := okStyle
	if errored > 0 {
		style = errStyle
	}
	return style.Render(fmt.Sprintf("✓ Embedding complete — %d embedded, %d skipped, %d errors",
		embedded, skipped, errored))
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
