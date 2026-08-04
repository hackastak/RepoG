package provider

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// Retry policy for provider HTTP calls. These are package variables rather than
// constants so tests can shrink the delays without waiting real seconds.
var (
	retryMaxAttempts = 3
	retryBaseDelay   = 500 * time.Millisecond
	retryMaxDelay    = 30 * time.Second
)

// SetRetryPolicyForTest overrides the retry backoff for a test run and returns a
// function that restores the previous values. It lets provider test suites — which
// point httptest servers at 5xx/429 responses — exercise the retry path without
// sleeping real seconds. Test-only; production code never calls it.
func SetRetryPolicyForTest(base, max time.Duration) func() {
	prevBase, prevMax := retryBaseDelay, retryMaxDelay
	retryBaseDelay, retryMaxDelay = base, max
	return func() { retryBaseDelay, retryMaxDelay = prevBase, prevMax }
}

// retryableStatus reports whether an HTTP status represents a transient failure
// worth retrying: rate limiting (429) or a server-side error (5xx). A 4xx other
// than 429 is the caller's fault (bad key, bad request) and never retried.
func retryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// DoWithRetry sends req and retries transient failures — network errors and
// 429/5xx responses — with exponential backoff plus jitter, up to a small fixed
// number of attempts. It honors a Retry-After header when the server sends one,
// and it aborts early if ctx is cancelled.
//
// The AI providers previously had no retry at all (unlike the GitHub client),
// so a single 429 permanently failed a batch of embeddings mid-run. Routing
// every provider's request through this helper gives them all the same
// resilience from one place.
//
// The request body is restored from req.GetBody before each retry, since the
// prior attempt consumed it. http.NewRequestWithContext sets GetBody
// automatically for the *bytes.Reader / *bytes.Buffer / *strings.Reader bodies
// these providers build, so callers need no extra wiring.
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; ; attempt++ {
		// Rewind the body for retries; the previous Do consumed it.
		if attempt > 0 && req.GetBody != nil {
			body, gerr := req.GetBody()
			if gerr != nil {
				return resp, err
			}
			req.Body = body
		}

		resp, err = client.Do(req)

		// A transport error short-circuits the status check (resp is nil).
		if err == nil && !retryableStatus(resp.StatusCode) {
			return resp, nil
		}

		// Out of attempts, or the failure is a cancellation rather than a
		// transient server condition: hand the last result back to the caller.
		if attempt >= retryMaxAttempts || ctx.Err() != nil {
			return resp, err
		}

		delay := backoffDelay(attempt)
		if resp != nil {
			if ra := retryAfter(resp.Header); ra > 0 {
				delay = ra
			}
			// Drain and close so the connection returns to the pool.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
}

// backoffDelay returns an exponentially growing, jittered delay for the given
// zero-based attempt, capped at retryMaxDelay. Full jitter (a random point in
// the lower half of the window) spreads retries out so concurrent callers don't
// all wake at the same instant.
func backoffDelay(attempt int) time.Duration {
	d := retryBaseDelay << attempt
	if d <= 0 || d > retryMaxDelay {
		d = retryMaxDelay
	}
	half := d / 2
	return half + time.Duration(rand.Int63n(int64(half)+1))
}

// retryAfter parses a Retry-After header, which may be either a number of
// seconds or an HTTP date. It returns 0 when the header is absent or unusable.
func retryAfter(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
