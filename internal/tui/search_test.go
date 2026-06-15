package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/search"
)

// sampleSearchResult builds a populated query result for rendering tests.
func sampleSearchResult() search.SearchQueryResult {
	return search.SearchQueryResult{
		Results: []search.SearchResult{
			{
				RepoFullName: "charmbracelet/bubbletea",
				Description:  "A powerful TUI framework",
				Language:     "Go",
				Stars:        25000,
				HTMLURL:      "https://github.com/charmbracelet/bubbletea",
				ChunkType:    "readme",
				Content:      "Bubble Tea is a framework based on the Elm architecture.",
				Similarity:   0.91,
			},
			{
				RepoFullName: "rivo/tview",
				Language:     "Go",
				Stars:        10000,
				HTMLURL:      "https://github.com/rivo/tview",
				ChunkType:    "metadata",
				Content:      "Terminal UI library with rich widgets.",
				Similarity:   0.82,
			},
		},
		TotalConsidered:  120,
		QueryEmbeddingMs: 42,
		SearchMs:         7,
	}
}

// TestSearchViewRendersResults feeds a completed search and confirms the view
// leaves the searching state and renders the result rows + timing stats.
func TestSearchViewRendersResults(t *testing.T) {
	v := newSearchView(&appContext{})
	v.searching = true

	updated, _ := v.Update(searchDoneMsg{result: sampleSearchResult(), query: "tui framework"})
	sv := updated.(*searchView)

	if sv.searching {
		t.Fatal("expected searching to be false after a completed search")
	}
	if !sv.searched {
		t.Fatal("expected searched to be true after a completed search")
	}

	out := sv.View(80, 24)
	for _, want := range []string{"charmbracelet/bubbletea", "rivo/tview", "2 results", "chunks considered"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected search output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestSearchViewNoEmbeddings shows the sync/embed hint when the corpus is empty.
func TestSearchViewNoEmbeddings(t *testing.T) {
	v := newSearchView(&appContext{})

	updated, _ := v.Update(searchDoneMsg{noEmbeddings: true, query: "anything"})
	sv := updated.(*searchView)

	if out := sv.View(80, 24); !strings.Contains(out, "No embeddings") {
		t.Fatalf("expected a no-embeddings hint, got:\n%s", out)
	}
}

// TestSearchViewNoResults renders the empty-state for a query that matched
// nothing, echoing the query back.
func TestSearchViewNoResults(t *testing.T) {
	v := newSearchView(&appContext{})

	updated, _ := v.Update(searchDoneMsg{query: "no such repo"})
	sv := updated.(*searchView)

	out := sv.View(80, 24)
	if !strings.Contains(out, "No matching") || !strings.Contains(out, "no such repo") {
		t.Fatalf("expected an empty-result hint mentioning the query, got:\n%s", out)
	}
}

// TestSearchViewError surfaces a search error in the body.
func TestSearchViewError(t *testing.T) {
	v := newSearchView(&appContext{})

	updated, _ := v.Update(searchDoneMsg{err: errSentinel("boom")})
	sv := updated.(*searchView)

	if out := sv.View(80, 24); !strings.Contains(out, "boom") {
		t.Fatalf("expected the error in output, got:\n%s", out)
	}
}

// TestSearchViewEnterStartsSearch confirms Enter on a non-empty query enters the
// searching state and returns a command to run the query.
func TestSearchViewEnterStartsSearch(t *testing.T) {
	v := newSearchView(&appContext{})
	v.input.SetValue("graph database")

	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	sv := updated.(*searchView)

	if !sv.searching {
		t.Fatal("expected searching to be true after Enter on a non-empty query")
	}
	if cmd == nil {
		t.Fatal("expected a search command, got nil")
	}
}

// TestSearchViewEnterIgnoresBlank does nothing when the query is only whitespace.
func TestSearchViewEnterIgnoresBlank(t *testing.T) {
	v := newSearchView(&appContext{})
	v.input.SetValue("   ")

	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	sv := updated.(*searchView)

	if sv.searching {
		t.Fatal("expected a blank query not to start a search")
	}
	if cmd != nil {
		t.Fatal("expected no command for a blank query")
	}
}

// TestSearchViewEscBlurs confirms esc releases focus so global keys work again.
func TestSearchViewEscBlurs(t *testing.T) {
	v := newSearchView(&appContext{})
	if !v.capturingText() {
		t.Fatal("expected the query box to be focused initially")
	}

	v.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if v.capturingText() {
		t.Fatal("expected esc to blur the query box")
	}
}

// TestRootSuppressesGlobalKeysWhileTyping verifies the root model does not treat
// digit keys as tab switches while the Search box is focused, but ctrl+c still
// quits. This guards the textInputView routing added with the Search view.
func TestRootSuppressesGlobalKeysWhileTyping(t *testing.T) {
	m := newRootModel(&appContext{}, false)

	// Switch to the Search tab (its input focuses on Init).
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m = nm.(rootModel)
	if m.active != tabSearch {
		t.Fatalf("expected to be on the Search tab, got %v", m.active)
	}

	// "1" must NOT jump to the Repos tab while the query box is focused.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = nm.(rootModel)
	if m.active != tabSearch {
		t.Fatalf("expected to stay on Search while typing, jumped to %v", m.active)
	}

	// ctrl+c stays global and quits even while typing.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = nm.(rootModel)
	if !m.quitting {
		t.Fatal("expected ctrl+c to quit even while the query box is focused")
	}
}

// errSentinel is a tiny error type so tests don't pull in extra imports.
type errSentinel string

func (e errSentinel) Error() string { return string(e) }
