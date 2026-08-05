package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// streamMessages runs produce on a background goroutine, delivering each message
// it emits onto ch, and returns the first message so the calling tea.Cmd has
// something to hand Bubbletea immediately.
//
// Every send races ctx.Done(). That is the whole point: the answer/summary
// streams token-by-token into a buffered channel, and if the consumer stops
// draining — the user quit, or superseded the request with a new one — a plain
// `ch <- msg` would block forever once the buffer fills, stranding the goroutine
// and holding the underlying HTTP request open until its client timeout. Racing
// ctx.Done() lets a cancelled stream unblock and exit instead.
//
// Cancellation is delivered by the owning view's cancel func (see streamCanceler);
// ctx is also passed into the streaming call itself, so cancelling aborts the
// in-flight HTTP request rather than just detaching from it.
func streamMessages(ctx context.Context, ch chan tea.Msg, produce func(send func(tea.Msg))) tea.Msg {
	go func() {
		produce(func(msg tea.Msg) {
			select {
			case ch <- msg:
			case <-ctx.Done():
			}
		})
	}()

	select {
	case msg := <-ch:
		return msg
	case <-ctx.Done():
		return nil
	}
}

// waitForStreamMsg returns a command that delivers the next message from ch. It
// races ctx.Done() so the reader goroutine Bubbletea spawns for it unblocks
// (returning a nil, no-op message) when the stream is cancelled, rather than
// parking on a channel the producer has stopped feeding.
func waitForStreamMsg(ctx context.Context, ch chan tea.Msg) tea.Cmd {
	if ch == nil || ctx == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case msg := <-ch:
			return msg
		case <-ctx.Done():
			return nil
		}
	}
}

// streamCanceler is implemented by views that run a background streaming
// goroutine. The root model calls cancelStream on every view when the app quits
// (and when the view set is rebuilt), so an in-flight stream tears down instead
// of leaking its goroutine and HTTP request.
type streamCanceler interface {
	cancelStream()
}
