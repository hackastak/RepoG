package provider_test

import (
	"context"
	"testing"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/provider"
	_ "github.com/hackastak/repog/internal/provider/gemini"
	_ "github.com/hackastak/repog/internal/provider/ollama"
	_ "github.com/hackastak/repog/internal/provider/openai"
	_ "github.com/hackastak/repog/internal/provider/openrouter"
)

func TestEmbeddingProviderRegistration(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		dims     int
	}{
		{"Gemini", "gemini", "gemini-embedding-2-preview", 768},
		{"OpenAI", "openai", "text-embedding-3-small", 1536},
		{"OpenRouter", "openrouter", "openai/text-embedding-3-small", 1536},
		{"Ollama", "ollama", "nomic-embed-text", 768},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ProviderConfig{
				Provider:   tt.provider,
				Model:      tt.model,
				Dimensions: tt.dims,
			}

			p, err := provider.NewEmbeddingProvider(cfg, "fake-api-key")
			if err != nil {
				t.Fatalf("Failed to create %s provider: %v", tt.provider, err)
			}

			if p.Name() != tt.provider {
				t.Errorf("Expected provider name %s, got %s", tt.provider, p.Name())
			}

			if p.Dimensions() != tt.dims {
				t.Errorf("Expected dimensions %d, got %d", tt.dims, p.Dimensions())
			}

			if p.BatchSize() == 0 {
				t.Errorf("BatchSize should be > 0")
			}
		})
	}
}

func TestLLMProviderRegistration(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		fallback string
	}{
		{"Gemini", "gemini", "gemini-2.5-flash", "gemini-3.0-flash"},
		{"OpenAI", "openai", "gpt-4o", "gpt-3.5-turbo"},
		{"OpenRouter", "openrouter", "openai/gpt-4o", "openai/gpt-3.5-turbo"},
		{"Ollama", "ollama", "llama3.2", "llama2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ProviderConfig{
				Provider: tt.provider,
				Model:    tt.model,
				Fallback: tt.fallback,
			}

			p, err := provider.NewLLMProvider(cfg, "fake-api-key")
			if err != nil {
				t.Fatalf("Failed to create %s provider: %v", tt.provider, err)
			}

			if p.Name() != tt.provider {
				t.Errorf("Expected provider name %s, got %s", tt.provider, p.Name())
			}
		})
	}
}

func TestUnknownProvider(t *testing.T) {
	cfg := config.ProviderConfig{
		Provider:   "unknown",
		Model:      "test",
		Dimensions: 768,
	}

	_, err := provider.NewEmbeddingProvider(cfg, "fake-key")
	if err == nil {
		t.Error("Expected error for unknown provider, got nil")
	}

	_, err = provider.NewLLMProvider(cfg, "fake-key")
	if err == nil {
		t.Error("Expected error for unknown provider, got nil")
	}
}

// When cfg.MaxTokens is set, NewEmbeddingProvider returns a wrapper that overrides
// MaxTokens and inherits the rest from the embedded provider by method promotion.
// These tests pin that every non-overridden method still reaches the wrapped
// provider, so the promotion can't silently regress into a no-op or a zero value.
func TestMaxTokensOverride(t *testing.T) {
	const overrideTokens = 4096

	inner := provider.NewMockEmbeddingProvider() // name "mock", 768 dims, batch 20, 8192 tokens
	var queriedWith string
	inner.QueryFunc = func(_ context.Context, query string) []float32 {
		queriedWith = query
		return []float32{1, 2, 3}
	}

	provider.RegisterEmbeddingProvider("wrappertest", func(_ config.ProviderConfig, _ string) (provider.EmbeddingProvider, error) {
		return inner, nil
	})

	cfg := config.ProviderConfig{Provider: "wrappertest", Model: "test", Dimensions: 768, MaxTokens: overrideTokens}
	wrapped, err := provider.NewEmbeddingProvider(cfg, "fake-key")
	if err != nil {
		t.Fatalf("NewEmbeddingProvider() error = %v", err)
	}

	// The one method the wrapper exists to change.
	if got := wrapped.MaxTokens(); got != overrideTokens {
		t.Errorf("MaxTokens() = %d, want %d (config override)", got, overrideTokens)
	}

	// Everything else must still reach the wrapped provider.
	if got := wrapped.Name(); got != "mock" {
		t.Errorf("Name() = %q, want %q (promoted from embedded provider)", got, "mock")
	}
	if got := wrapped.Dimensions(); got != 768 {
		t.Errorf("Dimensions() = %d, want 768 (promoted)", got)
	}
	if got := wrapped.BatchSize(); got != 20 {
		t.Errorf("BatchSize() = %d, want 20 (promoted)", got)
	}
	if err := wrapped.Validate(context.Background()); err != nil {
		t.Errorf("Validate() = %v, want nil (promoted)", err)
	}

	if got := wrapped.EmbedQuery(context.Background(), "hello"); len(got) != 3 {
		t.Errorf("EmbedQuery() returned %d dims, want 3 from the wrapped provider", len(got))
	}
	if queriedWith != "hello" {
		t.Errorf("EmbedQuery() passed %q to the wrapped provider, want %q", queriedWith, "hello")
	}

	res := wrapped.EmbedChunks(context.Background(), []provider.EmbedRequest{{ID: 7, Content: "x"}})
	if len(res.Results) != 1 || res.Results[0].ID != 7 {
		t.Errorf("EmbedChunks() = %+v, want one result with ID 7 from the wrapped provider", res.Results)
	}
}

func TestMaxTokensNotOverriddenWhenUnset(t *testing.T) {
	inner := provider.NewMockEmbeddingProvider()
	provider.RegisterEmbeddingProvider("wrappertest-unset", func(_ config.ProviderConfig, _ string) (provider.EmbeddingProvider, error) {
		return inner, nil
	})

	cfg := config.ProviderConfig{Provider: "wrappertest-unset", Model: "test", Dimensions: 768} // MaxTokens left at 0
	p, err := provider.NewEmbeddingProvider(cfg, "fake-key")
	if err != nil {
		t.Fatalf("NewEmbeddingProvider() error = %v", err)
	}

	if got := p.MaxTokens(); got != 8192 {
		t.Errorf("MaxTokens() = %d, want 8192 (model default, unwrapped)", got)
	}
}
