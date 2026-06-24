package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackastak/repog/internal/provider"
)

func TestNewAnthropicLLMProvider(t *testing.T) {
	t.Run("success with explicit model and fallback", func(t *testing.T) {
		p, err := NewAnthropicLLMProvider("key", "my-model", "my-fallback")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.apiKey != "key" {
			t.Errorf("apiKey = %q, want %q", p.apiKey, "key")
		}
		if p.model != "my-model" {
			t.Errorf("model = %q, want %q", p.model, "my-model")
		}
		if p.fallbackModel != "my-fallback" {
			t.Errorf("fallbackModel = %q, want %q", p.fallbackModel, "my-fallback")
		}
		if p.baseURL != defaultBaseURL {
			t.Errorf("baseURL = %q, want %q", p.baseURL, defaultBaseURL)
		}
	})

	t.Run("empty apiKey returns error", func(t *testing.T) {
		p, err := NewAnthropicLLMProvider("", "m", "f")
		if err == nil {
			t.Fatal("expected error for empty apiKey, got nil")
		}
		if p != nil {
			t.Errorf("expected nil provider, got %+v", p)
		}
	})

	t.Run("defaults filled when model and fallback empty", func(t *testing.T) {
		p, err := NewAnthropicLLMProvider("key", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.model != "claude-3-5-haiku-20241022" {
			t.Errorf("default model = %q, want claude-3-5-haiku-20241022", p.model)
		}
		if p.fallbackModel != "claude-3-5-sonnet-20241022" {
			t.Errorf("default fallback = %q, want claude-3-5-sonnet-20241022", p.fallbackModel)
		}
	})
}

func TestName(t *testing.T) {
	p, _ := NewAnthropicLLMProvider("key", "m", "f")
	if got := p.Name(); got != "anthropic" {
		t.Errorf("Name() = %q, want anthropic", got)
	}
}

func newProviderWithURL(t *testing.T, url, model, fallback string) *AnthropicLLMProvider {
	t.Helper()
	if model == "" {
		model = "primary-model"
	}
	if fallback == "" {
		fallback = "fallback-model"
	}
	p, err := NewAnthropicLLMProvider("test-key", model, fallback)
	if err != nil {
		t.Fatalf("failed to construct provider: %v", err)
	}
	p.baseURL = url
	return p
}

func TestCall_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/messages") {
			t.Errorf("path = %q, want suffix /messages", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q, want test-key", got)
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
			t.Errorf("anthropic-version = %q, want %q", got, anthropicVersion)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "text", "text": "Hello "},
				{"type": "tool_use", "text": "ignored"},
				{"type": "text", "text": "world"}
			],
			"usage": {"input_tokens": 11, "output_tokens": 22}
		}`))
	}))
	defer srv.Close()

	p := newProviderWithURL(t, srv.URL, "", "")
	res, errLLM := p.Call(context.Background(), provider.LLMRequest{Prompt: "hi"})
	if errLLM != nil {
		t.Fatalf("unexpected error: %+v", errLLM)
	}
	if res.Text != "Hello world" {
		t.Errorf("Text = %q, want %q", res.Text, "Hello world")
	}
	if res.InputTokens != 11 {
		t.Errorf("InputTokens = %d, want 11", res.InputTokens)
	}
	if res.OutputTokens != 22 {
		t.Errorf("OutputTokens = %d, want 22", res.OutputTokens)
	}
}

func TestCall_Non200Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 500 is not 404/400, so no fallback - returns immediately
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "boom"}`))
	}))
	defer srv.Close()

	p := newProviderWithURL(t, srv.URL, "", "")
	res, errLLM := p.Call(context.Background(), provider.LLMRequest{Prompt: "hi"})
	if errLLM == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
	if errLLM.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", errLLM.StatusCode)
	}
	if !strings.Contains(errLLM.Message, "boom") {
		t.Errorf("Message = %q, want it to contain boom", errLLM.Message)
	}
}

func TestCall_ModelFallback(t *testing.T) {
	var calls int
	var modelsSeen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// First call (primary model) returns 400 -> triggers fallback.
		if calls == 1 {
			modelsSeen = append(modelsSeen, "primary")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "model not found"}`))
			return
		}
		modelsSeen = append(modelsSeen, "fallback")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"content": [{"type": "text", "text": "fallback ok"}],
			"usage": {"input_tokens": 1, "output_tokens": 2}
		}`))
	}))
	defer srv.Close()

	p := newProviderWithURL(t, srv.URL, "primary-model", "fallback-model")
	res, errLLM := p.Call(context.Background(), provider.LLMRequest{
		Prompt:       "hi",
		SystemPrompt: "be brief",
	})
	if errLLM != nil {
		t.Fatalf("unexpected error: %+v", errLLM)
	}
	if res.Text != "fallback ok" {
		t.Errorf("Text = %q, want %q", res.Text, "fallback ok")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (primary + fallback)", calls)
	}
}

func TestCall_AllModelsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad"}`))
	}))
	defer srv.Close()

	p := newProviderWithURL(t, srv.URL, "", "")
	res, errLLM := p.Call(context.Background(), provider.LLMRequest{Prompt: "hi"})
	if errLLM == nil {
		t.Fatal("expected error after both models fail, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
	if errLLM.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", errLLM.StatusCode)
	}
}

func TestValidate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content": [{"type": "text", "text": "ok"}], "usage": {"input_tokens": 1, "output_tokens": 1}}`))
	}))
	defer srv.Close()

	p := newProviderWithURL(t, srv.URL, "", "")
	if err := p.Validate(context.Background()); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "down"}`))
	}))
	defer srv.Close()

	p := newProviderWithURL(t, srv.URL, "", "")
	err := p.Validate(context.Background())
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("error = %q, want it to contain 'validation failed'", err.Error())
	}
}

func TestStream_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Anthropic-style SSE: event lines plus "data: " lines.
		// The parser only reads "data: " lines and unmarshals streamEvent.
		sse := strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"usage":{"input_tokens":7,"output_tokens":0}}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":", world"}}`,
			``,
			`event: ping`,
			`data: {"type":"content_block_delta","delta":{"type":"not_text","text":"skipme"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","usage":{"input_tokens":0,"output_tokens":13}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	p := newProviderWithURL(t, srv.URL, "", "")

	var accumulated strings.Builder
	res, errLLM := p.Stream(context.Background(), provider.LLMRequest{
		Prompt:       "hi",
		SystemPrompt: "sys",
	}, func(chunk string) {
		accumulated.WriteString(chunk)
	})
	if errLLM != nil {
		t.Fatalf("unexpected error: %+v", errLLM)
	}
	if accumulated.String() != "Hello, world" {
		t.Errorf("onChunk accumulated = %q, want %q", accumulated.String(), "Hello, world")
	}
	if res.Text != "Hello, world" {
		t.Errorf("Text = %q, want %q", res.Text, "Hello, world")
	}
	if res.InputTokens != 7 {
		t.Errorf("InputTokens = %d, want 7", res.InputTokens)
	}
	if res.OutputTokens != 13 {
		t.Errorf("OutputTokens = %d, want 13", res.OutputTokens)
	}
}

func TestStream_Non200Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "stream down"}`))
	}))
	defer srv.Close()

	p := newProviderWithURL(t, srv.URL, "", "")
	res, errLLM := p.Stream(context.Background(), provider.LLMRequest{Prompt: "hi"}, func(string) {})
	if errLLM == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
	if errLLM.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", errLLM.StatusCode)
	}
}

func TestStream_ModelFallback(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "no such model"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"fb\"}}\n\n"))
	}))
	defer srv.Close()

	p := newProviderWithURL(t, srv.URL, "primary-model", "fallback-model")
	var acc strings.Builder
	res, errLLM := p.Stream(context.Background(), provider.LLMRequest{Prompt: "hi"}, func(c string) {
		acc.WriteString(c)
	})
	if errLLM != nil {
		t.Fatalf("unexpected error: %+v", errLLM)
	}
	if res.Text != "fb" {
		t.Errorf("Text = %q, want fb", res.Text)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}
