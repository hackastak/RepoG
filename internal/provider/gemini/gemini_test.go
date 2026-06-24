package gemini

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

// makeFloat32Slice returns a deterministic float32 slice of length n.
func makeFloat32Slice(n int) []float32 {
	vals := make([]float32, n)
	for i := range vals {
		vals[i] = float32(i) * 0.001
	}
	return vals
}

// ---------------------------------------------------------------------------
// LLM provider tests
// ---------------------------------------------------------------------------

func TestNewGeminiLLMProvider(t *testing.T) {
	t.Run("success with explicit models", func(t *testing.T) {
		p, err := NewGeminiLLMProvider("key", "my-model", "my-fallback")
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
		p, err := NewGeminiLLMProvider("", "m", "f")
		if err == nil {
			t.Fatal("expected error for empty apiKey, got nil")
		}
		if p != nil {
			t.Errorf("expected nil provider, got %#v", p)
		}
	})

	t.Run("default model and fallback", func(t *testing.T) {
		p, err := NewGeminiLLMProvider("key", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.model != "gemini-2.5-flash" {
			t.Errorf("default model = %q, want %q", p.model, "gemini-2.5-flash")
		}
		if p.fallbackModel != "gemini-3.0-flash" {
			t.Errorf("default fallbackModel = %q, want %q", p.fallbackModel, "gemini-3.0-flash")
		}
	})
}

func TestGeminiLLMProvider_Name(t *testing.T) {
	p, _ := NewGeminiLLMProvider("key", "m", "f")
	if got := p.Name(); got != "gemini" {
		t.Errorf("Name() = %q, want %q", got, "gemini")
	}
}

func TestGeminiLLMProvider_Call_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The key and model appear in the URL.
		if !strings.Contains(r.URL.RawQuery, "key=key") {
			t.Errorf("expected key in query, got %q", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.Path, "primary-model") {
			t.Errorf("expected model in path, got %q", r.URL.Path)
		}
		resp := generateContentResponse{
			Candidates: []candidate{
				{Content: content{Parts: []textPart{{Text: "hello world"}}}},
			},
			UsageMetadata: usageMetadata{
				PromptTokenCount:     12,
				CandidatesTokenCount: 34,
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewGeminiLLMProvider("key", "primary-model", "fallback-model")
	p.baseURL = server.URL

	res, llmErr := p.Call(context.Background(), provider.LLMRequest{
		Prompt:       "test prompt",
		SystemPrompt: "be helpful",
	})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "hello world" {
		t.Errorf("Text = %q, want %q", res.Text, "hello world")
	}
	if res.InputTokens != 12 {
		t.Errorf("InputTokens = %d, want 12", res.InputTokens)
	}
	if res.OutputTokens != 34 {
		t.Errorf("OutputTokens = %d, want 34", res.OutputTokens)
	}
}

func TestGeminiLLMProvider_Call_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	p, _ := NewGeminiLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	res, llmErr := p.Call(context.Background(), provider.LLMRequest{Prompt: "x"})
	if llmErr == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result, got %#v", res)
	}
	if llmErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", llmErr.StatusCode, http.StatusInternalServerError)
	}
	if !strings.Contains(llmErr.Message, "boom") {
		t.Errorf("Message = %q, want to contain %q", llmErr.Message, "boom")
	}
}

func TestGeminiLLMProvider_Call_Fallback(t *testing.T) {
	var hits []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		// Primary model 404s, fallback succeeds.
		if strings.Contains(r.URL.Path, "primary") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
			return
		}
		resp := generateContentResponse{
			Candidates: []candidate{
				{Content: content{Parts: []textPart{{Text: "from fallback"}}}},
			},
			UsageMetadata: usageMetadata{PromptTokenCount: 1, CandidatesTokenCount: 2},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewGeminiLLMProvider("key", "primary", "fallback")
	p.baseURL = server.URL

	res, llmErr := p.Call(context.Background(), provider.LLMRequest{Prompt: "x"})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "from fallback" {
		t.Errorf("Text = %q, want %q", res.Text, "from fallback")
	}
	if len(hits) != 2 {
		t.Errorf("expected 2 requests (primary + fallback), got %d: %v", len(hits), hits)
	}
}

func TestGeminiLLMProvider_Call_AllNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("missing"))
	}))
	defer server.Close()

	p, _ := NewGeminiLLMProvider("key", "primary", "fallback")
	p.baseURL = server.URL

	res, llmErr := p.Call(context.Background(), provider.LLMRequest{Prompt: "x"})
	if llmErr == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result, got %#v", res)
	}
	if llmErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", llmErr.StatusCode)
	}
}

func TestGeminiLLMProvider_Validate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := generateContentResponse{
				Candidates: []candidate{
					{Content: content{Parts: []textPart{{Text: "ok"}}}},
				},
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, _ := NewGeminiLLMProvider("key", "m", "f")
		p.baseURL = server.URL

		if err := p.Validate(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
		}))
		defer server.Close()

		p, _ := NewGeminiLLMProvider("key", "m", "f")
		p.baseURL = server.URL

		if err := p.Validate(context.Background()); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestGeminiLLMProvider_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "alt=sse") {
			t.Errorf("expected alt=sse in query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []streamChunk{
			{
				Candidates: []candidate{
					{Content: content{Parts: []textPart{{Text: "Hello "}}}},
				},
				UsageMetadata: usageMetadata{PromptTokenCount: 5},
			},
			{
				Candidates: []candidate{
					{Content: content{Parts: []textPart{{Text: "world"}}}},
				},
				UsageMetadata: usageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 7},
			},
		}
		for _, c := range chunks {
			b, _ := json.Marshal(c)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		}
		// An empty data line should be skipped gracefully.
		_, _ = fmt.Fprint(w, "data: \n\n")
	}))
	defer server.Close()

	p, _ := NewGeminiLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	var collected strings.Builder
	res, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "x"}, func(s string) {
		collected.WriteString(s)
	})
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "Hello world" {
		t.Errorf("Text = %q, want %q", res.Text, "Hello world")
	}
	if collected.String() != "Hello world" {
		t.Errorf("collected = %q, want %q", collected.String(), "Hello world")
	}
	if res.InputTokens != 5 {
		t.Errorf("InputTokens = %d, want 5", res.InputTokens)
	}
	if res.OutputTokens != 7 {
		t.Errorf("OutputTokens = %d, want 7", res.OutputTokens)
	}
}

func TestGeminiLLMProvider_Stream_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer server.Close()

	p, _ := NewGeminiLLMProvider("key", "m", "f")
	p.baseURL = server.URL

	res, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "x"}, nil)
	if llmErr == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result, got %#v", res)
	}
	if llmErr.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want %d", llmErr.StatusCode, http.StatusBadGateway)
	}
}

func TestGeminiLLMProvider_Stream_Fallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "primary") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("nope"))
			return
		}
		w.WriteHeader(http.StatusOK)
		c := streamChunk{
			Candidates: []candidate{
				{Content: content{Parts: []textPart{{Text: "fallback stream"}}}},
			},
			UsageMetadata: usageMetadata{PromptTokenCount: 3, CandidatesTokenCount: 4},
		}
		b, _ := json.Marshal(c)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
	}))
	defer server.Close()

	p, _ := NewGeminiLLMProvider("key", "primary", "fallback")
	p.baseURL = server.URL

	res, llmErr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "x"}, nil)
	if llmErr != nil {
		t.Fatalf("unexpected error: %+v", llmErr)
	}
	if res.Text != "fallback stream" {
		t.Errorf("Text = %q, want %q", res.Text, "fallback stream")
	}
}

// ---------------------------------------------------------------------------
// Embedding provider tests
// ---------------------------------------------------------------------------

func TestNewGeminiEmbeddingProvider(t *testing.T) {
	t.Run("success with explicit dimensions", func(t *testing.T) {
		p, err := NewGeminiEmbeddingProvider("key", "my-model", 512)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.dimensions != 512 {
			t.Errorf("dimensions = %d, want 512", p.dimensions)
		}
		if p.batchSize != 20 {
			t.Errorf("batchSize = %d, want 20", p.batchSize)
		}
		if p.baseURL != defaultBaseURL {
			t.Errorf("baseURL = %q, want %q", p.baseURL, defaultBaseURL)
		}
	})

	t.Run("empty apiKey returns error", func(t *testing.T) {
		p, err := NewGeminiEmbeddingProvider("", "m", 8)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if p != nil {
			t.Errorf("expected nil provider, got %#v", p)
		}
	})

	t.Run("default model and dimensions from spec", func(t *testing.T) {
		p, err := NewGeminiEmbeddingProvider("key", "", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.model != "gemini-embedding-2-preview" {
			t.Errorf("model = %q, want %q", p.model, "gemini-embedding-2-preview")
		}
		// Default spec for gemini-embedding-2-preview is 3072 dims.
		if p.dimensions != 3072 {
			t.Errorf("dimensions = %d, want 3072", p.dimensions)
		}
	})

	t.Run("unknown model falls back to default spec dimensions", func(t *testing.T) {
		p, err := NewGeminiEmbeddingProvider("key", "totally-unknown-model", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.dimensions != defaultGeminiModelSpec.Dimensions {
			t.Errorf("dimensions = %d, want %d", p.dimensions, defaultGeminiModelSpec.Dimensions)
		}
	})
}

func TestGeminiEmbeddingProvider_Getters(t *testing.T) {
	p, _ := NewGeminiEmbeddingProvider("key", "text-embedding-004", 0)
	if p.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gemini")
	}
	if p.Dimensions() != 768 {
		t.Errorf("Dimensions() = %d, want 768", p.Dimensions())
	}
	if p.BatchSize() != 20 {
		t.Errorf("BatchSize() = %d, want 20", p.BatchSize())
	}
	if p.MaxTokens() != 2048 {
		t.Errorf("MaxTokens() = %d, want 2048", p.MaxTokens())
	}

	t.Run("MaxTokens unknown model uses default spec", func(t *testing.T) {
		pu, _ := NewGeminiEmbeddingProvider("key", "weird-model", 16)
		if pu.MaxTokens() != defaultGeminiModelSpec.MaxTokens {
			t.Errorf("MaxTokens() = %d, want %d", pu.MaxTokens(), defaultGeminiModelSpec.MaxTokens)
		}
	})
}

func TestGeminiEmbeddingProvider_EmbedChunks_Success(t *testing.T) {
	const dims = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "batchEmbedContents") {
			t.Errorf("expected batchEmbedContents path, got %q", r.URL.Path)
		}
		resp := batchEmbedResponse{
			Embeddings: []embeddingData{
				{Values: makeFloat32Slice(dims)},
				{Values: makeFloat32Slice(dims)},
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewGeminiEmbeddingProvider("key", "m", dims)
	p.baseURL = server.URL

	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{
		{ID: 1, Content: "a"},
		{ID: 2, Content: "b"},
	})
	if res.Errors != 0 {
		t.Errorf("Errors = %d, want 0", res.Errors)
	}
	if len(res.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(res.Results))
	}
	for i, r := range res.Results {
		if r.Error != nil {
			t.Errorf("result %d error: %v", i, r.Error)
		}
		if len(r.Embedding) != dims {
			t.Errorf("result %d embedding len = %d, want %d", i, len(r.Embedding), dims)
		}
	}
	if res.Results[0].ID != 1 || res.Results[1].ID != 2 {
		t.Errorf("unexpected IDs: %d, %d", res.Results[0].ID, res.Results[1].ID)
	}
}

func TestGeminiEmbeddingProvider_EmbedChunks_Empty(t *testing.T) {
	p, _ := NewGeminiEmbeddingProvider("key", "m", 8)
	// No baseURL override; empty input must short-circuit before any HTTP call.
	res := p.EmbedChunks(context.Background(), nil)
	if res.Errors != 0 {
		t.Errorf("Errors = %d, want 0", res.Errors)
	}
	if len(res.Results) != 0 {
		t.Errorf("len(Results) = %d, want 0", len(res.Results))
	}
}

func TestGeminiEmbeddingProvider_EmbedChunks_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	p, _ := NewGeminiEmbeddingProvider("key", "m", 8)
	p.baseURL = server.URL

	chunks := []provider.EmbedRequest{{ID: 1, Content: "a"}, {ID: 2, Content: "b"}}
	res := p.EmbedChunks(context.Background(), chunks)
	if res.Errors != len(chunks) {
		t.Errorf("Errors = %d, want %d", res.Errors, len(chunks))
	}
	for _, r := range res.Results {
		if r.Error == nil {
			t.Errorf("expected error for ID %d, got nil", r.ID)
		}
		if r.Embedding != nil {
			t.Errorf("expected nil embedding for ID %d", r.ID)
		}
	}
}

func TestGeminiEmbeddingProvider_EmbedChunks_DimensionMismatch(t *testing.T) {
	const dims = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := batchEmbedResponse{
			Embeddings: []embeddingData{
				{Values: makeFloat32Slice(dims)},     // correct
				{Values: makeFloat32Slice(dims - 1)}, // wrong dims
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewGeminiEmbeddingProvider("key", "m", dims)
	p.baseURL = server.URL

	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{
		{ID: 1, Content: "a"},
		{ID: 2, Content: "b"},
	})
	if res.Errors != 1 {
		t.Errorf("Errors = %d, want 1", res.Errors)
	}
	if res.Results[0].Error != nil {
		t.Errorf("result 0 should be fine, got error: %v", res.Results[0].Error)
	}
	if res.Results[1].Error == nil {
		t.Error("result 1 should have a dimension-mismatch error")
	}
}

func TestGeminiEmbeddingProvider_EmbedChunks_MissingEmbedding(t *testing.T) {
	const dims = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return fewer embeddings than chunks.
		resp := batchEmbedResponse{
			Embeddings: []embeddingData{
				{Values: makeFloat32Slice(dims)},
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewGeminiEmbeddingProvider("key", "m", dims)
	p.baseURL = server.URL

	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{
		{ID: 1, Content: "a"},
		{ID: 2, Content: "b"},
	})
	if res.Errors != 1 {
		t.Errorf("Errors = %d, want 1", res.Errors)
	}
	if res.Results[1].Error == nil {
		t.Error("expected error for missing embedding on second chunk")
	}
}

func TestGeminiEmbeddingProvider_EmbedQuery_Success(t *testing.T) {
	const dims = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "embedContent") {
			t.Errorf("expected embedContent path, got %q", r.URL.Path)
		}
		resp := embedQueryResponse{
			Embedding: embeddingData{Values: makeFloat32Slice(dims)},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewGeminiEmbeddingProvider("key", "m", dims)
	p.baseURL = server.URL

	got := p.EmbedQuery(context.Background(), "query text")
	if len(got) != dims {
		t.Fatalf("len(embedding) = %d, want %d", len(got), dims)
	}
}

func TestGeminiEmbeddingProvider_EmbedQuery_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	p, _ := NewGeminiEmbeddingProvider("key", "m", 8)
	p.baseURL = server.URL

	if got := p.EmbedQuery(context.Background(), "q"); got != nil {
		t.Errorf("expected nil on non-200, got %v", got)
	}
}

func TestGeminiEmbeddingProvider_EmbedQuery_DimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embedQueryResponse{
			Embedding: embeddingData{Values: makeFloat32Slice(4)}, // wrong dims
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewGeminiEmbeddingProvider("key", "m", 8)
	p.baseURL = server.URL

	if got := p.EmbedQuery(context.Background(), "q"); got != nil {
		t.Errorf("expected nil on dimension mismatch, got %v", got)
	}
}

func TestGeminiEmbeddingProvider_Validate(t *testing.T) {
	const dims = 8
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := embedQueryResponse{Embedding: embeddingData{Values: makeFloat32Slice(dims)}}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p, _ := NewGeminiEmbeddingProvider("key", "m", dims)
		p.baseURL = server.URL

		if err := p.Validate(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		p, _ := NewGeminiEmbeddingProvider("key", "m", dims)
		p.baseURL = server.URL

		if err := p.Validate(context.Background()); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
