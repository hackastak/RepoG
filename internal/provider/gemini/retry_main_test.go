package gemini

import (
	"os"
	"testing"
	"time"

	"github.com/hackastak/repog/internal/provider"
)

// TestMain shrinks the shared retry backoff so the suites that return 5xx/429
// from httptest servers exercise the retry path without sleeping real seconds.
func TestMain(m *testing.M) {
	restore := provider.SetRetryPolicyForTest(time.Millisecond, 5*time.Millisecond)
	code := m.Run()
	restore()
	os.Exit(code)
}
