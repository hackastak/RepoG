package tui

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs the TUI tests under goleak so a streaming view that strands its
// producer goroutine (the bug streamMessages fixes) fails the suite instead of
// leaking silently. These tests drive Update/View and the stream helpers
// directly; none construct a tea.Program, so there are no renderer goroutines to
// account for.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
