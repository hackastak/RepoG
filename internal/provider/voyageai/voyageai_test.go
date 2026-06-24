package voyageai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hackastak/repog/internal/provider"
)

// makeEmbedding returns a deterministic float32 slice of the given length.
func makeEmbedding(n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(i) * 0.001
	}
	return out
}

// newTestServer spins up an httptest server returning embedResp for /embeddings.
func newTestServer(t *testing.T, status int, body interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			b, _ := json.Marshal(body)
			_, _ = w.Write(b)
		}
	}))
}

func TestNewVoyageAIEmbeddingProvider(t *testing.T) {
	t.Run("success with explicit values", func(t *testing.T) {
		p, err := NewVoyageAIEmbeddingProvider("key", "voyage-3", 768)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.apiKey != "key" {
			t.Errorf("apiKey = %q, want key", p.apiKey)
		}
		if p.model != "voyage-3" {
			t.Errorf("model = %q, want voyage-3", p.model)
		}
		if p.dimensions != 768 {
			t.Errorf("dimensions = %d, want 768", p.dimensions)
		}
		if p.batchSize != 128 {
			t.Errorf("batchSize = %d, want 128", p.batchSize)
		}
		if p.baseURL != defaultBaseURL {
			t.Errorf("baseURL = %q, want %q", p.baseURL, defaultBaseURL)
		}
	})

	t.Run("empty apiKey error", func(t *testing.T) {
		_, err := NewVoyageAIEmbeddingProvider("", "voyage-3", 768)
		if err == nil {
			t.Fatal("expected error for empty apiKey, got nil")
		}
	})

	t.Run("default model", func(t *testing.T) {
		p, err := NewVoyageAIEmbeddingProvider("key", "", 768)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.model != "voyage-code-3" {
			t.Errorf("model = %q, want voyage-code-3", p.model)
		}
	})

	t.Run("default dimensions from model spec", func(t *testing.T) {
		// voyage-3-lite has 512 dimensions in the spec table.
		p, err := NewVoyageAIEmbeddingProvider("key", "voyage-3-lite", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.dimensions != 512 {
			t.Errorf("dimensions = %d, want 512", p.dimensions)
		}
	})

	t.Run("default dimensions for unknown model", func(t *testing.T) {
		p, err := NewVoyageAIEmbeddingProvider("key", "unknown-model", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.dimensions != defaultVoyageAIModelSpec.Dimensions {
			t.Errorf("dimensions = %d, want %d", p.dimensions, defaultVoyageAIModelSpec.Dimensions)
		}
	})
}

func TestName(t *testing.T) {
	p, _ := NewVoyageAIEmbeddingProvider("key", "voyage-3", 768)
	if got := p.Name(); got != "voyageai" {
		t.Errorf("Name() = %q, want voyageai", got)
	}
}

func TestDimensions(t *testing.T) {
	p, _ := NewVoyageAIEmbeddingProvider("key", "voyage-3", 999)
	if got := p.Dimensions(); got != 999 {
		t.Errorf("Dimensions() = %d, want 999", got)
	}
}

func TestBatchSize(t *testing.T) {
	p, _ := NewVoyageAIEmbeddingProvider("key", "voyage-3", 768)
	if got := p.BatchSize(); got != 128 {
		t.Errorf("BatchSize() = %d, want 128", got)
	}
}

func TestMaxTokens(t *testing.T) {
	t.Run("known model", func(t *testing.T) {
		p, _ := NewVoyageAIEmbeddingProvider("key", "voyage-3", 768)
		if got := p.MaxTokens(); got != 32000 {
			t.Errorf("MaxTokens() = %d, want 32000", got)
		}
	})

	t.Run("unknown model uses default", func(t *testing.T) {
		p, _ := NewVoyageAIEmbeddingProvider("key", "unknown-model", 768)
		if got := p.MaxTokens(); got != defaultVoyageAIModelSpec.MaxTokens {
			t.Errorf("MaxTokens() = %d, want %d", got, defaultVoyageAIModelSpec.MaxTokens)
		}
	})
}

func TestGetVoyageAIModelSpec(t *testing.T) {
	t.Run("known model", func(t *testing.T) {
		spec := getVoyageAIModelSpec("voyage-code-3")
		if spec.Dimensions != 1024 || spec.MaxTokens != 16000 {
			t.Errorf("spec = %+v, want {1024 16000}", spec)
		}
	})

	t.Run("unknown model", func(t *testing.T) {
		spec := getVoyageAIModelSpec("does-not-exist")
		if spec != defaultVoyageAIModelSpec {
			t.Errorf("spec = %+v, want %+v", spec, defaultVoyageAIModelSpec)
		}
	})
}

func TestValidate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := embeddingResponse{
			Object: "list",
			Data: []embeddingData{
				{Object: "embedding", Embedding: makeEmbedding(768), Index: 0},
			},
			Model: "voyage-3",
			Usage: usageInfo{TotalTokens: 5},
		}
		server := newTestServer(t, http.StatusOK, resp)
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		if err := p.Validate(context.Background()); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		server := newTestServer(t, http.StatusInternalServerError, nil)
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		if err := p.Validate(context.Background()); err == nil {
			t.Error("Validate() error = nil, want non-nil")
		}
	})
}

func TestEmbedChunks(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := embeddingResponse{
			Object: "list",
			Data: []embeddingData{
				{Object: "embedding", Embedding: makeEmbedding(768), Index: 0},
				{Object: "embedding", Embedding: makeEmbedding(768), Index: 1},
			},
			Model: "voyage-3",
			Usage: usageInfo{TotalTokens: 10},
		}
		server := newTestServer(t, http.StatusOK, resp)
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		chunks := []provider.EmbedRequest{
			{ID: 1, Content: "hello"},
			{ID: 2, Content: "world"},
		}
		result := p.EmbedChunks(context.Background(), chunks)
		if result.Errors != 0 {
			t.Fatalf("Errors = %d, want 0", result.Errors)
		}
		if len(result.Results) != 2 {
			t.Fatalf("len(Results) = %d, want 2", len(result.Results))
		}
		for i, r := range result.Results {
			if r.Error != nil {
				t.Errorf("result[%d].Error = %v", i, r.Error)
			}
			if len(r.Embedding) != 768 {
				t.Errorf("result[%d] embedding len = %d, want 768", i, len(r.Embedding))
			}
		}
		if result.Results[0].ID != 1 || result.Results[1].ID != 2 {
			t.Errorf("IDs not preserved: %d, %d", result.Results[0].ID, result.Results[1].ID)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		result := p.EmbedChunks(context.Background(), nil)
		if result.Errors != 0 {
			t.Errorf("Errors = %d, want 0", result.Errors)
		}
		if len(result.Results) != 0 {
			t.Errorf("len(Results) = %d, want 0", len(result.Results))
		}
	})

	t.Run("non-200 error", func(t *testing.T) {
		server := newTestServer(t, http.StatusBadRequest, map[string]string{"detail": "bad"})
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		chunks := []provider.EmbedRequest{{ID: 1, Content: "hello"}}
		result := p.EmbedChunks(context.Background(), chunks)
		if result.Errors != 1 {
			t.Errorf("Errors = %d, want 1", result.Errors)
		}
		if len(result.Results) != 1 || result.Results[0].Error == nil {
			t.Errorf("expected one result with error, got %+v", result.Results)
		}
		if result.Results[0].Embedding != nil {
			t.Errorf("expected nil embedding on error")
		}
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		resp := embeddingResponse{
			Object: "list",
			Data: []embeddingData{
				// Returns 512 dims but provider expects 768.
				{Object: "embedding", Embedding: makeEmbedding(512), Index: 0},
			},
			Model: "voyage-3",
		}
		server := newTestServer(t, http.StatusOK, resp)
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		chunks := []provider.EmbedRequest{{ID: 7, Content: "hello"}}
		result := p.EmbedChunks(context.Background(), chunks)
		if result.Errors != 1 {
			t.Errorf("Errors = %d, want 1", result.Errors)
		}
		if len(result.Results) != 1 || result.Results[0].Error == nil {
			t.Fatalf("expected error result, got %+v", result.Results)
		}
		if result.Results[0].Embedding != nil {
			t.Errorf("expected nil embedding on dimension mismatch")
		}
	})

	t.Run("missing embedding for chunk", func(t *testing.T) {
		// Server returns fewer data entries than chunks.
		resp := embeddingResponse{
			Object: "list",
			Data: []embeddingData{
				{Object: "embedding", Embedding: makeEmbedding(768), Index: 0},
			},
			Model: "voyage-3",
		}
		server := newTestServer(t, http.StatusOK, resp)
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		chunks := []provider.EmbedRequest{
			{ID: 1, Content: "a"},
			{ID: 2, Content: "b"},
		}
		result := p.EmbedChunks(context.Background(), chunks)
		if result.Errors != 1 {
			t.Errorf("Errors = %d, want 1", result.Errors)
		}
		if len(result.Results) != 2 {
			t.Fatalf("len(Results) = %d, want 2", len(result.Results))
		}
		if result.Results[0].Error != nil {
			t.Errorf("first result should succeed, got %v", result.Results[0].Error)
		}
		if result.Results[1].Error == nil {
			t.Errorf("second result should have error")
		}
	})

	t.Run("network error", func(t *testing.T) {
		server := newTestServer(t, http.StatusOK, nil)
		// Close immediately so the request fails to connect.
		url := server.URL
		server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = url

		chunks := []provider.EmbedRequest{{ID: 1, Content: "x"}}
		result := p.EmbedChunks(context.Background(), chunks)
		if result.Errors != 1 {
			t.Errorf("Errors = %d, want 1", result.Errors)
		}
		if len(result.Results) != 1 || result.Results[0].Error == nil {
			t.Errorf("expected error result on network failure")
		}
	})

	t.Run("invalid json response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not json"))
		}))
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		chunks := []provider.EmbedRequest{{ID: 1, Content: "x"}}
		result := p.EmbedChunks(context.Background(), chunks)
		if result.Errors != 1 {
			t.Errorf("Errors = %d, want 1", result.Errors)
		}
	})
}

func TestEmbedQuery(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := embeddingResponse{
			Object: "list",
			Data: []embeddingData{
				{Object: "embedding", Embedding: makeEmbedding(768), Index: 0},
			},
			Model: "voyage-3",
		}
		server := newTestServer(t, http.StatusOK, resp)
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		emb := p.EmbedQuery(context.Background(), "hello")
		if len(emb) != 768 {
			t.Fatalf("embedding len = %d, want 768", len(emb))
		}
	})

	t.Run("nil on non-200", func(t *testing.T) {
		server := newTestServer(t, http.StatusInternalServerError, nil)
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		if emb := p.EmbedQuery(context.Background(), "hello"); emb != nil {
			t.Errorf("expected nil, got %v", emb)
		}
	})

	t.Run("nil on network error", func(t *testing.T) {
		server := newTestServer(t, http.StatusOK, nil)
		url := server.URL
		server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = url

		if emb := p.EmbedQuery(context.Background(), "hello"); emb != nil {
			t.Errorf("expected nil, got %v", emb)
		}
	})

	t.Run("nil on empty data", func(t *testing.T) {
		resp := embeddingResponse{Object: "list", Data: []embeddingData{}, Model: "voyage-3"}
		server := newTestServer(t, http.StatusOK, resp)
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		if emb := p.EmbedQuery(context.Background(), "hello"); emb != nil {
			t.Errorf("expected nil, got %v", emb)
		}
	})

	t.Run("nil on dimension mismatch", func(t *testing.T) {
		resp := embeddingResponse{
			Object: "list",
			Data: []embeddingData{
				{Object: "embedding", Embedding: makeEmbedding(512), Index: 0},
			},
			Model: "voyage-3",
		}
		server := newTestServer(t, http.StatusOK, resp)
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		if emb := p.EmbedQuery(context.Background(), "hello"); emb != nil {
			t.Errorf("expected nil, got %v", emb)
		}
	})

	t.Run("nil on invalid json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{bad"))
		}))
		defer server.Close()

		p, _ := NewVoyageAIEmbeddingProvider("test-key", "voyage-3", 768)
		p.baseURL = server.URL

		if emb := p.EmbedQuery(context.Background(), "hello"); emb != nil {
			t.Errorf("expected nil, got %v", emb)
		}
	})
}
