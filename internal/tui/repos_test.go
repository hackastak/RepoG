package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestReposViewLoadBeforeRenderNoPanic guards the regression where rows loaded
// before the first View() (which set columns) caused bubbles/table.renderRow to
// index into zero columns and panic. Columns are now set at construction, so
// SetRows is safe at any time.
func TestReposViewLoadBeforeRenderNoPanic(t *testing.T) {
	v := newReposView(&appContext{}) // nil db; we inject the load result directly

	rows := []repoRow{
		{FullName: "owner/alpha", Language: "Go", Stars: 120, IsOwned: true, Synced: true, Embedded: true},
		{FullName: "owner/beta", Language: "", Stars: 3, Synced: true},
	}

	// Data arrives before any WindowSizeMsg / View — this is the panic path.
	view, _ := v.Update(reposLoadedMsg{rows: rows})

	out := view.View(80, 24)
	if !strings.Contains(out, "owner/alpha") {
		t.Fatalf("expected rendered table to include a repo name, got:\n%s", out)
	}
}

// TestReposViewSelectToggle verifies space toggles selection on the focused row
// and the footer reflects the count.
func TestReposViewSelectToggle(t *testing.T) {
	v := newReposView(&appContext{})
	v.rows = []repoRow{{FullName: "owner/alpha", Stars: 1, Synced: true}}
	v.loading = false
	v.refreshRows()

	if got := v.selectedCount(); got != 0 {
		t.Fatalf("expected 0 selected initially, got %d", got)
	}

	view, _ := v.Update(tea.KeyMsg{Type: tea.KeySpace})
	rv := view.(*reposView)
	if got := rv.selectedCount(); got != 1 {
		t.Fatalf("expected 1 selected after space, got %d", got)
	}

	if out := rv.View(80, 24); !strings.Contains(out, "1 selected") {
		t.Fatalf("expected footer to show selection count, got:\n%s", out)
	}
}

// TestReposViewEmptyRenders confirms the empty state renders without a size and
// without panicking.
func TestReposViewEmptyRenders(t *testing.T) {
	v := newReposView(&appContext{})
	v.loading = false
	if out := v.View(0, 0); !strings.Contains(out, "No repositories") {
		t.Fatalf("expected empty-state hint, got:\n%s", out)
	}
}
