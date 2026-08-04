package voyageai

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
	// Register Voyage AI embedding provider
	provider.RegisterEmbeddingProvider("voyageai", func(cfg config.ProviderConfig, apiKey string) (provider.EmbeddingProvider, error) {
		return NewVoyageAIEmbeddingProvider(apiKey, cfg.Model, cfg.Dimensions)
	})
}

const defaultBaseURL = "https://api.voyageai.com/v1"

// VoyageAIModelSpec holds default specifications for a Voyage AI embedding model
type VoyageAIModelSpec struct {
	Dimensions int
	MaxTokens  int
}

// voyageaiModelDefaults contains defaults for Voyage AI embedding models.
// Based on Voyage AI documentation.
var voyageaiModelDefaults = map[string]VoyageAIModelSpec{
	// Voyage 3 series (latest, 32k context)
	"voyage-3":      {Dimensions: 1024, MaxTokens: 32000},
	"voyage-3-lite": {Dimensions: 512, MaxTokens: 32000},

	// Voyage Code 3 (optimized for code, 16k context)
	"voyage-code-3": {Dimensions: 1024, MaxTokens: 16000},

	// Voyage 2 series (legacy)
	"voyage-2":       {Dimensions: 1024, MaxTokens: 4000},
	"voyage-large-2": {Dimensions: 1536, MaxTokens: 16000},
	"voyage-code-2":  {Dimensions: 1536, MaxTokens: 16000},

	// Domain-specific models
	"voyage-finance-2": {Dimensions: 1024, MaxTokens: 4000},
	"voyage-law-2":     {Dimensions: 1024, MaxTokens: 4000},
}

// defaultVoyageAIModelSpec is used for unknown models
var defaultVoyageAIModelSpec = VoyageAIModelSpec{Dimensions: 1024, MaxTokens: 4000}

// getVoyageAIModelSpec returns the spec for a model, falling back to defaults
func getVoyageAIModelSpec(model string) VoyageAIModelSpec {
	if spec, ok := voyageaiModelDefaults[model]; ok {
		return spec
	}
	return defaultVoyageAIModelSpec
}

// VoyageAIEmbeddingProvider implements the EmbeddingProvider interface for Voyage AI
type VoyageAIEmbeddingProvider struct {
	apiKey     string
	model      string
	dimensions int
	batchSize  int
	baseURL    string
}

// NewVoyageAIEmbeddingProvider creates a new Voyage AI embedding provider
func NewVoyageAIEmbeddingProvider(apiKey, model string, dimensions int) (*VoyageAIEmbeddingProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = "voyage-code-3" // Default model for code embeddings
	}
	if dimensions == 0 {
		// Use model spec defaults
		spec := getVoyageAIModelSpec(model)
		dimensions = spec.Dimensions
	}

	return &VoyageAIEmbeddingProvider{
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		batchSize:  128, // Voyage AI supports large batches
		baseURL:    defaultBaseURL,
	}, nil
}

// Name returns the provider name
func (v *VoyageAIEmbeddingProvider) Name() string {
	return "voyageai"
}

// Dimensions returns the embedding vector dimensions
func (v *VoyageAIEmbeddingProvider) Dimensions() int {
	return v.dimensions
}

// BatchSize returns the maximum batch size
func (v *VoyageAIEmbeddingProvider) BatchSize() int {
	return v.batchSize
}

// MaxTokens returns the maximum token limit for the model
func (v *VoyageAIEmbeddingProvider) MaxTokens() int {
	spec := getVoyageAIModelSpec(v.model)
	return spec.MaxTokens
}

// Validate tests the provider connection
func (v *VoyageAIEmbeddingProvider) Validate(ctx context.Context) error {
	// Make a test embed call with minimal content
	result := v.EmbedQuery(ctx, "test")
	if result == nil {
		return fmt.Errorf("validation failed: could not embed test query")
	}
	return nil
}

// EmbedChunks embeds multiple chunks in a batch
func (v *VoyageAIEmbeddingProvider) EmbedChunks(ctx context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
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

	reqBody := embeddingRequest{
		Input:     inputs,
		Model:     v.model,
		InputType: "document", // For indexing documents
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
	url := fmt.Sprintf("%s/embeddings", v.baseURL)
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
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

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

	// Process results - Voyage AI returns embeddings in order
	for i, chunk := range chunks {
		if i >= len(embedResp.Data) {
			result.Errors++
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     fmt.Errorf("no embedding returned from API"),
			})
			continue
		}

		embedding := embedResp.Data[i].Embedding
		if len(embedding) != v.dimensions {
			result.Errors++
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     fmt.Errorf("invalid dimensions: expected %d, got %d", v.dimensions, len(embedding)),
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
func (v *VoyageAIEmbeddingProvider) EmbedQuery(ctx context.Context, query string) []float32 {
	reqBody := embeddingRequest{
		Input:     []string{query},
		Model:     v.model,
		InputType: "query", // For search queries
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}

	url := fmt.Sprintf("%s/embeddings", v.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

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

	if len(embedResp.Data) == 0 || len(embedResp.Data[0].Embedding) != v.dimensions {
		return nil
	}

	return embedResp.Data[0].Embedding
}

// Request/response types for Voyage AI API

type embeddingRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"` // "query" or "document"
}

type embeddingResponse struct {
	Object string          `json:"object"`
	Data   []embeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  usageInfo       `json:"usage"`
}

type embeddingData struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type usageInfo struct {
	TotalTokens int `json:"total_tokens"`
}
