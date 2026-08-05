package tui

import (
	"context"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/goleak"
)

// streamTestMsg is a trivial payload for exercising the stream helper.
type streamTestMsg struct{ n int }

// TestStreamMessagesCancelUnblocksProducer proves the leak fix has teeth. The
// producer streams far more messages than the channel buffers while the consumer
// reads only the first, exactly what happens when the user quits or supersedes a
// stream mid-flight. Without ctx.Done() racing each send the goroutine would park
// forever on the full channel; cancelling must let it drain and return. Remove
// the ctx.Done() case from streamMessages and this test hangs, then goleak fails.
func TestStreamMessagesCancelUnblocksProducer(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan tea.Msg, 2)
	var done sync.WaitGroup
	done.Add(1)
	first := streamMessages(ctx, ch, func(send func(tea.Msg)) {
		defer done.Done()
		// Far more than the 2-slot buffer: once it fills and the consumer stops
		// reading, each send blocks until ctx is cancelled.
		for i := 0; i < 1000; i++ {
			send(streamTestMsg{n: i})
		}
	})
	if first == nil {
		t.Fatal("expected the first streamed message, got nil")
	}

	// Abandon the consumer (ch is never drained again) and cancel, as quitting
	// the TUI does. The producer must observe ctx.Done() and return.
	cancel()
	done.Wait()
}

// TestStreamMessagesDeliversInOrder is the happy path: the first message is
// returned inline and the rest drain from the channel in order.
func TestStreamMessagesDeliversInOrder(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan tea.Msg, 8)
	first := streamMessages(ctx, ch, func(send func(tea.Msg)) {
		for i := 0; i < 3; i++ {
			send(streamTestMsg{n: i})
		}
	})

	got := []tea.Msg{first, <-ch, <-ch}
	for i, m := range got {
		tm, ok := m.(streamTestMsg)
		if !ok || tm.n != i {
			t.Fatalf("message %d = %#v, want streamTestMsg{n:%d}", i, m, i)
		}
	}
}

// TestWaitForStreamMsgCancel confirms the reader command unblocks with a nil
// message when the stream is cancelled, so the goroutine Bubbletea runs it on
// doesn't leak waiting on a channel the producer has abandoned.
func TestWaitForStreamMsgCancel(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan tea.Msg) // unbuffered, never fed

	cmd := waitForStreamMsg(ctx, ch)
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

// TestAskViewCancelStreamCancelsContext confirms the view's cancelStream cancels
// the in-flight stream's context and resets the fields.
func TestAskViewCancelStreamCancelsContext(t *testing.T) {
	v := newAskView(&appContext{})
	ctx, cancel := context.WithCancel(context.Background())
	v.streamCtx = ctx
	v.cancel = cancel

	v.cancelStream()

	if ctx.Err() == nil {
		t.Fatal("expected the stream context to be cancelled")
	}
	if v.cancel != nil || v.streamCtx != nil {
		t.Fatal("expected cancel and streamCtx to be cleared")
	}
}

// TestRootModelCtrlCCancelsStreams confirms quitting cancels an in-flight stream
// so its goroutine and HTTP request tear down instead of leaking on exit.
func TestRootModelCtrlCCancelsStreams(t *testing.T) {
	av := newAskView(&appContext{})
	ctx, cancel := context.WithCancel(context.Background())
	av.streamCtx = ctx
	av.cancel = cancel

	m := rootModel{
		app:    &appContext{},
		views:  map[tab]view{tabAsk: av},
		active: tabAsk,
	}

	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Fatal("expected ctrl+c to return a quit command")
	}
	if ctx.Err() == nil {
		t.Fatal("expected ctrl+c to cancel the in-flight stream")
	}
}
