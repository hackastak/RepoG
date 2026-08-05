package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/recommend"
	"github.com/hackastak/repog/internal/summarize"
)

// reposMode is the Repos tab's display state: the table, or a per-repo action
// result pane that temporarily replaces it.
type reposMode int

const (
	reposModeTable reposMode = iota
	reposModeSummary
	reposModeRecommend
)

// recommendCount over-fetches relative to the CLI default of 3: a TUI pane has
// room to browse and the result viewport scrolls.
const recommendCount = 5

// focusedRow returns the repo under the table cursor, or false when the list is
// empty / the cursor is out of range.
func (v *reposView) focusedRow() (repoRow, bool) {
	i := v.table.Cursor()
	if i < 0 || i >= len(v.rows) {
		return repoRow{}, false
	}
	return v.rows[i], true
}

// releaseStream cancels the in-flight summary stream (if any) and clears its
// context, unwinding the producer goroutine and aborting its HTTP request.
// Calling it when nothing is streaming is a harmless no-op reset.
func (v *reposView) releaseStream() {
	if v.sumCancel != nil {
		v.sumCancel()
		v.sumCancel = nil
	}
	v.sumCtx = nil
}

// cancelStream implements streamCanceler; the root model calls it on quit so a
// streaming summary doesn't strand its goroutine when the program exits.
func (v *reposView) cancelStream() { v.releaseStream() }

// startSummarize kicks off a streaming AI summary of the focused repo. Like the
// Ask view, the summary streams token-by-token over a channel; opGen guards a
// superseded action's stale messages.
func (v *reposView) startSummarize() (view, tea.Cmd) {
	repo, ok := v.focusedRow()
	if !ok {
		return v, nil
	}
	// Tear down any previous action's stream before starting a new one.
	v.releaseStream()
	v.opGen++
	v.mode = reposModeSummary
	v.target = repo.FullName
	v.busy = true
	v.opErr = nil
	v.noEmbeddings = false
	v.summary = ""
	v.opContent = ""
	v.sumResult = summarize.SummarizeResult{}
	v.viewport.GotoTop()

	ch := make(chan tea.Msg, 256)
	v.sumStream = ch
	ctx, cancel := context.WithCancel(context.Background())
	v.sumCtx = ctx
	v.sumCancel = cancel
	// runSummarizeCmd reads the first message itself; the read chain then
	// continues from updateAction on each summarizeChunkMsg (issuing
	// waitForStreamMsg here too would add a second concurrent reader).
	return v, tea.Batch(v.spinner.Tick, runSummarizeCmd(v.app, repo.FullName, v.opGen, ctx, ch))
}

// startRecommend kicks off a (non-streaming) recommendation of repos related to
// the focused one, mirroring the Search view's single-shot command pattern.
func (v *reposView) startRecommend() (view, tea.Cmd) {
	repo, ok := v.focusedRow()
	if !ok {
		return v, nil
	}
	// A recommend supersedes any in-flight summary stream; tear it down.
	v.releaseStream()
	v.opGen++
	v.mode = reposModeRecommend
	v.target = repo.FullName
	v.busy = true
	v.opErr = nil
	v.noEmbeddings = false
	v.opContent = ""
	v.recResult = recommend.RecommendResult{}
	v.viewport.GotoTop()

	return v, tea.Batch(v.spinner.Tick, runRecommendCmd(v.app, repo, v.opGen))
}

// updateAction handles the action stream/done/spinner messages for both
// summarize and recommend. Messages from a superseded action (opGen mismatch)
// are dropped without re-issuing their wait command, ending that read chain.
func (v *reposView) updateAction(msg tea.Msg) (view, tea.Cmd) {
	switch msg := msg.(type) {
	case summarizeChunkMsg:
		if msg.gen != v.opGen {
			return v, nil
		}
		v.summary += msg.chunk
		return v, waitForStreamMsg(v.sumCtx, v.sumStream)

	case summarizeDoneMsg:
		if msg.gen != v.opGen {
			return v, nil
		}
		v.busy = false
		v.sumStream = nil
		v.releaseStream()
		v.opErr = msg.err
		if msg.err == nil {
			v.sumResult = msg.result
			// The final summary is authoritative; the empty-data fallback isn't
			// streamed token-wise.
			v.summary = msg.result.Summary
		}
		return v, nil

	case recommendDoneMsg:
		if msg.gen != v.opGen {
			return v, nil
		}
		v.busy = false
		v.opErr = msg.err
		v.noEmbeddings = msg.noEmbeddings
		if msg.err == nil && !msg.noEmbeddings {
			v.recResult = msg.result
		}
		return v, nil

	case spinner.TickMsg:
		if !v.busy {
			return v, nil
		}
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		return v, cmd
	}
	return v, nil
}

// updateActionKey handles keys while a result pane is open: esc dismisses it
// back to the table, everything else scrolls the viewport. A still-streaming
// summary keeps draining in the background (opGen is unchanged), so esc is a
// dismissal, not a cancel.
func (v *reposView) updateActionKey(msg tea.KeyMsg) (view, tea.Cmd) {
	if msg.String() == "esc" {
		v.mode = reposModeTable
		return v, nil
	}
	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return v, cmd
}

// renderAction draws the result pane for the active per-repo action: a title
// line, then the spinner, an error, an empty-state, or the scrollable result.
func (v *reposView) renderAction(width, height int) string {
	var b strings.Builder

	var title string
	switch v.mode {
	case reposModeSummary:
		title = "Summary: " + v.target
	case reposModeRecommend:
		title = "Related to: " + v.target
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	switch {
	case v.busy:
		verb := "Summarizing…"
		if v.mode == reposModeRecommend {
			verb = "Finding recommendations…"
		}
		b.WriteString(v.spinner.View() + helpStyle.Render(" "+verb))
		// A summary streams in while busy; show what we have so far.
		if v.mode == reposModeSummary && v.summary != "" {
			b.WriteString("\n\n" + v.summary)
		}
	case v.opErr != nil:
		b.WriteString(errStyle.Render(actionVerb(v.mode) + " failed: " + v.opErr.Error()))
	case v.noEmbeddings:
		b.WriteString(helpStyle.Render("No embeddings yet. Run a sync, then embed, from the Sync/Embed tab."))
	default:
		// The title + blank line take two rows; leave the rest for the result.
		v.viewport.Width = width
		v.viewport.Height = max(height-2, 1)
		content := v.renderActionResult()
		if content != v.opContent {
			v.viewport.SetContent(content)
			v.opContent = content
		}
		b.WriteString(v.viewport.View())
	}

	return b.String()
}

func actionVerb(m reposMode) string {
	if m == reposModeRecommend {
		return "Recommend"
	}
	return "Summarize"
}

// renderActionResult formats the finished result for the viewport.
func (v *reposView) renderActionResult() string {
	if v.mode == reposModeSummary {
		var b strings.Builder
		b.WriteString(v.summary)
		if v.sumResult.ChunksUsed > 0 {
			b.WriteString("\n\n" + helpStyle.Render(fmt.Sprintf(
				"%d chunks · %d in / %d out tokens · %dms",
				v.sumResult.ChunksUsed, v.sumResult.InputTokens,
				v.sumResult.OutputTokens, v.sumResult.DurationMs)))
		}
		return b.String()
	}

	// Recommend.
	if len(v.recResult.Recommendations) == 0 {
		return helpStyle.Render("No related repositories found.")
	}
	var b strings.Builder
	for i, r := range v.recResult.Recommendations {
		fmt.Fprintf(&b, "%d. %s\n", r.Rank, titleStyle.Render(r.RepoFullName))
		b.WriteString("   " + helpStyle.Render(r.HTMLURL) + "\n")
		b.WriteString("   " + r.Reasoning + "\n")
		if i < len(v.recResult.Recommendations)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n" + helpStyle.Render(fmt.Sprintf(
		"%d candidates considered · %d in / %d out tokens · %dms",
		v.recResult.CandidatesConsidered, v.recResult.InputTokens,
		v.recResult.OutputTokens, v.recResult.DurationMs)))
	return b.String()
}

// summarizeChunkMsg delivers one streamed token (or token group) of the summary.
type summarizeChunkMsg struct {
	gen   int
	chunk string
}

// summarizeDoneMsg delivers the final outcome of an async summarize.
type summarizeDoneMsg struct {
	gen    int
	result summarize.SummarizeResult
	err    error
}

// recommendDoneMsg delivers the outcome of an async recommend.
type recommendDoneMsg struct {
	gen          int
	result       recommend.RecommendResult
	noEmbeddings bool
	err          error
}

// All three route to the Repos tab so an action started there keeps advancing
// even if the user switches tabs mid-flight (see routedMsg).
func (summarizeChunkMsg) targetTab() tab { return tabRepos }
func (summarizeDoneMsg) targetTab() tab  { return tabRepos }
func (recommendDoneMsg) targetTab() tab  { return tabRepos }

// runSummarizeCmd validates preconditions on the UI goroutine, then streams the
// summary off a background goroutine onto ch, finishing with a summarizeDoneMsg.
// It reuses the shared LLM provider and the same summarize.SummarizeRepo the CLI
// calls; the empty-data case is handled inside SummarizeRepo (it streams a hint).
// ctx cancels the stream (see streamMessages), so quitting mid-summary tears the
// goroutine and its HTTP request down instead of leaking them.
func runSummarizeCmd(app *appContext, repo string, gen int, ctx context.Context, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		database := app.db()
		if database == nil {
			return summarizeDoneMsg{gen: gen, err: fmt.Errorf("not configured")}
		}
		llmProvider, err := app.llmProvider()
		if err != nil {
			return summarizeDoneMsg{gen: gen, err: err}
		}

		return streamMessages(ctx, ch, func(send func(tea.Msg)) {
			result, sumErr := summarize.SummarizeRepo(ctx, summarize.SummarizeOptions{
				Repo:        repo,
				DB:          database,
				LLMProvider: llmProvider,
			}, func(chunk string) {
				send(summarizeChunkMsg{gen: gen, chunk: chunk})
			})
			send(summarizeDoneMsg{gen: gen, result: result, err: sumErr})
		})
	}
}

// runRecommendCmd runs the recommendation off the UI goroutine and returns the
// result as a tea.Msg. The query is derived from the focused repo so the action
// reads as "repos related to this one"; the source repo is dropped from the
// results since it would otherwise rank as its own closest match. It reuses the
// shared providers and the same recommend.RecommendRepos the CLI calls, with the
// empty-corpus guard surfaced as an empty-state (mirrors runSearchCmd).
func runRecommendCmd(app *appContext, repo repoRow, gen int) tea.Cmd {
	return func() tea.Msg {
		database := app.db()
		if database == nil {
			return recommendDoneMsg{gen: gen, err: fmt.Errorf("not configured")}
		}

		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM chunk_embeddings").Scan(&count); err != nil {
			return recommendDoneMsg{gen: gen, err: err}
		}
		if count == 0 {
			return recommendDoneMsg{gen: gen, noEmbeddings: true}
		}

		embedProvider, err := app.embedProvider()
		if err != nil {
			return recommendDoneMsg{gen: gen, err: err}
		}
		llmProvider, err := app.llmProvider()
		if err != nil {
			return recommendDoneMsg{gen: gen, err: err}
		}

		query := fmt.Sprintf("Repositories similar to %s", repo.FullName)
		if repo.Language != "" {
			query += fmt.Sprintf(" (%s)", repo.Language)
		}

		// Over-fetch by one so dropping the source repo still leaves recommendCount.
		result, err := recommend.RecommendRepos(context.Background(), recommend.RecommendOptions{
			Query:             query,
			Limit:             recommendCount + 1,
			DB:                database,
			EmbeddingProvider: embedProvider,
			LLMProvider:       llmProvider,
		})
		if err != nil {
			return recommendDoneMsg{gen: gen, err: err}
		}
		result.Recommendations = dropRepo(result.Recommendations, repo.FullName, recommendCount)
		return recommendDoneMsg{gen: gen, result: result}
	}
}

// dropRepo removes the source repo from its own recommendation list (it ranks as
// its own nearest neighbour) and caps the list at limit.
func dropRepo(recs []recommend.Recommendation, self string, limit int) []recommend.Recommendation {
	out := make([]recommend.Recommendation, 0, len(recs))
	for _, r := range recs {
		if strings.EqualFold(r.RepoFullName, self) {
			continue
		}
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out
}
