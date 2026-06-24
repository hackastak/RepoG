package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackastak/repog/internal/provider"
)

// makeEmbedding returns a float32 slice of length n.
func makeEmbedding(n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(i) * 0.001
	}
	return out
}

// --- LLM provider tests ---

func TestNewOpenAILLMProvider(t *testing.T) {
	// Success with explicit values
	p, err := NewOpenAILLMProvider("key", "my-model", "my-fallback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.apiKey != "key" || p.model != "my-model" || p.fallbackModel != "my-fallback" {
		t.Errorf("fields not set correctly: %+v", p)
	}
	if p.baseURL != defaultBaseURL {
		t.Errorf("expected baseURL %q, got %q", defaultBaseURL, p.baseURL)
	}

	// Defaults filled when model/fallback empty
	p2, err := NewOpenAILLMProvider("key", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p2.model != "gpt-4o" {
		t.Errorf("expected default model gpt-4o, got %q", p2.model)
	}
	if p2.fallbackModel != "gpt-3.5-turbo" {
		t.Errorf("expected default fallback gpt-3.5-turbo, got %q", p2.fallbackModel)
	}

	// Empty apiKey error
	if _, err := NewOpenAILLMProvider("", "m", "f"); err == nil {
		t.Error("expected error for empty apiKey")
	}
}

func TestLLMName(t *testing.T) {
	p, _ := NewOpenAILLMProvider("key", "m", "f")
	if p.Name() != "openai" {
		t.Errorf("expected name openai, got %q", p.Name())
	}
}

func TestLLMCallSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Errorf("unexpected auth header: %q", got)
		}
		resp := chatCompletionResponse{
			Choices: []choice{
				{Message: message{Role: "assistant", Content: "hello world"}},
			},
			Usage: usageInfo{PromptTokens: 12, CompletionTokens: 5, TotalTokens: 17},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenAILLMProvider("key", "gpt-4o", "gpt-3.5-turbo")
	p.baseURL = server.URL

	res, llmErr := p.Call(context.Background(), provider.LLMRequest{
		Prompt:       "hi",
		SystemPrompt: "be nice",
	})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "hello world" {
		t.Errorf("expected text 'hello world', got %q", res.Text)
	}
	if res.InputTokens != 12 || res.OutputTokens != 5 {
		t.Errorf("unexpected token counts: in=%d out=%d", res.InputTokens, res.OutputTokens)
	}
}

func TestLLMCallNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	p, _ := NewOpenAILLMProvider("key", "gpt-4o", "gpt-3.5-turbo")
	p.baseURL = server.URL

	res, llmErr := p.Call(context.Background(), provider.LLMRequest{Prompt: "hi"})
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
	if llmErr == nil {
		t.Fatal("expected error")
	}
	if llmErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", llmErr.StatusCode)
	}
}

func TestLLMCallFallback(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, body.Model)
		if body.Model == "primary" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("model not found"))
			return
		}
		resp := chatCompletionResponse{
			Choices: []choice{{Message: message{Content: "fallback text"}}},
			Usage:   usageInfo{PromptTokens: 1, CompletionTokens: 2},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenAILLMProvider("key", "primary", "fallback")
	p.baseURL = server.URL

	res, llmErr := p.Call(context.Background(), provider.LLMRequest{Prompt: "hi"})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "fallback text" {
		t.Errorf("expected fallback text, got %q", res.Text)
	}
	if len(calls) != 2 || calls[0] != "primary" || calls[1] != "fallback" {
		t.Errorf("unexpected call sequence: %v", calls)
	}
}

func TestLLMValidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{
			Choices: []choice{{Message: message{Content: "ok"}}},
			Usage:   usageInfo{PromptTokens: 1, CompletionTokens: 1},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenAILLMProvider("key", "m", "f")
	p.baseURL = server.URL
	if err := p.Validate(context.Background()); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}

	// Validate failure path
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("err"))
	}))
	defer badServer.Close()
	p.baseURL = badServer.URL
	if err := p.Validate(context.Background()); err == nil {
		t.Error("expected validate error")
	}
}

func TestLLMStreamSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []streamChunk{
			{Choices: []choice{{Delta: messageDelta{Content: "Hello"}}}},
			{Choices: []choice{{Delta: messageDelta{Content: ", "}}}},
			{Choices: []choice{{Delta: messageDelta{Content: "world"}}}, Usage: &usageInfo{PromptTokens: 7, CompletionTokens: 3}},
		}
		for _, c := range chunks {
			b, _ := json.Marshal(c)
			_, _ = fmt.Fprintf(w, "data: %s\n", string(b))
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n")
	}))
	defer server.Close()

	p, _ := NewOpenAILLMProvider("key", "m", "f")
	p.baseURL = server.URL

	var sb strings.Builder
	res, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "hi", SystemPrompt: "sys"}, func(s string) {
		sb.WriteString(s)
	})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if sb.String() != "Hello, world" {
		t.Errorf("expected accumulated 'Hello, world', got %q", sb.String())
	}
	if res.Text != "Hello, world" {
		t.Errorf("expected result text 'Hello, world', got %q", res.Text)
	}
	if res.InputTokens != 7 || res.OutputTokens != 3 {
		t.Errorf("unexpected tokens: in=%d out=%d", res.InputTokens, res.OutputTokens)
	}
}

func TestLLMStreamNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	p, _ := NewOpenAILLMProvider("key", "m", "f")
	p.baseURL = server.URL
	res, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "hi"}, nil)
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
	if llmErr == nil || llmErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 error, got %+v", llmErr)
	}
}

// --- Embedding provider tests ---

func TestNewOpenAIEmbeddingProvider(t *testing.T) {
	// Success with explicit values
	p, err := NewOpenAIEmbeddingProvider("key", "text-embedding-3-large", 256)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.apiKey != "key" || p.model != "text-embedding-3-large" || p.dimensions != 256 {
		t.Errorf("fields not set correctly: %+v", p)
	}
	if p.batchSize != 100 {
		t.Errorf("expected batchSize 100, got %d", p.batchSize)
	}
	if p.baseURL != defaultBaseURL {
		t.Errorf("expected default baseURL, got %q", p.baseURL)
	}

	// Default model and dimensions defaulting from model spec
	p2, err := NewOpenAIEmbeddingProvider("key", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p2.model != "text-embedding-3-small" {
		t.Errorf("expected default model, got %q", p2.model)
	}
	if p2.dimensions != 1536 {
		t.Errorf("expected default dims 1536, got %d", p2.dimensions)
	}

	// Dimensions defaulting from a large model spec
	p3, err := NewOpenAIEmbeddingProvider("key", "text-embedding-3-large", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p3.dimensions != 3072 {
		t.Errorf("expected dims 3072, got %d", p3.dimensions)
	}

	// Empty apiKey error
	if _, err := NewOpenAIEmbeddingProvider("", "m", 1); err == nil {
		t.Error("expected error for empty apiKey")
	}
}

func TestEmbeddingAccessors(t *testing.T) {
	p, _ := NewOpenAIEmbeddingProvider("key", "text-embedding-3-small", 768)
	if p.Name() != "openai" {
		t.Errorf("expected name openai, got %q", p.Name())
	}
	if p.Dimensions() != 768 {
		t.Errorf("expected 768 dims, got %d", p.Dimensions())
	}
	if p.BatchSize() != 100 {
		t.Errorf("expected batch 100, got %d", p.BatchSize())
	}
	if p.MaxTokens() != 8191 {
		t.Errorf("expected 8191 max tokens, got %d", p.MaxTokens())
	}
}

func TestGetOpenAIModelSpec(t *testing.T) {
	spec := getOpenAIModelSpec("text-embedding-3-large")
	if spec.Dimensions != 3072 || spec.MaxTokens != 8191 {
		t.Errorf("unexpected known spec: %+v", spec)
	}
	unknown := getOpenAIModelSpec("does-not-exist")
	if unknown != defaultOpenAIModelSpec {
		t.Errorf("expected default spec for unknown model, got %+v", unknown)
	}
}

func TestEmbedChunksSuccess(t *testing.T) {
	const dims = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req embeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := embeddingResponse{}
		for i := range req.Input {
			resp.Data = append(resp.Data, embeddingData{Embedding: makeEmbedding(dims), Index: i})
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenAIEmbeddingProvider("key", "m", dims)
	p.baseURL = server.URL

	chunks := []provider.EmbedRequest{
		{ID: 1, Content: "a"},
		{ID: 2, Content: "b"},
	}
	res := p.EmbedChunks(context.Background(), chunks)
	if res.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", res.Errors)
	}
	if len(res.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res.Results))
	}
	for _, r := range res.Results {
		if r.Error != nil {
			t.Errorf("unexpected result error: %v", r.Error)
		}
		if len(r.Embedding) != dims {
			t.Errorf("expected %d dims, got %d", dims, len(r.Embedding))
		}
	}
}

func TestEmbedChunksEmpty(t *testing.T) {
	p, _ := NewOpenAIEmbeddingProvider("key", "m", 8)
	res := p.EmbedChunks(context.Background(), nil)
	if res.Errors != 0 || len(res.Results) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

func TestEmbedChunksNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	p, _ := NewOpenAIEmbeddingProvider("key", "m", 8)
	p.baseURL = server.URL

	chunks := []provider.EmbedRequest{{ID: 1, Content: "a"}}
	res := p.EmbedChunks(context.Background(), chunks)
	if res.Errors != 1 {
		t.Errorf("expected 1 error, got %d", res.Errors)
	}
	if len(res.Results) != 1 || res.Results[0].Error == nil {
		t.Errorf("expected per-chunk error, got %+v", res.Results)
	}
}

func TestEmbedChunksDimensionMismatch(t *testing.T) {
	const wantDims = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{
			Data: []embeddingData{{Embedding: makeEmbedding(wantDims + 4), Index: 0}},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenAIEmbeddingProvider("key", "m", wantDims)
	p.baseURL = server.URL

	chunks := []provider.EmbedRequest{{ID: 1, Content: "a"}}
	res := p.EmbedChunks(context.Background(), chunks)
	if res.Errors != 1 {
		t.Errorf("expected 1 error, got %d", res.Errors)
	}
	if len(res.Results) != 1 || res.Results[0].Error == nil {
		t.Fatalf("expected dimension-mismatch error, got %+v", res.Results)
	}
	if !strings.Contains(res.Results[0].Error.Error(), "invalid dimensions") {
		t.Errorf("expected dimension error message, got %v", res.Results[0].Error)
	}
}

func TestEmbedQuerySuccess(t *testing.T) {
	const dims = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{
			Data: []embeddingData{{Embedding: makeEmbedding(dims), Index: 0}},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenAIEmbeddingProvider("key", "m", dims)
	p.baseURL = server.URL

	emb := p.EmbedQuery(context.Background(), "query")
	if len(emb) != dims {
		t.Errorf("expected %d dims, got %d", dims, len(emb))
	}
}

func TestEmbedQueryNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("err"))
	}))
	defer server.Close()

	p, _ := NewOpenAIEmbeddingProvider("key", "m", 8)
	p.baseURL = server.URL

	if emb := p.EmbedQuery(context.Background(), "query"); emb != nil {
		t.Errorf("expected nil embedding, got %v", emb)
	}
}

func TestEmbeddingValidate(t *testing.T) {
	const dims = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{
			Data: []embeddingData{{Embedding: makeEmbedding(dims), Index: 0}},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewOpenAIEmbeddingProvider("key", "m", dims)
	p.baseURL = server.URL
	if err := p.Validate(context.Background()); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}

	// Failure path
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badServer.Close()
	p.baseURL = badServer.URL
	if err := p.Validate(context.Background()); err == nil {
		t.Error("expected validate error")
	}
}
