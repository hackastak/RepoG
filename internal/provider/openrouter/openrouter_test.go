package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/provider"
)

// TestRegisteredFactories exercises the init()-registered factory callbacks via
// the provider registry, confirming the openrouter providers are wired up.
func TestRegisteredFactories(t *testing.T) {
	embProv, err := provider.NewEmbeddingProvider(config.ProviderConfig{
		Provider:   "openrouter",
		Model:      "openai/text-embedding-3-small",
		Dimensions: 1536,
	}, "key")
	if err != nil {
		t.Fatalf("unexpected error creating embedding provider: %v", err)
	}
	if embProv.Name() != "openrouter" {
		t.Errorf("unexpected embedding provider name: %q", embProv.Name())
	}

	llmProv, err := provider.NewLLMProvider(config.ProviderConfig{
		Provider: "openrouter",
		Model:    "openai/gpt-4o",
		Fallback: "openai/gpt-3.5-turbo",
	}, "key")
	if err != nil {
		t.Fatalf("unexpected error creating LLM provider: %v", err)
	}
	if llmProv.Name() != "openrouter" {
		t.Errorf("unexpected LLM provider name: %q", llmProv.Name())
	}
}

// makeEmbedding returns a deterministic float32 slice of length dim.
func makeEmbedding(dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = float32(i) * 0.001
	}
	return v
}

// ---------------------------------------------------------------------------
// LLM provider tests
// ---------------------------------------------------------------------------

func TestNewOpenRouterLLMProvider(t *testing.T) {
	// Error path: empty API key
	if _, err := NewOpenRouterLLMProvider("", "m", "f"); err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}

	// Defaults filled in for empty model/fallback
	p, err := NewOpenRouterLLMProvider("key", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.model != "openai/gpt-4o" {
		t.Errorf("expected default model openai/gpt-4o, got %q", p.model)
	}
	if p.fallbackModel != "openai/gpt-3.5-turbo" {
		t.Errorf("expected default fallback openai/gpt-3.5-turbo, got %q", p.fallbackModel)
	}
	if p.baseURL != defaultBaseURL {
		t.Errorf("expected default baseURL %q, got %q", defaultBaseURL, p.baseURL)
	}

	// Explicit values preserved
	p2, err := NewOpenRouterLLMProvider("key", "custom/model", "custom/fallback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p2.model != "custom/model" || p2.fallbackModel != "custom/fallback" {
		t.Errorf("explicit model/fallback not preserved: %q / %q", p2.model, p2.fallbackModel)
	}
}

func TestLLMProviderName(t *testing.T) {
	p, _ := NewOpenRouterLLMProvider("key", "m", "f")
	if got := p.Name(); got != "openrouter" {
		t.Errorf("expected name openrouter, got %q", got)
	}
}

func TestLLMCallSuccess(t *testing.T) {
	var gotAuth, gotTitle string
	var gotBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotTitle = r.Header.Get("X-Title")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		resp := chatCompletionResponse{
			Choices: []choice{{Message: message{Role: "assistant", Content: "hello world"}}},
			Usage:   usageInfo{PromptTokens: 5, CompletionTokens: 7},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("secret-key", "primary/model", "fallback/model")
	p.baseURL = server.URL

	res, llmErr := p.Call(context.Background(), provider.LLMRequest{
		Prompt:       "do something",
		SystemPrompt: "be helpful",
	})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "hello world" {
		t.Errorf("expected text 'hello world', got %q", res.Text)
	}
	if res.InputTokens != 5 || res.OutputTokens != 7 {
		t.Errorf("unexpected tokens: in=%d out=%d", res.InputTokens, res.OutputTokens)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("unexpected auth header: %q", gotAuth)
	}
	if gotTitle != "RepoG" {
		t.Errorf("unexpected title header: %q", gotTitle)
	}
	// Default MaxTokens / Temperature applied
	if gotBody.MaxTokens != 1024 {
		t.Errorf("expected default max_tokens 1024, got %d", gotBody.MaxTokens)
	}
	if gotBody.Temperature != 0.3 {
		t.Errorf("expected default temperature 0.3, got %v", gotBody.Temperature)
	}
	// System prompt + user message present
	if len(gotBody.Messages) != 2 || gotBody.Messages[0].Role != "system" || gotBody.Messages[1].Role != "user" {
		t.Errorf("unexpected messages: %+v", gotBody.Messages)
	}
	if gotBody.Model != "primary/model" {
		t.Errorf("expected primary model, got %q", gotBody.Model)
	}
}

func TestLLMCallNoSystemPrompt(t *testing.T) {
	var gotBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		resp := chatCompletionResponse{Choices: []choice{{Message: message{Content: "ok"}}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	_, llmErr := p.Call(context.Background(), provider.LLMRequest{
		Prompt:      "hi",
		MaxTokens:   42,
		Temperature: 0.9,
	})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != "user" {
		t.Errorf("expected only user message, got %+v", gotBody.Messages)
	}
	if gotBody.MaxTokens != 42 || gotBody.Temperature != 0.9 {
		t.Errorf("explicit max_tokens/temperature not preserved: %d / %v", gotBody.MaxTokens, gotBody.Temperature)
	}
}

func TestLLMCallFallback(t *testing.T) {
	var modelsSeen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		modelsSeen = append(modelsSeen, body.Model)
		if body.Model == "primary/model" {
			// Primary fails with 404 -> should trigger fallback
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"model not found"}`))
			return
		}
		resp := chatCompletionResponse{Choices: []choice{{Message: message{Content: "from fallback"}}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "primary/model", "fallback/model")
	p.baseURL = server.URL

	res, llmErr := p.Call(context.Background(), provider.LLMRequest{Prompt: "x"})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "from fallback" {
		t.Errorf("expected fallback text, got %q", res.Text)
	}
	if len(modelsSeen) != 2 || modelsSeen[0] != "primary/model" || modelsSeen[1] != "fallback/model" {
		t.Errorf("unexpected model sequence: %v", modelsSeen)
	}
}

func TestLLMCallAllModelsFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "primary/model", "fallback/model")
	p.baseURL = server.URL

	_, llmErr := p.Call(context.Background(), provider.LLMRequest{Prompt: "x"})
	if llmErr == nil {
		t.Fatal("expected error when all models fail")
	}
	if llmErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", llmErr.StatusCode)
	}
}

func TestLLMCallNon200NonFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server boom`))
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	_, llmErr := p.Call(context.Background(), provider.LLMRequest{Prompt: "x"})
	if llmErr == nil {
		t.Fatal("expected error for 500 response")
	}
	if llmErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", llmErr.StatusCode)
	}
}

func TestLLMCallBadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	_, llmErr := p.Call(context.Background(), provider.LLMRequest{Prompt: "x"})
	if llmErr == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestLLMValidateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{Choices: []choice{{Message: message{Content: "ok"}}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	if err := p.Validate(context.Background()); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
}

func TestLLMValidateFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	if err := p.Validate(context.Background()); err == nil {
		t.Fatal("expected validation failure")
	}
}

func TestLLMStreamSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Stream {
			t.Error("expected stream=true in request body")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		chunks := []streamChunk{
			{Choices: []choice{{Delta: messageDelta{Content: "Hello"}}}},
			{Choices: []choice{{Delta: messageDelta{Content: ", "}}}},
			{Choices: []choice{{Delta: messageDelta{Content: "world"}}}},
			{Choices: []choice{{Delta: messageDelta{Content: ""}}}, Usage: &usageInfo{PromptTokens: 3, CompletionTokens: 4}},
		}
		for _, c := range chunks {
			b, _ := json.Marshal(c)
			_, _ = fmt.Fprintf(w, "data: %s\n", string(b))
			if flusher != nil {
				flusher.Flush()
			}
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	var accumulated strings.Builder
	res, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "x", SystemPrompt: "sys"}, func(s string) {
		accumulated.WriteString(s)
	})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if accumulated.String() != "Hello, world" {
		t.Errorf("onChunk accumulation wrong: %q", accumulated.String())
	}
	if res.Text != "Hello, world" {
		t.Errorf("result text wrong: %q", res.Text)
	}
	if res.InputTokens != 3 || res.OutputTokens != 4 {
		t.Errorf("unexpected tokens: in=%d out=%d", res.InputTokens, res.OutputTokens)
	}
}

func TestLLMStreamNilOnChunk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(streamChunk{Choices: []choice{{Delta: messageDelta{Content: "abc"}}}})
		_, _ = fmt.Fprintf(w, "data: %s\n", string(b))
		// invalid data line should be skipped
		_, _ = fmt.Fprint(w, "data: {not-json}\n")
		// non-data line should be skipped
		_, _ = fmt.Fprint(w, "ignored line\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n")
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	res, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "x"}, nil)
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "abc" {
		t.Errorf("expected text abc, got %q", res.Text)
	}
}

func TestLLMStreamFallback(t *testing.T) {
	var modelsSeen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		modelsSeen = append(modelsSeen, body.Model)
		if body.Model == "primary/model" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
			return
		}
		b, _ := json.Marshal(streamChunk{Choices: []choice{{Delta: messageDelta{Content: "fb"}}}})
		_, _ = fmt.Fprintf(w, "data: %s\n", string(b))
		_, _ = fmt.Fprint(w, "data: [DONE]\n")
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "primary/model", "fallback/model")
	p.baseURL = server.URL

	res, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "x"}, nil)
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "fb" {
		t.Errorf("expected fallback text fb, got %q", res.Text)
	}
	if len(modelsSeen) != 2 {
		t.Errorf("expected 2 attempts, got %v", modelsSeen)
	}
}

func TestLLMStreamNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	_, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "x"}, nil)
	if llmErr == nil {
		t.Fatal("expected error for 500 stream response")
	}
	if llmErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", llmErr.StatusCode)
	}
}

func TestLLMStreamAllFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	}))
	defer server.Close()

	p, _ := NewOpenRouterLLMProvider("key", "primary/model", "fallback/model")
	p.baseURL = server.URL

	_, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "x"}, nil)
	if llmErr == nil {
		t.Fatal("expected error when all stream models fail")
	}
	if llmErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", llmErr.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Embedding provider tests
// ---------------------------------------------------------------------------

func TestGetOpenRouterModelSpec(t *testing.T) {
	spec := getOpenRouterModelSpec("openai/text-embedding-3-large")
	if spec.Dimensions != 3072 || spec.MaxTokens != 8191 {
		t.Errorf("unexpected known spec: %+v", spec)
	}
	def := getOpenRouterModelSpec("unknown/model")
	if def != defaultOpenRouterModelSpec {
		t.Errorf("expected default spec for unknown model, got %+v", def)
	}
}

func TestNewOpenRouterEmbeddingProvider(t *testing.T) {
	// Error path: empty API key
	if _, err := NewOpenRouterEmbeddingProvider("", "m", 10); err == nil {
		t.Fatal("expected error for empty API key")
	}

	// Default model + default dimensions from spec
	p, err := NewOpenRouterEmbeddingProvider("key", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.model != "openai/text-embedding-3-small" {
		t.Errorf("expected default model, got %q", p.model)
	}
	if p.dimensions != 1536 {
		t.Errorf("expected default dimensions 1536, got %d", p.dimensions)
	}
	if p.batchSize != 100 {
		t.Errorf("expected batch size 100, got %d", p.batchSize)
	}
	if p.baseURL != defaultBaseURL {
		t.Errorf("expected default baseURL, got %q", p.baseURL)
	}

	// Explicit dimensions preserved; spec lookup based on model
	p2, _ := NewOpenRouterEmbeddingProvider("key", "cohere/embed-english-v3.0", 0)
	if p2.dimensions != 1024 {
		t.Errorf("expected dimensions 1024 from model spec, got %d", p2.dimensions)
	}
	p3, _ := NewOpenRouterEmbeddingProvider("key", "custom/model", 256)
	if p3.dimensions != 256 {
		t.Errorf("expected explicit dimensions 256, got %d", p3.dimensions)
	}
}

func TestEmbeddingGetters(t *testing.T) {
	p, _ := NewOpenRouterEmbeddingProvider("key", "voyageai/voyage-3", 0)
	if p.Name() != "openrouter" {
		t.Errorf("unexpected name: %q", p.Name())
	}
	if p.Dimensions() != 1024 {
		t.Errorf("unexpected dimensions: %d", p.Dimensions())
	}
	if p.BatchSize() != 100 {
		t.Errorf("unexpected batch size: %d", p.BatchSize())
	}
	if p.MaxTokens() != 32000 {
		t.Errorf("unexpected max tokens: %d", p.MaxTokens())
	}

	// Unknown model -> default max tokens
	pUnknown, _ := NewOpenRouterEmbeddingProvider("key", "weird/model", 64)
	if pUnknown.MaxTokens() != defaultOpenRouterModelSpec.MaxTokens {
		t.Errorf("expected default max tokens, got %d", pUnknown.MaxTokens())
	}
}

func TestEmbedChunksEmpty(t *testing.T) {
	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	res := p.EmbedChunks(context.Background(), nil)
	if len(res.Results) != 0 || res.Errors != 0 {
		t.Errorf("expected empty result for no chunks, got %+v", res)
	}
}

func TestEmbedChunksSuccess(t *testing.T) {
	const dim = 8
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		resp := embeddingResponse{
			Data: []embeddingData{
				{Embedding: makeEmbedding(dim), Index: 0},
				{Embedding: makeEmbedding(dim), Index: 1},
			},
			Usage: usageInfo{TotalTokens: 2},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("secret", "m", dim)
	p.baseURL = server.URL

	chunks := []provider.EmbedRequest{
		{ID: 11, Content: "alpha"},
		{ID: 22, Content: "beta"},
	}
	res := p.EmbedChunks(context.Background(), chunks)
	if res.Errors != 0 {
		t.Fatalf("expected no errors, got %d", res.Errors)
	}
	if len(res.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res.Results))
	}
	if res.Results[0].ID != 11 || res.Results[1].ID != 22 {
		t.Errorf("IDs not mapped correctly: %+v", res.Results)
	}
	if len(res.Results[0].Embedding) != dim {
		t.Errorf("unexpected embedding length: %d", len(res.Results[0].Embedding))
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("unexpected auth header: %q", gotAuth)
	}
}

func TestEmbedChunksDimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{
			Data: []embeddingData{{Embedding: makeEmbedding(4), Index: 0}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 8) // expects 8, server returns 4
	p.baseURL = server.URL

	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{{ID: 1, Content: "x"}})
	if res.Errors != 1 {
		t.Fatalf("expected 1 error for dimension mismatch, got %d", res.Errors)
	}
	if res.Results[0].Error == nil || !strings.Contains(res.Results[0].Error.Error(), "invalid dimensions") {
		t.Errorf("expected dimension mismatch error, got %v", res.Results[0].Error)
	}
}

func TestEmbedChunksMissingData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only one embedding returned for two chunks
		resp := embeddingResponse{
			Data: []embeddingData{{Embedding: makeEmbedding(4), Index: 0}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	p.baseURL = server.URL

	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{
		{ID: 1, Content: "a"},
		{ID: 2, Content: "b"},
	})
	if res.Errors != 1 {
		t.Fatalf("expected 1 error for missing data, got %d", res.Errors)
	}
	// First succeeds, second is missing
	if res.Results[0].Error != nil {
		t.Errorf("expected first result to succeed, got %v", res.Results[0].Error)
	}
	if res.Results[1].Error == nil {
		t.Error("expected error for missing second embedding")
	}
}

func TestEmbedChunksNon200WithErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := embeddingResponse{Error: &apiError{Message: "rate limited", Code: "429"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	p.baseURL = server.URL

	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{{ID: 1, Content: "x"}})
	if res.Errors != 1 {
		t.Fatalf("expected 1 error, got %d", res.Errors)
	}
	if res.Results[0].Error == nil || !strings.Contains(res.Results[0].Error.Error(), "rate limited") {
		t.Errorf("expected parsed API error, got %v", res.Results[0].Error)
	}
}

func TestEmbedChunksNon200RawBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal failure`))
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	p.baseURL = server.URL

	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{{ID: 1, Content: "x"}})
	if res.Errors != 1 {
		t.Fatalf("expected 1 error, got %d", res.Errors)
	}
	if res.Results[0].Error == nil || !strings.Contains(res.Results[0].Error.Error(), "API error") {
		t.Errorf("expected raw API error, got %v", res.Results[0].Error)
	}
}

func TestEmbedChunksErrorInResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 200 status but error field set in the body
		resp := embeddingResponse{Error: &apiError{Message: "embedded error", Code: 1}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	p.baseURL = server.URL

	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{{ID: 1, Content: "x"}})
	if res.Errors != 1 {
		t.Fatalf("expected 1 error, got %d", res.Errors)
	}
	if res.Results[0].Error == nil || !strings.Contains(res.Results[0].Error.Error(), "embedded error") {
		t.Errorf("expected in-body API error, got %v", res.Results[0].Error)
	}
}

func TestEmbedChunksBadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json at all`))
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	p.baseURL = server.URL

	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{{ID: 1, Content: "x"}})
	if res.Errors != 1 {
		t.Fatalf("expected 1 error for bad JSON, got %d", res.Errors)
	}
}

func TestEmbedQuerySuccess(t *testing.T) {
	const dim = 6
	var gotBody embeddingRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		resp := embeddingResponse{Data: []embeddingData{{Embedding: makeEmbedding(dim), Index: 0}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", dim)
	p.baseURL = server.URL

	vec := p.EmbedQuery(context.Background(), "find me")
	if len(vec) != dim {
		t.Fatalf("expected vector of length %d, got %d", dim, len(vec))
	}
	if gotBody.Dimensions != dim {
		t.Errorf("expected dimensions %d in request, got %d", dim, gotBody.Dimensions)
	}
	if len(gotBody.Input) != 1 || gotBody.Input[0] != "find me" {
		t.Errorf("unexpected input: %+v", gotBody.Input)
	}
}

func TestEmbedQueryNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	p.baseURL = server.URL

	if vec := p.EmbedQuery(context.Background(), "x"); vec != nil {
		t.Errorf("expected nil for non-200, got %v", vec)
	}
}

func TestEmbedQueryDimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{Data: []embeddingData{{Embedding: makeEmbedding(3), Index: 0}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 8)
	p.baseURL = server.URL

	if vec := p.EmbedQuery(context.Background(), "x"); vec != nil {
		t.Errorf("expected nil for dimension mismatch, got %v", vec)
	}
}

func TestEmbedQueryEmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{Data: []embeddingData{}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	p.baseURL = server.URL

	if vec := p.EmbedQuery(context.Background(), "x"); vec != nil {
		t.Errorf("expected nil for empty data, got %v", vec)
	}
}

func TestEmbedQueryBadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`garbage`))
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	p.baseURL = server.URL

	if vec := p.EmbedQuery(context.Background(), "x"); vec != nil {
		t.Errorf("expected nil for bad JSON, got %v", vec)
	}
}

func TestEmbeddingValidateSuccess(t *testing.T) {
	const dim = 4
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{Data: []embeddingData{{Embedding: makeEmbedding(dim), Index: 0}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", dim)
	p.baseURL = server.URL

	if err := p.Validate(context.Background()); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
}

func TestEmbeddingValidateFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p, _ := NewOpenRouterEmbeddingProvider("key", "m", 4)
	p.baseURL = server.URL

	if err := p.Validate(context.Background()); err == nil {
		t.Fatal("expected validation failure")
	}
}
