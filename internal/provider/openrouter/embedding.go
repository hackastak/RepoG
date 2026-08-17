package openrouter

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
	// Register OpenRouter providers
	provider.RegisterEmbeddingProvider("openrouter", func(cfg config.ProviderConfig, apiKey string) (provider.EmbeddingProvider, error) {
		return NewOpenRouterEmbeddingProvider(apiKey, cfg.Model, cfg.Dimensions)
	})
	provider.RegisterLLMProvider("openrouter", func(cfg config.ProviderConfig, apiKey string) (provider.LLMProvider, error) {
		return NewOpenRouterLLMProvider(apiKey, cfg.Model, cfg.Fallback)
	})
}

const defaultBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterModelSpec holds default specifications for an OpenRouter embedding model
type OpenRouterModelSpec struct {
	Dimensions int
	MaxTokens  int
}

// openrouterModelDefaults contains defaults for embedding models available via OpenRouter.
// OpenRouter routes to various providers, so specs match the underlying models.
var openrouterModelDefaults = map[string]OpenRouterModelSpec{
	// OpenAI models via OpenRouter
	"openai/text-embedding-3-small": {Dimensions: 1536, MaxTokens: 8191},
	"openai/text-embedding-3-large": {Dimensions: 3072, MaxTokens: 8191},
	"openai/text-embedding-ada-002": {Dimensions: 1536, MaxTokens: 8191},

	// Cohere models via OpenRouter
	"cohere/embed-english-v3.0":       {Dimensions: 1024, MaxTokens: 512},
	"cohere/embed-multilingual-v3.0":  {Dimensions: 1024, MaxTokens: 512},
	"cohere/embed-english-light-v3.0": {Dimensions: 384, MaxTokens: 512},

	// Voyage AI models via OpenRouter
	"voyageai/voyage-3":      {Dimensions: 1024, MaxTokens: 32000},
	"voyageai/voyage-code-3": {Dimensions: 1024, MaxTokens: 16000},
}

// defaultOpenRouterModelSpec is used for unknown models
var defaultOpenRouterModelSpec = OpenRouterModelSpec{Dimensions: 1536, MaxTokens: 8191}

// getOpenRouterModelSpec returns the spec for a model, falling back to defaults
func getOpenRouterModelSpec(model string) OpenRouterModelSpec {
	if spec, ok := openrouterModelDefaults[model]; ok {
		return spec
	}
	return defaultOpenRouterModelSpec
}

// OpenRouterEmbeddingProvider implements the EmbeddingProvider interface for OpenRouter
type OpenRouterEmbeddingProvider struct {
	apiKey     string
	model      string
	dimensions int
	batchSize  int
	baseURL    string
}

// NewOpenRouterEmbeddingProvider creates a new OpenRouter embedding provider
func NewOpenRouterEmbeddingProvider(apiKey, model string, dimensions int) (*OpenRouterEmbeddingProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = "openai/text-embedding-3-small" // Default model
	}
	if dimensions == 0 {
		// Use model spec defaults
		spec := getOpenRouterModelSpec(model)
		dimensions = spec.Dimensions
	}

	return &OpenRouterEmbeddingProvider{
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		batchSize:  100, // OpenRouter supports large batches like OpenAI
		baseURL:    defaultBaseURL,
	}, nil
}

// Name returns the provider name
func (o *OpenRouterEmbeddingProvider) Name() string {
	return "openrouter"
}

// Dimensions returns the embedding vector dimensions
func (o *OpenRouterEmbeddingProvider) Dimensions() int {
	return o.dimensions
}

// BatchSize returns the maximum batch size
func (o *OpenRouterEmbeddingProvider) BatchSize() int {
	return o.batchSize
}

// MaxTokens returns the maximum token limit for the model
func (o *OpenRouterEmbeddingProvider) MaxTokens() int {
	spec := getOpenRouterModelSpec(o.model)
	return spec.MaxTokens
}

// Validate tests the provider connection
func (o *OpenRouterEmbeddingProvider) Validate(ctx context.Context) error {
	// Make a test embed call with minimal content
	result := o.EmbedQuery(ctx, "test")
	if result == nil {
		return fmt.Errorf("validation failed: could not embed test query")
	}
	return nil
}

// EmbedChunks embeds multiple chunks in a batch
func (o *OpenRouterEmbeddingProvider) EmbedChunks(ctx context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
	result := provider.BatchEmbedResult{
		Results: make([]provider.EmbedResult, 0),
		Errors:  0,
	}

	if len(chunks) == 0 {
		return result
	}

	// Build request with array of strings
	inputs := make([]string, len(chunks))
	for i, chunk := range chunks {
		inputs[i] = chunk.Content
	}

	// Note: OpenRouter may not support dimensions parameter for some models
	// Try without it first
	reqBody := embeddingRequest{
		Input: inputs,
		Model: o.model,
		// Dimensions: o.dimensions, // Commenting out to test if this causes 404
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	// Make HTTP request
	url := fmt.Sprintf("%s/embeddings", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("X-Title", "RepoG")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := provider.DoWithRetry(ctx, client, req)
	if err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	if resp.StatusCode != 200 {
		// Try to parse error response for better error message
		var errResp embeddingResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil {
			// Log chunk details for debugging
			var chunkLengths []int
			for _, chunk := range chunks {
				chunkLengths = append(chunkLengths, len(chunk.Content))
			}
			err := fmt.Errorf("OpenRouter API error: %s (code: %v, status: %s, model: %s, batch_size: %d, chunk_lengths: %v)",
				errResp.Error.Message, errResp.Error.Code, resp.Status, o.model, len(chunks), chunkLengths)
			result.Errors = len(chunks)
			for _, chunk := range chunks {
				result.Results = append(result.Results, provider.EmbedResult{
					ID:        chunk.ID,
					Embedding: nil,
					Error:     err,
				})
			}
			return result
		}
		// Fall back to raw response body
		err := fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	var embedResp embeddingResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	// Check for API errors in response
	if embedResp.Error != nil {
		err := fmt.Errorf("API error: %s (code: %v)", embedResp.Error.Message, embedResp.Error.Code)
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	// Process results - OpenRouter returns embeddings in order
	for i, chunk := range chunks {
		if i >= len(embedResp.Data) {
			result.Errors++
			missingErr := fmt.Errorf("API returned %d embeddings for %d chunks (model: %s)", len(embedResp.Data), len(chunks), o.model)
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     missingErr,
			})
			continue
		}

		embedding := embedResp.Data[i].Embedding
		if len(embedding) != o.dimensions {
			result.Errors++
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     fmt.Errorf("invalid dimensions: expected %d, got %d", o.dimensions, len(embedding)),
			})
			continue
		}

		result.Results = append(result.Results, provider.EmbedResult{
			ID:        chunk.ID,
			Embedding: embedding,
			Error:     nil,
		})
	}

	return result
}

// EmbedQuery embeds a single query for search
func (o *OpenRouterEmbeddingProvider) EmbedQuery(ctx context.Context, query string) []float32 {
	reqBody := embeddingRequest{
		Input:      []string{query},
		Model:      o.model,
		Dimensions: o.dimensions,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}

	url := fmt.Sprintf("%s/embeddings", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("X-Title", "RepoG")

	client := &http.Client{Timeout: 30 * time.Second}
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

	if len(embedResp.Data) == 0 || len(embedResp.Data[0].Embedding) != o.dimensions {
		return nil
	}

	return embedResp.Data[0].Embedding
}

// Request/response types for OpenRouter API

type embeddingRequest struct {
	Input      []string `json:"input"`
	Model      string   `json:"model"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Usage usageInfo       `json:"usage"`
	Error *apiError       `json:"error,omitempty"`
}

type apiError struct {
	Message string      `json:"message"`
	Code    interface{} `json:"code"` // Can be string or number
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type usageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
