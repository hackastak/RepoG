package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// withFastRetries shrinks the backoff so tests don't sleep real seconds, and
// restores the defaults afterward.
func withFastRetries(t *testing.T) {
	t.Helper()
	origBase, origMax := retryBaseDelay, retryMaxDelay
	retryBaseDelay = time.Millisecond
	retryMaxDelay = 5 * time.Millisecond
	t.Cleanup(func() {
		retryBaseDelay, retryMaxDelay = origBase, origMax
	})
}

func newPostRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	return req
}

func TestDoWithRetry_RetriesThenSucceeds(t *testing.T) {
	withFastRetries(t)

	var calls int
	bodies := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		buf := make([]byte, 32)
		n, _ := r.Body.Read(buf)
		bodies = append(bodies, string(buf[:n]))
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), newPostRequest(t, server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if calls != 3 {
		t.Errorf("server hit %d times, want 3 (two 429s then success)", calls)
	}
	// The body must be re-sent intact on every retry, not consumed once.
	for i, b := range bodies {
		if b != "payload" {
			t.Errorf("attempt %d received body %q, want %q", i, b, "payload")
		}
	}
}

func TestDoWithRetry_ExhaustsAndReturnsLastResponse(t *testing.T) {
	withFastRetries(t)

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), newPostRequest(t, server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503", resp.StatusCode)
	}
	// 1 initial attempt + retryMaxAttempts retries.
	if want := retryMaxAttempts + 1; calls != want {
		t.Errorf("server hit %d times, want %d", calls, want)
	}
}

func TestDoWithRetry_DoesNotRetryNonRetryableStatus(t *testing.T) {
	withFastRetries(t)

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized) // 401: caller's fault, never retried
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), newPostRequest(t, server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("server hit %d times, want 1 (401 is not retryable)", calls)
	}
}

func TestDoWithRetry_StopsOnContextCancel(t *testing.T) {
	// Leave the default (longer) delay so the context cancels mid-wait.
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader("x"))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	// Cancel almost immediately so the first backoff wait is interrupted.
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	resp, derr := DoWithRetry(ctx, server.Client(), req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if derr == nil {
		t.Fatal("expected a context error, got nil")
	}
	if calls > 2 {
		t.Errorf("server hit %d times; cancellation should have stopped the retries early", calls)
	}
}

func TestRetryableStatus(t *testing.T) {
	retryable := []int{429, 500, 502, 503, 504}
	for _, code := range retryable {
		if !retryableStatus(code) {
			t.Errorf("retryableStatus(%d) = false, want true", code)
		}
	}
	notRetryable := []int{200, 201, 400, 401, 403, 404, 422}
	for _, code := range notRetryable {
		if retryableStatus(code) {
			t.Errorf("retryableStatus(%d) = true, want false", code)
		}
	}
}

func TestRetryAfterHeader(t *testing.T) {
	h := http.Header{}
	if got := retryAfter(h); got != 0 {
		t.Errorf("missing header: got %v, want 0", got)
	}
	h.Set("Retry-After", "2")
	if got := retryAfter(h); got != 2*time.Second {
		t.Errorf("seconds form: got %v, want 2s", got)
	}
	h.Set("Retry-After", "garbage")
	if got := retryAfter(h); got != 0 {
		t.Errorf("garbage: got %v, want 0", got)
	}
}
