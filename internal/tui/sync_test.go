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

// TestSyncViewSuspendKeepsResumePoint confirms "p" halts a running sync: it
// cancels the run context (a hard teardown, same as quit), bumps gen so in-flight
// events are discarded, and records what to resume — without zeroing the counters.
func TestSyncViewSuspendKeepsResumePoint(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseSyncing
	v.chain = true
	v.gen = 1
	v.syncSynced = 3
	ctx, cancel := context.WithCancel(context.Background())
	v.streamCtx = ctx
	v.cancel = cancel

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	v = updated.(*syncView)

	if v.running || !v.paused {
		t.Fatalf("expected paused (not running), got running=%v paused=%v", v.running, v.paused)
	}
	if v.pausedPhase != phaseSyncing || !v.pausedChain {
		t.Fatalf("expected resume point syncing+chain, got phase=%v chain=%v", v.pausedPhase, v.pausedChain)
	}
	if ctx.Err() == nil {
		t.Fatal("expected suspend to cancel the run context")
	}
	if v.gen != 2 {
		t.Fatalf("expected gen to advance on suspend, got %d", v.gen)
	}
	if v.syncSynced != 3 {
		t.Fatalf("expected counters to be preserved across suspend, got %d", v.syncSynced)
	}
	if out := v.View(80, 24); !strings.Contains(out, "Paused") {
		t.Fatalf("expected paused status in output, got:\n%s", out)
	}
}

// TestSyncViewSuspendSnapshotsEmbedBaseline confirms suspending an embed run
// snapshots the cumulative counts as the baseline the resumed run adds onto.
func TestSyncViewSuspendSnapshotsEmbedBaseline(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseEmbedding
	v.gen = 1
	v.embedded, v.embErrored, v.embSkipped, v.embTotal = 12, 1, 4, 40
	ctx, cancel := context.WithCancel(context.Background())
	v.streamCtx = ctx
	v.cancel = cancel

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	v = updated.(*syncView)

	if v.baseEmbedded != 12 || v.baseEmbErrored != 1 || v.baseEmbSkipped != 4 || v.baseEmbTotal != 40 {
		t.Fatalf("unexpected embed baseline: %+v",
			[]int{v.baseEmbedded, v.baseEmbErrored, v.baseEmbSkipped, v.baseEmbTotal})
	}
	if v.pausedPhase != phaseEmbedding {
		t.Fatalf("expected resume point embedding, got %v", v.pausedPhase)
	}
}

// TestSyncViewResumeCarriesEmbedCounts confirms "r" continues a suspended embed:
// the resumed pipeline re-reports only the remaining chunks, and applyEmbedCounts
// adds them onto the pre-suspend baseline while holding the total steady.
func TestSyncViewResumeCarriesEmbedCounts(t *testing.T) {
	v := newSyncView(&appContext{})
	// Simulate a run suspended after 12/40 chunks.
	v.paused = true
	v.pausedPhase = phaseEmbedding
	v.gen = 1
	v.embedded, v.embErrored, v.embSkipped, v.embTotal = 12, 0, 0, 40
	v.baseEmbedded, v.baseEmbErrored, v.baseEmbSkipped, v.baseEmbTotal = 12, 0, 0, 40

	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	v = updated.(*syncView)
	if !v.running || v.phase != phaseEmbedding {
		t.Fatalf("expected resumed embedding run, got running=%v phase=%v", v.running, v.phase)
	}
	if !v.embCarrying {
		t.Fatal("expected embCarrying to be set on a resumed embed")
	}
	if cmd == nil {
		t.Fatal("expected a command to relaunch the embed pipeline")
	}

	// The resumed pipeline reports 8 more chunks embedded (of the 28 remaining),
	// numbered from its own zero. Displayed progress must read 20/40, not 8/28.
	updated, _ = v.Update(embedEventMsg{gen: v.gen, ev: embed.EmbedEvent{
		Type: "batch", BatchIndex: 1, BatchTotal: 2, ChunksEmbedded: 8, TotalChunks: 28,
	}})
	v = updated.(*syncView)
	if v.embedded != 20 || v.embTotal != 40 {
		t.Fatalf("expected carried 20/40, got %d/%d", v.embedded, v.embTotal)
	}

	// Final event of the resumed run finishes the remaining 28.
	updated, _ = v.Update(embedEventMsg{gen: v.gen, ev: embed.EmbedEvent{
		Type: "done", ChunksEmbedded: 28, TotalChunks: 28,
	}})
	v = updated.(*syncView)
	if v.embedded != 40 {
		t.Fatalf("expected 40 embedded after resume completes, got %d", v.embedded)
	}
	if v.embCarrying || v.paused || v.running {
		t.Fatalf("expected a clean finish, got carrying=%v paused=%v running=%v",
			v.embCarrying, v.paused, v.running)
	}
	if out := v.View(80, 24); !strings.Contains(out, "40 embedded") {
		t.Fatalf("expected banner to show carried total, got:\n%s", out)
	}
}

// TestSyncViewResumeDedupesSyncedRepos confirms a resumed sync doesn't
// double-count repos it already tallied before the suspend: the re-run re-emits
// them (as repo/skip events) and markSeen filters them out.
func TestSyncViewResumeDedupesSyncedRepos(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseSyncing
	v.gen = 1

	feed := func(gen int, ev sync.IngestEvent) {
		updated, _ := v.Update(syncEventMsg{gen: gen, ev: ev})
		v = updated.(*syncView)
	}
	feed(1, sync.IngestEvent{Type: "repo", Repo: "me/alpha", Status: "new"})
	feed(1, sync.IngestEvent{Type: "repo", Repo: "me/beta", Status: "new"})
	if v.syncSynced != 2 {
		t.Fatalf("expected 2 synced before suspend, got %d", v.syncSynced)
	}

	// Suspend then resume.
	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	v = updated.(*syncView)
	updated, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	v = updated.(*syncView)
	if !v.running || v.phase != phaseSyncing {
		t.Fatalf("expected resumed syncing run, got running=%v phase=%v", v.running, v.phase)
	}

	// The re-run re-emits the two already-synced repos, then a fresh one.
	feed(v.gen, sync.IngestEvent{Type: "repo", Repo: "me/alpha", Status: "new"})
	feed(v.gen, sync.IngestEvent{Type: "skip", Repo: "me/beta", Reason: "unchanged"})
	feed(v.gen, sync.IngestEvent{Type: "repo", Repo: "me/gamma", Status: "new"})

	if v.syncSynced != 3 {
		t.Fatalf("expected 3 synced after dedup (alpha, beta, gamma), got %d", v.syncSynced)
	}
	if v.syncSkipped != 0 {
		t.Fatalf("expected already-seen beta not to be counted as a skip, got %d", v.syncSkipped)
	}
}

// TestSyncViewCancelOpReturnsToIdle confirms "c" abandons a suspended run: the
// resume point is dropped and the view goes idle.
func TestSyncViewCancelOpReturnsToIdle(t *testing.T) {
	v := newSyncView(&appContext{})
	v.paused = true
	v.pausedPhase = phaseEmbedding
	v.embCarrying = true
	v.phase = phaseEmbedding
	v.gen = 1

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	v = updated.(*syncView)

	if v.running || v.paused || v.phase != phaseIdle {
		t.Fatalf("expected idle after cancel, got running=%v paused=%v phase=%v",
			v.running, v.paused, v.phase)
	}
	if v.embCarrying || v.resuming {
		t.Fatalf("expected resume state cleared, got carrying=%v resuming=%v", v.embCarrying, v.resuming)
	}
	if v.gen != 2 {
		t.Fatalf("expected gen to advance on cancel, got %d", v.gen)
	}
	if out := v.View(80, 24); !strings.Contains(out, "Cancelled") {
		t.Fatalf("expected cancel log entry, got:\n%s", out)
	}
}

// TestSyncViewTriggersInertWhilePaused confirms s/e/a can't start a new run over a
// suspended one — only r (resume) and c (cancel) act.
func TestSyncViewTriggersInertWhilePaused(t *testing.T) {
	v := newSyncView(&appContext{})
	v.paused = true
	v.pausedPhase = phaseSyncing
	v.gen = 1

	for _, key := range []rune{'s', 'e', 'a'} {
		updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		v = updated.(*syncView)
		if v.running || v.gen != 1 {
			t.Fatalf("expected %q to be inert while paused, got running=%v gen=%d", key, v.running, v.gen)
		}
	}
}

// TestSyncViewLateEventAfterSuspendDiscarded confirms an event already in flight
// from the stopped run is dropped once suspend bumps the generation.
func TestSyncViewLateEventAfterSuspendDiscarded(t *testing.T) {
	v := newSyncView(&appContext{})
	v.running = true
	v.phase = phaseSyncing
	v.gen = 1
	v.syncSynced = 1
	ctx, cancel := context.WithCancel(context.Background())
	v.streamCtx = ctx
	v.cancel = cancel

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	v = updated.(*syncView)

	// An event stamped with the pre-suspend generation arrives late; it must be
	// ignored so the paused counts stay put.
	updated, _ = v.Update(syncEventMsg{gen: 1, ev: sync.IngestEvent{Type: "repo", Repo: "me/late"}})
	v = updated.(*syncView)
	if v.syncSynced != 1 {
		t.Fatalf("expected late stale event to be ignored, got syncSynced=%d", v.syncSynced)
	}
}

// errTest is a small sentinel for the precondition-error path.
var errTest = errTestErr("kaboom")

type errTestErr string

func (e errTestErr) Error() string { return string(e) }
