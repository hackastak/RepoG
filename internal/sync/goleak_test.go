package sync

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs every test in this package under goleak, which fails the run if
// a goroutine outlives the tests, e.g. a producer stranded on a channel send
// after its consumer was abandoned.
//
// The two net/http ignores cover connection-pool goroutines (readLoop/writeLoop)
// that the shared DefaultTransport keeps alive after the httptest servers close;
// they sit idle until the 90s IdleConnTimeout, well past goleak's retry window.
// They are test-infrastructure artifacts, not a leak in this package's code.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
	)
}
