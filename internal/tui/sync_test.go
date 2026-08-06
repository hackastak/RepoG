package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hackastak/repog/internal/embed"
	"github.com/hackastak/repog/internal/sync"
)

// TestSyncViewIdleRender shows the trigger prompt before any operation runs.
func TestSyncViewIdleRender(t *testing.T) {
	v := newSyncView(&appContext{})
	out := v.View(80, 24)
	for _, want := range []string{"Sync / Embed", "Press s to sync"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected idle output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestSyncViewStartSync verifies the "s" key starts a run: it enters the syncing
// phase, bumps the generation, and returns a command (the precondition check).
func TestSyncViewStartSync(t *testing.T) {
	v := newSyncView(&appContext{})

	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	sv := updated.(*syncView)

	if !sv.running || sv.phase != phaseSyncing {
		t.Fatalf("expected running syncing phase, got running=%v phase=%v", sv.running, sv.phase)
	}
	if sv.gen != 1 {
		t.Fatalf("expected gen to advance to 1, got %d", sv.gen)
	}
	if cmd == nil {
		t.Fatal("expected a command to kick off the sync, got nil")
	}
}

// TestSyncViewSyncProgressAndDone feeds repo/skip/error events then a done event
// and confirms counters, log, and terminal state.
func TestSyncViewSyncProgressAndDone(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseSyncing
	v.gen = 1

	feed := func(ev sync.IngestEvent) {
		updated, _ := v.Update(syncEventMsg{gen: 1, ev: ev})
		v = updated.(*syncView)
	}
	feed(sync.IngestEvent{Type: "repo", Repo: "me/alpha", Status: "new"})
	feed(sync.IngestEvent{Type: "skip", Repo: "me/beta", Reason: "unchanged"})
	feed(sync.IngestEvent{Type: "error", Repo: "me/gamma", Reason: "boom"})

	if v.syncSynced != 1 || v.syncSkipped != 1 || v.syncErrors != 1 {
		t.Fatalf("unexpected live counters: %+v", []int{v.syncSynced, v.syncSkipped, v.syncErrors})
	}

	updated, cmd := v.Update(syncEventMsg{gen: 1, ev: sync.IngestEvent{
		Type: "done", Total: 1, Skipped: 1, Errors: 1,
	}})
	v = updated.(*syncView)

	if v.running || v.phase != phaseIdle {
		t.Fatalf("expected idle after done, got running=%v phase=%v", v.running, v.phase)
	}
	if !v.hasRun {
		t.Fatal("expected hasRun to be true after a completed run")
	}
	if cmd != nil {
		t.Fatal("expected no follow-up command for a non-chained sync")
	}

	out := v.View(80, 24)
	for _, want := range []string{"me/alpha", "Sync complete", "me/gamma"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected log to contain %q, got:\n%s", want, out)
		}
	}
	// Skips are counted but kept out of the log.
	if strings.Contains(out, "me/beta") {
		t.Fatalf("expected skipped repo to stay out of the log, got:\n%s", out)
	}
}

// TestSyncViewChainStartsEmbed confirms the "a" flow continues into embed when
// the sync completes.
func TestSyncViewChainStartsEmbed(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseSyncing
	v.chain = true
	v.gen = 1

	updated, cmd := v.Update(syncEventMsg{gen: 1, ev: sync.IngestEvent{Type: "done", Total: 2}})
	v = updated.(*syncView)

	if !v.running || v.phase != phaseEmbedding {
		t.Fatalf("expected to enter embedding phase, got running=%v phase=%v", v.running, v.phase)
	}
	if cmd == nil {
		t.Fatal("expected a command to launch the chained embed, got nil")
	}
	if v.gen != 2 {
		t.Fatalf("expected gen to advance for the embed run, got %d", v.gen)
	}
}

// TestSyncViewEmbedDone confirms the embed pipeline's terminal event finalizes
// the view and renders its summary.
func TestSyncViewEmbedDone(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseEmbedding
	v.gen = 1

	updated, _ := v.Update(embedEventMsg{gen: 1, ev: embed.EmbedEvent{
		Type: "batch", BatchIndex: 1, BatchTotal: 2, ChunksEmbedded: 10, TotalChunks: 20,
	}})
	v = updated.(*syncView)
	if v.embBatchTotal != 2 || v.embedded != 10 {
		t.Fatalf("expected batch counters to update, got batchTotal=%d embedded=%d", v.embBatchTotal, v.embedded)
	}

	updated, _ = v.Update(embedEventMsg{gen: 1, ev: embed.EmbedEvent{
		Type: "done", ChunksEmbedded: 20, TotalChunks: 20,
	}})
	v = updated.(*syncView)

	if v.running || v.phase != phaseIdle {
		t.Fatalf("expected idle after embed done, got running=%v phase=%v", v.running, v.phase)
	}
	if out := v.View(80, 24); !strings.Contains(out, "Embedding complete") {
		t.Fatalf("expected embed summary in output, got:\n%s", out)
	}
}

// TestSyncViewStaleEventDiscarded ensures events from a superseded run (older
// generation) are ignored.
func TestSyncViewStaleEventDiscarded(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseSyncing
	v.gen = 2

	updated, _ := v.Update(syncEventMsg{gen: 1, ev: sync.IngestEvent{Type: "repo", Repo: "x/y"}})
	v = updated.(*syncView)

	if v.syncSynced != 0 {
		t.Fatalf("expected stale event to be ignored, got syncSynced=%d", v.syncSynced)
	}
}

// TestSyncViewKeysInertWhileRunning confirms trigger keys are ignored mid-run so
// a second operation can't clobber the active one.
func TestSyncViewKeysInertWhileRunning(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseSyncing
	v.gen = 1

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	sv := updated.(*syncView)

	if sv.gen != 1 || sv.phase != phaseSyncing {
		t.Fatalf("expected the run to be untouched, got gen=%d phase=%v", sv.gen, sv.phase)
	}
}

// TestSyncViewPreconditionError surfaces an op error in the status line and log
// and returns the view to idle.
func TestSyncViewPreconditionError(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseSyncing
	v.gen = 1

	updated, _ := v.Update(opErrMsg{gen: 1, err: errTest})
	v = updated.(*syncView)

	if v.running {
		t.Fatal("expected running to be false after an op error")
	}
	if out := v.View(80, 24); !strings.Contains(out, "kaboom") {
		t.Fatalf("expected the error in output, got:\n%s", out)
	}
}

// TestSyncViewCancelStreamCancelsContext confirms the view's cancelStream cancels
// the in-flight run's context and clears its fields, so quitting mid-sync tears
// the producer goroutine (and its HTTP requests / open transaction) down.
func TestSyncViewCancelStreamCancelsContext(t *testing.T) {
	v := newSyncView(&appContext{})
	ctx, cancel := context.WithCancel(context.Background())
	v.streamCtx = ctx
	v.cancel = cancel

	v.cancelStream()

	if ctx.Err() == nil {
		t.Fatal("expected the run context to be cancelled")
	}
	if v.cancel != nil || v.streamCtx != nil {
		t.Fatal("expected cancel and streamCtx to be cleared")
	}
}

// TestWaitForSyncEventCancel proves the reader unblocks on cancel instead of
// parking forever on a channel the cancelled producer has stopped feeding. Drop
// the ctx.Done() case from waitForSyncEvent and this hangs, then goleak fails.
func TestWaitForSyncEventCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan sync.IngestEvent) // unbuffered, never fed

	cmd := waitForSyncEvent(ctx, ch, 1)
	if cmd == nil {
		t.Fatal("expected a wait command")
	}

	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()

	cancel()
	if msg := <-got; msg != nil {
		t.Fatalf("expected nil message on cancel, got %#v", msg)
	}
}

// TestWaitForEmbedEventCancel is the embed-channel counterpart of the above.
func TestWaitForEmbedEventCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan embed.EmbedEvent) // unbuffered, never fed

	cmd := waitForEmbedEvent(ctx, ch, 1)
	if cmd == nil {
		t.Fatal("expected a wait command")
	}

	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()

	cancel()
	if msg := <-got; msg != nil {
		t.Fatalf("expected nil message on cancel, got %#v", msg)
	}
}

// TestRootModelQuitCancelsSyncStream confirms quitting (q) cancels an in-flight
// sync/embed run through the root model's cancelStreams sweep, so its producer
// goroutine doesn't outlive the program.
func TestRootModelQuitCancelsSyncStream(t *testing.T) {
	sv := newSyncView(&appContext{})
	ctx, cancel := context.WithCancel(context.Background())
	sv.streamCtx = ctx
	sv.cancel = cancel

	m := rootModel{
		app:    &appContext{},
		views:  map[tab]view{tabSync: sv},
		active: tabSync,
	}

	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd == nil {
		t.Fatal("expected q to return a quit command")
	}
	if ctx.Err() == nil {
		t.Fatal("expected quit to cancel the in-flight sync stream")
	}
}

// errTest is a small sentinel for the precondition-error path.
var errTest = errTestErr("kaboom")

type errTestErr string

func (e errTestErr) Error() string { return string(e) }
