package provider

import (
	"fmt"

	"github.com/hackastak/repog/internal/config"
)

// EmbeddingProviderFactory is a function that creates an EmbeddingProvider
type EmbeddingProviderFactory func(cfg config.ProviderConfig, apiKey string) (EmbeddingProvider, error)

// LLMProviderFactory is a function that creates an LLMProvider
type LLMProviderFactory func(cfg config.ProviderConfig, apiKey string) (LLMProvider, error)

var (
	embeddingProviders = make(map[string]EmbeddingProviderFactory)
	llmProviders       = make(map[string]LLMProviderFactory)
)

// RegisterEmbeddingProvider registers an embedding provider factory
func RegisterEmbeddingProvider(name string, factory EmbeddingProviderFactory) {
	embeddingProviders[name] = factory
}

// RegisterLLMProvider registers an LLM provider factory
func RegisterLLMProvider(name string, factory LLMProviderFactory) {
	llmProviders[name] = factory
}

// maxTokensOverrideWrapper wraps an EmbeddingProvider to override MaxTokens
type maxTokensOverrideWrapper struct {
	EmbeddingProvider
	customMaxTokens int
}

func (w *maxTokensOverrideWrapper) MaxTokens() int {
	return w.customMaxTokens
}

// NewEmbeddingProvider creates an EmbeddingProvider based on config
// If cfg.MaxTokens is set (> 0), it wraps the provider to use the custom value
func NewEmbeddingProvider(cfg config.ProviderConfig, apiKey string) (EmbeddingProvider, error) {
	factory, ok := embeddingProviders[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
	provider, err := factory(cfg, apiKey)
	if err != nil {
		return nil, err
	}

	// If custom MaxTokens is set, wrap the provider
	if cfg.MaxTokens > 0 {
		return &maxTokensOverrideWrapper{
			EmbeddingProvider: provider,
			customMaxTokens:   cfg.MaxTokens,
		}, nil
	}

	return provider, nil
}

// GetModelDefaultMaxTokens returns the default MaxTokens for a model without applying config overrides.
// This is useful for showing the default value during configuration.
func GetModelDefaultMaxTokens(cfg config.ProviderConfig, apiKey string) (int, error) {
	factory, ok := embeddingProviders[cfg.Provider]
	if !ok {
		return 0, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
	// Create provider without the MaxTokens override to get model default
	cfgCopy := cfg
	cfgCopy.MaxTokens = 0
	provider, err := factory(cfgCopy, apiKey)
	if err != nil {
		return 0, err
	}
	return provider.MaxTokens(), nil
}

// Ensure the wrapper implements EmbeddingProvider. Every method other than
// MaxTokens is promoted from the embedded interface.
var _ EmbeddingProvider = (*maxTokensOverrideWrapper)(nil)

// NewLLMProvider creates an LLMProvider based on config
func NewLLMProvider(cfg config.ProviderConfig, apiKey string) (LLMProvider, error) {
	factory, ok := llmProviders[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.Provider)
	}
	return factory(cfg, apiKey)
}
