package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/sync"
)

// TestRoutedSyncEventReachesOwningView confirms a Sync/Embed pipeline event is
// delivered to the Sync view even when a different tab is active, so a run keeps
// progressing across tab switches.
func TestRoutedSyncEventReachesOwningView(t *testing.T) {
	m := newRootModel(&appContext{}, false)

	// Put a sync run in flight, then move focus to a different tab.
	sv := m.views[tabSync].(*syncView)
	sv.running = true
	sv.phase = phaseSyncing
	sv.gen = 1
	m.active = tabStatus

	updated, _ := m.Update(syncEventMsg{gen: 1, ev: sync.IngestEvent{
		Type: "repo", Repo: "me/alpha", Status: "new",
	}})
	m = updated.(rootModel)

	if got := m.views[tabSync].(*syncView).syncSynced; got != 1 {
		t.Fatalf("expected the routed event to reach the Sync view (syncSynced=1), got %d", got)
	}
	// The active tab must not have changed as a side effect of routing.
	if m.active != tabStatus {
		t.Fatalf("expected active tab to stay Status, got %v", m.active)
	}
}

// TestRoutedAskChunkReachesOwningView confirms a streamed Ask token lands on the
// Ask view while another tab is active.
func TestRoutedAskChunkReachesOwningView(t *testing.T) {
	m := newRootModel(&appContext{}, false)

	av := m.views[tabAsk].(*askView)
	av.gen = 1
	av.streaming = true
	m.active = tabRepos

	updated, _ := m.Update(askChunkMsg{gen: 1, chunk: "hello"})
	m = updated.(rootModel)

	if got := m.views[tabAsk].(*askView).answer; !strings.Contains(got, "hello") {
		t.Fatalf("expected the routed token to reach the Ask view, got answer=%q", got)
	}
}

// TestNonRoutedMsgGoesToActiveView confirms ordinary (non-streaming) messages
// still delegate to the active view, leaving the routing path untouched.
func TestNonRoutedMsgGoesToActiveView(t *testing.T) {
	m := newRootModel(&appContext{}, false)
	m.active = tabSync

	// A plain key on the (idle) Sync view starts a run; this only happens if the
	// key was delegated to the active view rather than swallowed by routing.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(rootModel)

	if !m.views[tabSync].(*syncView).running {
		t.Fatal("expected the active Sync view to handle the key and start running")
	}
	if cmd == nil {
		t.Fatal("expected a command from the started run")
	}
}
