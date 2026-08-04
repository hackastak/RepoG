package embed

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs every test in this package under goleak, which fails the run if
// any goroutine outlives the tests. This package launches producer goroutines
// that write to channels; the check guards against one being stranded (e.g. an
// abandoned consumer that never drains the channel) on a future change.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
