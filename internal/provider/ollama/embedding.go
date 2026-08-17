package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/provider"
)

func init() {
	// Register Ollama providers
	provider.RegisterEmbeddingProvider("ollama", func(cfg config.ProviderConfig, apiKey string) (provider.EmbeddingProvider, error) {
		return NewOllamaEmbeddingProvider(cfg.Model, cfg.Dimensions, cfg.BaseURL)
	})
	provider.RegisterLLMProvider("ollama", func(cfg config.ProviderConfig, apiKey string) (provider.LLMProvider, error) {
		return NewOllamaLLMProvider(cfg.Model, cfg.Fallback, cfg.BaseURL)
	})
}

const defaultBaseURL = "http://localhost:11434"

// OllamaModelSpec holds default specifications for an Ollama embedding model
type OllamaModelSpec struct {
	Dimensions int
	MaxTokens  int
}

// ollamaModelDefaults contains defaults for popular Ollama embedding models.
// Token limits are based on model documentation and HuggingFace model cards.
var ollamaModelDefaults = map[string]OllamaModelSpec{
	// nomic-embed-text variants: 8k context, 768 dimensions
	"nomic-embed-text":        {Dimensions: 768, MaxTokens: 8192},
	"nomic-embed-text:latest": {Dimensions: 768, MaxTokens: 8192},
	"nomic-embed-text:v1":     {Dimensions: 768, MaxTokens: 8192},
	"nomic-embed-text:v1.5":   {Dimensions: 768, MaxTokens: 8192},

	// mxbai-embed-large: mixedbread.ai large model, 512 context
	"mxbai-embed-large":        {Dimensions: 1024, MaxTokens: 512},
	"mxbai-embed-large:latest": {Dimensions: 1024, MaxTokens: 512},

	// all-minilm: Lightweight model, optimized for 256 tokens
	"all-minilm":        {Dimensions: 384, MaxTokens: 256},
	"all-minilm:latest": {Dimensions: 384, MaxTokens: 256},
	"all-minilm:l6-v2":  {Dimensions: 384, MaxTokens: 256},

	// snowflake-arctic-embed v1 variants: 512 context
	"snowflake-arctic-embed":        {Dimensions: 1024, MaxTokens: 512},
	"snowflake-arctic-embed:l":      {Dimensions: 1024, MaxTokens: 512},
	"snowflake-arctic-embed:m":      {Dimensions: 768, MaxTokens: 512},
	"snowflake-arctic-embed:s":      {Dimensions: 384, MaxTokens: 512},
	"snowflake-arctic-embed:latest": {Dimensions: 1024, MaxTokens: 512},

	// snowflake-arctic-embed v2 variants: 8k context with RoPE
	"snowflake-arctic-embed:l-v2.0": {Dimensions: 1024, MaxTokens: 8192},
	"snowflake-arctic-embed:m-v2.0": {Dimensions: 768, MaxTokens: 8192},
	"snowflake-arctic-embed2":       {Dimensions: 1024, MaxTokens: 8192},
	"snowflake-arctic-embed-l-v2.0": {Dimensions: 1024, MaxTokens: 8192},
	"snowflake-arctic-embed-m-v2.0": {Dimensions: 768, MaxTokens: 8192},
	"snowflake-arctic-embed:m-long": {Dimensions: 768, MaxTokens: 2048},

	// BGE models from BAAI
	"bge-m3":            {Dimensions: 1024, MaxTokens: 8192},
	"bge-m3:latest":     {Dimensions: 1024, MaxTokens: 8192},
	"bge-large":         {Dimensions: 1024, MaxTokens: 512},
	"bge-large:latest":  {Dimensions: 1024, MaxTokens: 512},
	"bge-large:en-v1.5": {Dimensions: 1024, MaxTokens: 512},
	"bge-base":          {Dimensions: 768, MaxTokens: 512},
	"bge-base:en-v1.5":  {Dimensions: 768, MaxTokens: 512},
	"bge-small":         {Dimensions: 384, MaxTokens: 512},
	"bge-small:en-v1.5": {Dimensions: 384, MaxTokens: 512},

	// E5 models
	"e5-mistral-7b-instruct": {Dimensions: 4096, MaxTokens: 32768},

	// Jina embeddings v2: 8k context
	"jina-embeddings-v2-base-en":  {Dimensions: 768, MaxTokens: 8192},
	"jina-embeddings-v2-small-en": {Dimensions: 512, MaxTokens: 8192},

	// Granite embedding
	"granite-embedding":        {Dimensions: 1024, MaxTokens: 512},
	"granite-embedding:latest": {Dimensions: 1024, MaxTokens: 512},

	// Paraphrase models
	"paraphrase-multilingual": {Dimensions: 768, MaxTokens: 512},
}

// defaultModelSpec is used for unknown models
var defaultModelSpec = OllamaModelSpec{Dimensions: 768, MaxTokens: 2048}

// getModelSpec returns the spec for a model, falling back to defaults
func getModelSpec(model string) OllamaModelSpec {
	if spec, ok := ollamaModelDefaults[model]; ok {
		return spec
	}
	return defaultModelSpec
}

// OllamaEmbeddingProvider implements the EmbeddingProvider interface for Ollama
type OllamaEmbeddingProvider struct {
	model      string
	dimensions int
	batchSize  int
	baseURL    string
}

// NewOllamaEmbeddingProvider creates a new Ollama embedding provider
func NewOllamaEmbeddingProvider(model string, dimensions int, baseURL string) (*OllamaEmbeddingProvider, error) {
	if model == "" {
		model = "nomic-embed-text" // Default embedding model
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if dimensions == 0 {
		// Use model spec defaults
		spec := getModelSpec(model)
		dimensions = spec.Dimensions
	}

	return &OllamaEmbeddingProvider{
		model:      model,
		dimensions: dimensions,
		batchSize:  10, // Conservative batch size for local processing
		baseURL:    baseURL,
	}, nil
}

// Name returns the provider name
func (o *OllamaEmbeddingProvider) Name() string {
	return "ollama"
}

// Dimensions returns the embedding vector dimensions
func (o *OllamaEmbeddingProvider) Dimensions() int {
	return o.dimensions
}

// BatchSize returns the maximum batch size
func (o *OllamaEmbeddingProvider) BatchSize() int {
	return o.batchSize
}

// MaxTokens returns the maximum token limit for the model
func (o *OllamaEmbeddingProvider) MaxTokens() int {
	spec := getModelSpec(o.model)
	return spec.MaxTokens
}

// Validate tests the provider connection
func (o *OllamaEmbeddingProvider) Validate(ctx context.Context) error {
	// Make a test embed call with minimal content
	result := o.EmbedQuery(ctx, "test")
	if result == nil {
		return fmt.Errorf("validation failed: could not embed test query (is Ollama running?)")
	}
	return nil
}

// EmbedChunks embeds multiple chunks in a batch
func (o *OllamaEmbeddingProvider) EmbedChunks(ctx context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
	result := provider.BatchEmbedResult{
		Results: make([]provider.EmbedResult, 0),
		Errors:  0,
	}

	if len(chunks) == 0 {
		return result
	}

	// Ollama doesn't support batch embeddings, so we process one at a time
	for _, chunk := range chunks {
		embedding := o.EmbedQuery(ctx, chunk.Content)
		if embedding == nil {
			result.Errors++
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     fmt.Errorf("failed to embed chunk"),
			})
		} else {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: embedding,
				Error:     nil,
			})
		}
	}

	return result
}

// EmbedQuery embeds a single query for search
func (o *OllamaEmbeddingProvider) EmbedQuery(ctx context.Context, query string) []float32 {
	reqBody := embeddingRequest{
		Model:  o.model,
		Prompt: query,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}

	url := fmt.Sprintf("%s/api/embeddings", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second} // Longer timeout for local processing
	resp, err := provider.DoWithRetry(ctx, client, req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var embedResp embeddingResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil
	}

	if len(embedResp.Embedding) == 0 {
		return nil
	}

	// Ollama returns float64, convert to float32
	embedding := make([]float32, len(embedResp.Embedding))
	for i, v := range embedResp.Embedding {
		embedding[i] = float32(v)
	}

	// Verify dimensions match
	if len(embedding) != o.dimensions {
		// Update dimensions if this is the first call
		o.dimensions = len(embedding)
	}

	return embedding
}

// Request/response types for Ollama API

type embeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}
