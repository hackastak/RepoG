package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackastak/repog/internal/provider"
)

// makeEmbedding returns a deterministic float64 slice of the requested length.
func makeEmbedding(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i) * 0.001
	}
	return out
}

// --- LLM provider tests ---

func TestNewOllamaLLMProvider_Defaults(t *testing.T) {
	p, err := NewOllamaLLMProvider("", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.model != "llama3.2" {
		t.Errorf("expected default model llama3.2, got %q", p.model)
	}
	if p.fallbackModel != "llama2" {
		t.Errorf("expected default fallback llama2, got %q", p.fallbackModel)
	}
	if p.baseURL != defaultBaseURL {
		t.Errorf("expected default baseURL %q, got %q", defaultBaseURL, p.baseURL)
	}
}

func TestNewOllamaLLMProvider_Custom(t *testing.T) {
	p, err := NewOllamaLLMProvider("mymodel", "myfallback", "http://example.com:1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.model != "mymodel" || p.fallbackModel != "myfallback" || p.baseURL != "http://example.com:1234" {
		t.Errorf("custom values not stored: %+v", p)
	}
}

func TestOllamaLLMProvider_Name(t *testing.T) {
	p, _ := NewOllamaLLMProvider("m", "f", "http://x")
	if p.Name() != "ollama" {
		t.Errorf("expected name ollama, got %q", p.Name())
	}
}

func TestOllamaLLMProvider_Call_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify request body decodes.
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Stream {
			t.Errorf("expected non-streaming request")
		}
		resp := chatResponse{Message: message{Role: "assistant", Content: "hello world from ollama"}, Done: true}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, _ := NewOllamaLLMProvider("m", "f", srv.URL)
	res, lerr := p.Call(context.Background(), provider.LLMRequest{
		Prompt:       "hi there",
		SystemPrompt: "be brief",
	})
	if lerr != nil {
		t.Fatalf("unexpected error: %+v", lerr)
	}
	if res.Text != "hello world from ollama" {
		t.Errorf("unexpected text: %q", res.Text)
	}
	// input = fields of "be brief hi there" = 4, output = fields of text = 4
	if res.InputTokens != 4 {
		t.Errorf("expected 4 input tokens, got %d", res.InputTokens)
	}
	if res.OutputTokens != 4 {
		t.Errorf("expected 4 output tokens, got %d", res.OutputTokens)
	}
}

func TestOllamaLLMProvider_Call_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := NewOllamaLLMProvider("m", "f", srv.URL)
	res, lerr := p.Call(context.Background(), provider.LLMRequest{Prompt: "x"})
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}
	if lerr == nil {
		t.Fatal("expected error")
	}
	if lerr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 status, got %d", lerr.StatusCode)
	}
}

func TestOllamaLLMProvider_Call_FallbackOn404(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Model)
		if req.Model == "primary" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		resp := chatResponse{Message: message{Content: "fallback answer"}, Done: true}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, _ := NewOllamaLLMProvider("primary", "fallback", srv.URL)
	res, lerr := p.Call(context.Background(), provider.LLMRequest{Prompt: "q"})
	if lerr != nil {
		t.Fatalf("unexpected error: %+v", lerr)
	}
	if res.Text != "fallback answer" {
		t.Errorf("expected fallback answer, got %q", res.Text)
	}
	if len(calls) != 2 || calls[0] != "primary" || calls[1] != "fallback" {
		t.Errorf("expected primary then fallback calls, got %v", calls)
	}
}

func TestOllamaLLMProvider_Call_AllModelsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p, _ := NewOllamaLLMProvider("primary", "fallback", srv.URL)
	res, lerr := p.Call(context.Background(), provider.LLMRequest{Prompt: "q"})
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}
	if lerr == nil || lerr.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 lastError, got %+v", lerr)
	}
}

func TestOllamaLLMProvider_Validate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{Message: message{Content: "ok"}, Done: true}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, _ := NewOllamaLLMProvider("m", "f", srv.URL)
	if err := p.Validate(context.Background()); err != nil {
		t.Errorf("expected validate to pass, got %v", err)
	}
}

func TestOllamaLLMProvider_Validate_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := NewOllamaLLMProvider("m", "f", srv.URL)
	if err := p.Validate(context.Background()); err == nil {
		t.Error("expected validation failure")
	}
}

func TestOllamaLLMProvider_Stream_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		if !req.Stream {
			t.Errorf("expected stream=true")
		}
		// Ollama streams newline-delimited JSON objects (NOT SSE).
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		chunks := []chatResponse{
			{Message: message{Content: "Hello "}, Done: false},
			{Message: message{Content: "streaming "}, Done: false},
			{Message: message{Content: "world"}, Done: true},
		}
		for _, c := range chunks {
			b, _ := json.Marshal(c)
			_, _ = w.Write(append(b, '\n'))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	p, _ := NewOllamaLLMProvider("m", "f", srv.URL)
	var accumulated strings.Builder
	res, lerr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "go"}, func(chunk string) {
		accumulated.WriteString(chunk)
	})
	if lerr != nil {
		t.Fatalf("unexpected error: %+v", lerr)
	}
	if res.Text != "Hello streaming world" {
		t.Errorf("unexpected full text: %q", res.Text)
	}
	if accumulated.String() != "Hello streaming world" {
		t.Errorf("onChunk did not accumulate correctly: %q", accumulated.String())
	}
}

func TestOllamaLLMProvider_Stream_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "broken", http.StatusBadGateway)
	}))
	defer srv.Close()

	p, _ := NewOllamaLLMProvider("m", "f", srv.URL)
	res, lerr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "go"}, nil)
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}
	if lerr == nil || lerr.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 error, got %+v", lerr)
	}
}

func TestOllamaLLMProvider_Stream_FallbackOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Model == "primary" {
			http.Error(w, "missing", http.StatusNotFound)
			return
		}
		resp := chatResponse{Message: message{Content: "ok"}, Done: true}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(append(b, '\n'))
	}))
	defer srv.Close()

	p, _ := NewOllamaLLMProvider("primary", "fallback", srv.URL)
	res, lerr := p.Stream(context.Background(), provider.LLMRequest{Prompt: "go"}, nil)
	if lerr != nil {
		t.Fatalf("unexpected error: %+v", lerr)
	}
	if res.Text != "ok" {
		t.Errorf("expected fallback text ok, got %q", res.Text)
	}
}

// --- Embedding provider tests ---

func TestNewOllamaEmbeddingProvider_Defaults(t *testing.T) {
	p, err := NewOllamaEmbeddingProvider("", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.model != "nomic-embed-text" {
		t.Errorf("expected default model nomic-embed-text, got %q", p.model)
	}
	if p.baseURL != defaultBaseURL {
		t.Errorf("expected default baseURL, got %q", p.baseURL)
	}
	// nomic-embed-text spec dimensions = 768
	if p.Dimensions() != 768 {
		t.Errorf("expected 768 dimensions, got %d", p.Dimensions())
	}
}

func TestOllamaEmbeddingProvider_Getters(t *testing.T) {
	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", 0, "http://x")
	if p.Name() != "ollama" {
		t.Errorf("expected ollama, got %q", p.Name())
	}
	if p.Dimensions() != 768 {
		t.Errorf("expected 768 dimensions, got %d", p.Dimensions())
	}
	if p.BatchSize() != 10 {
		t.Errorf("expected batch size 10, got %d", p.BatchSize())
	}
	if p.MaxTokens() != 8192 {
		t.Errorf("expected 8192 max tokens, got %d", p.MaxTokens())
	}
}

func TestOllamaEmbeddingProvider_GettersUnknownModel(t *testing.T) {
	// Unknown model with explicit dimensions; MaxTokens uses defaultModelSpec.
	p, _ := NewOllamaEmbeddingProvider("totally-unknown-model", 99, "http://x")
	if p.Dimensions() != 99 {
		t.Errorf("expected 99 dimensions, got %d", p.Dimensions())
	}
	if p.MaxTokens() != defaultModelSpec.MaxTokens {
		t.Errorf("expected %d max tokens, got %d", defaultModelSpec.MaxTokens, p.MaxTokens())
	}
}

func TestGetModelSpec(t *testing.T) {
	if got := getModelSpec("mxbai-embed-large"); got.Dimensions != 1024 || got.MaxTokens != 512 {
		t.Errorf("unexpected spec for known model: %+v", got)
	}
	if got := getModelSpec("nope"); got != defaultModelSpec {
		t.Errorf("expected default spec for unknown model, got %+v", got)
	}
}

func TestOllamaEmbeddingProvider_EmbedQuery_Success(t *testing.T) {
	const dim = 768
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		if req.Prompt != "hello" {
			t.Errorf("unexpected prompt: %q", req.Prompt)
		}
		_ = json.NewEncoder(w).Encode(embeddingResponse{Embedding: makeEmbedding(dim)})
	}))
	defer srv.Close()

	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", dim, srv.URL)
	emb := p.EmbedQuery(context.Background(), "hello")
	if len(emb) != dim {
		t.Fatalf("expected %d dims, got %d", dim, len(emb))
	}
	if emb[1] != float32(0.001) {
		t.Errorf("unexpected value at index 1: %v", emb[1])
	}
}

func TestOllamaEmbeddingProvider_EmbedQuery_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", 768, srv.URL)
	if emb := p.EmbedQuery(context.Background(), "x"); emb != nil {
		t.Errorf("expected nil on error, got %v", emb)
	}
}

func TestOllamaEmbeddingProvider_EmbedQuery_EmptyEmbedding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(embeddingResponse{Embedding: []float64{}})
	}))
	defer srv.Close()

	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", 768, srv.URL)
	if emb := p.EmbedQuery(context.Background(), "x"); emb != nil {
		t.Errorf("expected nil for empty embedding, got %v", emb)
	}
}

func TestOllamaEmbeddingProvider_EmbedQuery_DimensionMismatch(t *testing.T) {
	const returnedDim = 512
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(embeddingResponse{Embedding: makeEmbedding(returnedDim)})
	}))
	defer srv.Close()

	// Construct with a mismatched declared dimension.
	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", 768, srv.URL)
	emb := p.EmbedQuery(context.Background(), "x")
	if len(emb) != returnedDim {
		t.Fatalf("expected %d dims, got %d", returnedDim, len(emb))
	}
	// Provider self-corrects its dimensions to match returned vector.
	if p.Dimensions() != returnedDim {
		t.Errorf("expected dimensions to update to %d, got %d", returnedDim, p.Dimensions())
	}
}

func TestOllamaEmbeddingProvider_Validate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(embeddingResponse{Embedding: makeEmbedding(768)})
	}))
	defer srv.Close()

	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", 768, srv.URL)
	if err := p.Validate(context.Background()); err != nil {
		t.Errorf("expected validate ok, got %v", err)
	}
}

func TestOllamaEmbeddingProvider_Validate_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", 768, srv.URL)
	if err := p.Validate(context.Background()); err == nil {
		t.Error("expected validation failure")
	}
}

func TestOllamaEmbeddingProvider_EmbedChunks_Success(t *testing.T) {
	const dim = 768
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(embeddingResponse{Embedding: makeEmbedding(dim)})
	}))
	defer srv.Close()

	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", dim, srv.URL)
	chunks := []provider.EmbedRequest{
		{ID: 1, Content: "alpha"},
		{ID: 2, Content: "beta"},
	}
	res := p.EmbedChunks(context.Background(), chunks)
	if res.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", res.Errors)
	}
	if len(res.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res.Results))
	}
	for i, r := range res.Results {
		if r.Error != nil {
			t.Errorf("result %d has error: %v", i, r.Error)
		}
		if len(r.Embedding) != dim {
			t.Errorf("result %d has %d dims, want %d", i, len(r.Embedding), dim)
		}
	}
	if res.Results[0].ID != 1 || res.Results[1].ID != 2 {
		t.Errorf("IDs not preserved: %d, %d", res.Results[0].ID, res.Results[1].ID)
	}
}

func TestOllamaEmbeddingProvider_EmbedChunks_Empty(t *testing.T) {
	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", 768, "http://unused")
	res := p.EmbedChunks(context.Background(), nil)
	if res.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", res.Errors)
	}
	if len(res.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(res.Results))
	}
}

func TestOllamaEmbeddingProvider_EmbedChunks_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := NewOllamaEmbeddingProvider("nomic-embed-text", 768, srv.URL)
	res := p.EmbedChunks(context.Background(), []provider.EmbedRequest{{ID: 7, Content: "x"}})
	if res.Errors != 1 {
		t.Errorf("expected 1 error, got %d", res.Errors)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res.Results))
	}
	if res.Results[0].Error == nil {
		t.Error("expected result error to be set")
	}
	if res.Results[0].Embedding != nil {
		t.Error("expected nil embedding on error")
	}
	if res.Results[0].ID != 7 {
		t.Errorf("expected ID 7, got %d", res.Results[0].ID)
	}
}

// Ensure providers satisfy their interfaces at compile time.
var (
	_ provider.LLMProvider       = (*OllamaLLMProvider)(nil)
	_ provider.EmbeddingProvider = (*OllamaEmbeddingProvider)(nil)
)
