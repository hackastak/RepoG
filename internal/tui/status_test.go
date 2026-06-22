package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/status"
)

// sampleStatusResult builds a populated snapshot for rendering tests.
func sampleStatusResult() status.Result {
	var r status.Result
	r.Repos.Total = 250
	r.Repos.Owned = 90
	r.Repos.Starred = 162
	r.Repos.EmbeddedCount = 10
	r.Repos.PendingEmbed = 240
	r.Embed.TotalChunks = 838
	r.Embed.TotalEmbeddings = 42

	syncStatus := "completed"
	lastSynced := "2026-06-10T12:00:00Z"
	r.Sync.LastSyncStatus = &syncStatus
	r.Sync.LastSyncedAt = &lastSynced

	r.DB.Path = "/home/u/.repog/repog.db"
	r.DB.SizeMB = "5.14 MB"
	return r
}

// TestStatusViewLoadedRenders feeds a loaded message and confirms the panel
// leaves the loading state and renders the data.
func TestStatusViewLoadedRenders(t *testing.T) {
	v := newStatusView(&appContext{})

	updated, _ := v.Update(statusLoadedMsg{result: sampleStatusResult()})
	sv := updated.(*statusView)

	if sv.loading {
		t.Fatal("expected loading to be false after a successful load")
	}

	out := sv.View(80, 24)
	for _, want := range []string{"Repositories", "250", "Knowledge Base", "838", "completed", "Database"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected status output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestStatusViewLanguageBars confirms the Languages section renders names and
// percentages when the result carries a breakdown.
func TestStatusViewLanguageBars(t *testing.T) {
	v := newStatusView(&appContext{})

	res := sampleStatusResult()
	res.Languages = []status.LanguageStat{
		{Language: "Go", Count: 113, Percent: 45.2},
		{Language: "Rust", Count: 50, Percent: 20.0},
	}

	updated, _ := v.Update(statusLoadedMsg{result: res})
	sv := updated.(*statusView)

	out := sv.View(80, 24)
	for _, want := range []string{"Languages", "Go", "45.2%", "Rust"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected language section to contain %q, got:\n%s", want, out)
		}
	}
}

// TestStatusViewLoadingState renders the placeholder before any data arrives.
func TestStatusViewLoadingState(t *testing.T) {
	v := newStatusView(&appContext{})
	if out := v.View(80, 24); !strings.Contains(out, "Loading") {
		t.Fatalf("expected a loading placeholder, got:\n%s", out)
	}
}

// TestStatusViewError surfaces a load error in the panel and stays unloaded.
func TestStatusViewError(t *testing.T) {
	v := newStatusView(&appContext{})

	updated, _ := v.Update(statusLoadedMsg{err: errors.New("boom")})
	sv := updated.(*statusView)

	if sv.result != nil {
		t.Fatal("expected result to remain nil when the load errored")
	}
	if out := sv.View(80, 24); !strings.Contains(out, "boom") {
		t.Fatalf("expected the error in output, got:\n%s", out)
	}
}

// TestStatusViewReloadKey verifies ctrl+r re-enters loading and returns a
// command to fetch again.
func TestStatusViewReloadKey(t *testing.T) {
	v := newStatusView(&appContext{})
	v.loading = false

	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	sv := updated.(*statusView)

	if !sv.loading {
		t.Fatal("expected loading to be true after ctrl+r")
	}
	if cmd == nil {
		t.Fatal("expected a reload command, got nil")
	}
}
