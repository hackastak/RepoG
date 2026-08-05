package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/recommend"
	"github.com/hackastak/repog/internal/summarize"
)

// readyReposView builds a Repos view with rows loaded and the cursor on the
// first row, the state the action keys assume.
func readyReposView() *reposView {
	v := newReposView(&appContext{})
	v.loading = false
	v.rows = []repoRow{
		{FullName: "charmbracelet/bubbletea", Language: "Go", Stars: 100, Synced: true, Embedded: true},
		{FullName: "owner/beta", Language: "Rust", Stars: 5, Synced: true},
	}
	v.refreshRows()
	return v
}

// pressKey sends a single-rune key (e.g. "s", "r") to the view.
func pressKey(v *reposView, s string) *reposView {
	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return updated.(*reposView)
}

// TestReposStartSummarize confirms "s" opens the summary pane for the focused
// repo, enters the busy state, bumps the generation, and creates a stream.
func TestReposStartSummarize(t *testing.T) {
	v := readyReposView()
	prevGen := v.opGen

	rv := pressKey(v, "s")

	if rv.mode != reposModeSummary {
		t.Fatalf("expected summary mode, got %v", rv.mode)
	}
	if rv.target != "charmbracelet/bubbletea" {
		t.Fatalf("expected target to be the focused repo, got %q", rv.target)
	}
	if !rv.busy {
		t.Fatal("expected the view to be busy after starting a summary")
	}
	if rv.opGen != prevGen+1 {
		t.Fatalf("expected generation to advance, got %d", rv.opGen)
	}
	if rv.sumStream == nil {
		t.Fatal("expected a stream channel to be created")
	}
	if !strings.Contains(rv.HelpKeys(), "esc back") {
		t.Fatalf("expected action help keys, got %q", rv.HelpKeys())
	}
}

// TestReposStartRecommend confirms "r" opens the recommend pane for the focused
// repo without a stream channel (recommend resolves in one call).
func TestReposStartRecommend(t *testing.T) {
	v := readyReposView()

	rv := pressKey(v, "r")

	if rv.mode != reposModeRecommend {
		t.Fatalf("expected recommend mode, got %v", rv.mode)
	}
	if rv.target != "charmbracelet/bubbletea" {
		t.Fatalf("expected target to be the focused repo, got %q", rv.target)
	}
	if !rv.busy {
		t.Fatal("expected the view to be busy after starting a recommend")
	}
}

// TestReposActionEscReturnsToTable confirms esc dismisses the result pane.
func TestReposActionEscReturnsToTable(t *testing.T) {
	v := readyReposView()
	v.mode = reposModeSummary

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyEsc})
	rv := updated.(*reposView)

	if rv.mode != reposModeTable {
		t.Fatalf("expected esc to return to the table, got %v", rv.mode)
	}
}

// TestReposSummaryStreams confirms chunks accumulate and the read chain continues.
func TestReposSummaryStreams(t *testing.T) {
	v := readyReposView()
	v.mode = reposModeSummary
	v.busy = true
	v.sumStream = make(chan tea.Msg, 4)
	v.sumCtx = context.Background()

	updated, cmd := v.Update(summarizeChunkMsg{gen: v.opGen, chunk: "## Overview\n"})
	rv := updated.(*reposView)
	if rv.summary != "## Overview\n" {
		t.Fatalf("expected first chunk accumulated, got %q", rv.summary)
	}
	if cmd == nil {
		t.Fatal("expected a follow-up wait command after a chunk")
	}

	updated, _ = rv.Update(summarizeChunkMsg{gen: rv.opGen, chunk: "Some prose."})
	rv = updated.(*reposView)
	if rv.summary != "## Overview\nSome prose." {
		t.Fatalf("expected chunks to accumulate, got %q", rv.summary)
	}
}

// TestReposSummaryIgnoresStaleChunks drops chunks from a superseded action.
func TestReposSummaryIgnoresStaleChunks(t *testing.T) {
	v := readyReposView()
	v.mode = reposModeSummary
	v.opGen = 2

	updated, cmd := v.Update(summarizeChunkMsg{gen: 1, chunk: "stale"})
	rv := updated.(*reposView)
	if rv.summary != "" {
		t.Fatalf("expected stale chunk to be ignored, got %q", rv.summary)
	}
	if cmd != nil {
		t.Fatal("expected no follow-up command for a stale chunk")
	}
}

// TestReposSummaryRenders feeds a completed summary and checks the body shows the
// summary text and the metrics line.
func TestReposSummaryRenders(t *testing.T) {
	v := readyReposView()
	v.mode = reposModeSummary
	v.target = "charmbracelet/bubbletea"
	v.busy = true

	updated, _ := v.Update(summarizeDoneMsg{gen: v.opGen, result: summarize.SummarizeResult{
		Summary:      "## Overview\nA Go TUI framework.",
		ChunksUsed:   4,
		InputTokens:  900,
		OutputTokens: 120,
		DurationMs:   640,
	}})
	rv := updated.(*reposView)

	if rv.busy {
		t.Fatal("expected busy to clear after the summary completes")
	}
	out := rv.View(80, 24)
	for _, want := range []string{"Summary: charmbracelet/bubbletea", "A Go TUI framework", "4 chunks", "tokens"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected summary output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestReposRecommendRenders feeds completed recommendations and checks the body.
func TestReposRecommendRenders(t *testing.T) {
	v := readyReposView()
	v.mode = reposModeRecommend
	v.target = "charmbracelet/bubbletea"
	v.busy = true

	updated, _ := v.Update(recommendDoneMsg{gen: v.opGen, result: recommend.RecommendResult{
		Recommendations: []recommend.Recommendation{
			{Rank: 1, RepoFullName: "charmbracelet/lipgloss", HTMLURL: "https://github.com/charmbracelet/lipgloss", Reasoning: "Styling library for TUIs."},
		},
		CandidatesConsidered: 12,
	}})
	rv := updated.(*reposView)

	out := rv.View(80, 24)
	for _, want := range []string{"Related to: charmbracelet/bubbletea", "charmbracelet/lipgloss", "Styling library", "12 candidates"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected recommend output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestReposRecommendNoEmbeddings shows the sync/embed hint when the corpus is empty.
func TestReposRecommendNoEmbeddings(t *testing.T) {
	v := readyReposView()
	v.mode = reposModeRecommend
	v.busy = true

	updated, _ := v.Update(recommendDoneMsg{gen: v.opGen, noEmbeddings: true})
	rv := updated.(*reposView)

	if out := rv.View(80, 24); !strings.Contains(out, "No embeddings") {
		t.Fatalf("expected a no-embeddings hint, got:\n%s", out)
	}
}

// TestReposActionError surfaces an action error in the body.
func TestReposActionError(t *testing.T) {
	v := readyReposView()
	v.mode = reposModeRecommend
	v.busy = true

	updated, _ := v.Update(recommendDoneMsg{gen: v.opGen, err: errSentinel("boom")})
	rv := updated.(*reposView)

	if out := rv.View(80, 24); !strings.Contains(out, "boom") {
		t.Fatalf("expected the error in output, got:\n%s", out)
	}
}

// TestDropRepo removes the source repo from its own recommendations and caps the list.
func TestDropRepo(t *testing.T) {
	recs := []recommend.Recommendation{
		{Rank: 1, RepoFullName: "charmbracelet/bubbletea"},
		{Rank: 2, RepoFullName: "charmbracelet/lipgloss"},
		{Rank: 3, RepoFullName: "Charmbracelet/Bubbles"}, // case-insensitive non-match
		{Rank: 4, RepoFullName: "owner/extra"},
	}

	got := dropRepo(recs, "charmbracelet/bubbletea", 2)
	if len(got) != 2 {
		t.Fatalf("expected the list capped at 2, got %d", len(got))
	}
	for _, r := range got {
		if strings.EqualFold(r.RepoFullName, "charmbracelet/bubbletea") {
			t.Fatalf("expected the source repo dropped, found %q", r.RepoFullName)
		}
	}
}
