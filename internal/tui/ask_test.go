package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/ask"
)

// sampleAskResult builds a populated answer result for rendering tests.
func sampleAskResult() ask.AskResult {
	return ask.AskResult{
		Answer: "Bubble Tea is a Go TUI framework based on the Elm architecture.",
		Sources: []ask.SourceAttribution{
			{RepoFullName: "charmbracelet/bubbletea", ChunkType: "readme", Similarity: 0.91},
			{RepoFullName: "charmbracelet/lipgloss", ChunkType: "metadata", Similarity: 0.78},
		},
		Question:     "what is bubbletea",
		InputTokens:  1200,
		OutputTokens: 64,
		DurationMs:   850,
	}
}

// TestAskViewStreamsChunks confirms chunks accumulate into the answer and that
// the read chain continues (each chunk re-issues a wait command).
func TestAskViewStreamsChunks(t *testing.T) {
	v := newAskView(&appContext{})
	v.streaming = true
	v.stream = make(chan tea.Msg, 4)
	v.lastQ = "what is bubbletea"

	updated, cmd := v.Update(askChunkMsg{gen: v.gen, chunk: "Bubble "})
	av := updated.(*askView)
	if av.answer != "Bubble " {
		t.Fatalf("expected first chunk accumulated, got %q", av.answer)
	}
	if cmd == nil {
		t.Fatal("expected a follow-up wait command after a chunk")
	}

	updated, _ = av.Update(askChunkMsg{gen: av.gen, chunk: "Tea"})
	av = updated.(*askView)
	if av.answer != "Bubble Tea" {
		t.Fatalf("expected chunks to accumulate, got %q", av.answer)
	}
}

// TestAskViewIgnoresStaleChunks drops chunks from a superseded question so a
// slow earlier stream can't bleed into a newer one.
func TestAskViewIgnoresStaleChunks(t *testing.T) {
	v := newAskView(&appContext{})
	v.streaming = true
	v.gen = 2

	updated, cmd := v.Update(askChunkMsg{gen: 1, chunk: "stale"})
	av := updated.(*askView)
	if av.answer != "" {
		t.Fatalf("expected stale chunk to be ignored, got %q", av.answer)
	}
	if cmd != nil {
		t.Fatal("expected no follow-up command for a stale chunk")
	}
}

// TestAskViewRendersAnswer feeds a completed ask and confirms the view leaves the
// streaming state and renders the answer, sources, and metrics.
func TestAskViewRendersAnswer(t *testing.T) {
	v := newAskView(&appContext{})
	v.streaming = true
	v.lastQ = "what is bubbletea"

	updated, _ := v.Update(askDoneMsg{gen: v.gen, result: sampleAskResult()})
	av := updated.(*askView)

	if av.streaming {
		t.Fatal("expected streaming to be false after a completed ask")
	}
	if !av.answered {
		t.Fatal("expected answered to be true after a completed ask")
	}

	out := av.View(80, 24)
	for _, want := range []string{"Bubble Tea is a Go TUI framework", "charmbracelet/bubbletea", "Sources:", "tokens"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected ask output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestAskViewNoEmbeddings shows the sync/embed hint when the corpus is empty.
func TestAskViewNoEmbeddings(t *testing.T) {
	v := newAskView(&appContext{})
	v.streaming = true

	updated, _ := v.Update(askDoneMsg{gen: v.gen, noEmbeddings: true})
	av := updated.(*askView)

	if out := av.View(80, 24); !strings.Contains(out, "No embeddings") {
		t.Fatalf("expected a no-embeddings hint, got:\n%s", out)
	}
}

// TestAskViewError surfaces an ask error in the body.
func TestAskViewError(t *testing.T) {
	v := newAskView(&appContext{})
	v.streaming = true

	updated, _ := v.Update(askDoneMsg{gen: v.gen, err: errSentinel("boom")})
	av := updated.(*askView)

	if out := av.View(80, 24); !strings.Contains(out, "boom") {
		t.Fatalf("expected the error in output, got:\n%s", out)
	}
}

// TestAskViewEnterStartsAsk confirms Enter on a non-empty question enters the
// streaming state, bumps the generation, and returns a command.
func TestAskViewEnterStartsAsk(t *testing.T) {
	v := newAskView(&appContext{})
	v.input.SetValue("how does sync work")

	prevGen := v.gen
	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	av := updated.(*askView)

	if !av.streaming {
		t.Fatal("expected streaming to be true after Enter on a non-empty question")
	}
	if av.gen != prevGen+1 {
		t.Fatalf("expected generation to advance, got %d", av.gen)
	}
	if av.stream == nil {
		t.Fatal("expected a stream channel to be created")
	}
	if cmd == nil {
		t.Fatal("expected an ask command, got nil")
	}
}

// TestAskViewEnterIgnoresBlank does nothing when the question is only whitespace.
func TestAskViewEnterIgnoresBlank(t *testing.T) {
	v := newAskView(&appContext{})
	v.input.SetValue("   ")

	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	av := updated.(*askView)

	if av.streaming {
		t.Fatal("expected a blank question not to start an ask")
	}
	if cmd != nil {
		t.Fatal("expected no command for a blank question")
	}
}

// TestAskViewEnterIgnoredWhileStreaming prevents starting a second ask before the
// first finishes (which would orphan the in-flight stream).
func TestAskViewEnterIgnoredWhileStreaming(t *testing.T) {
	v := newAskView(&appContext{})
	v.input.SetValue("another question")
	v.streaming = true

	prevGen := v.gen
	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	av := updated.(*askView)

	if av.gen != prevGen {
		t.Fatal("expected Enter to be ignored while a stream is in flight")
	}
}

// TestAskViewEscBlurs confirms esc releases focus so global keys work again.
func TestAskViewEscBlurs(t *testing.T) {
	v := newAskView(&appContext{})
	if !v.capturingText() {
		t.Fatal("expected the question box to be focused initially")
	}

	v.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if v.capturingText() {
		t.Fatal("expected esc to blur the question box")
	}
}
