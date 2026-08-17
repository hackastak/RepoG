package gemini

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
	// Register Gemini providers
	provider.RegisterEmbeddingProvider("gemini", func(cfg config.ProviderConfig, apiKey string) (provider.EmbeddingProvider, error) {
		return NewGeminiEmbeddingProvider(apiKey, cfg.Model, cfg.Dimensions)
	})
	provider.RegisterLLMProvider("gemini", func(cfg config.ProviderConfig, apiKey string) (provider.LLMProvider, error) {
		return NewGeminiLLMProvider(apiKey, cfg.Model, cfg.Fallback)
	})
}

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// GeminiModelSpec holds default specifications for a Gemini embedding model
type GeminiModelSpec struct {
	Dimensions int
	MaxTokens  int
}

// geminiModelDefaults contains defaults for Gemini embedding models.
// Based on Google AI documentation: https://ai.google.dev/gemini-api/docs/models
var geminiModelDefaults = map[string]GeminiModelSpec{
	// Gemini Embedding 2.0 (multimodal, 8k context)
	// Native 3072 dims, but supports variable output via OutputDimensionality
	"gemini-embedding-2-preview": {Dimensions: 3072, MaxTokens: 8192},

	// Gemini Embedding 001 (text-only, 2k context)
	"gemini-embedding-001": {Dimensions: 3072, MaxTokens: 2048},

	// Text Embedding 004 (legacy)
	"text-embedding-004": {Dimensions: 768, MaxTokens: 2048},

	// Legacy embedding-001
	"embedding-001": {Dimensions: 768, MaxTokens: 2048},
}

// defaultGeminiModelSpec is used for unknown models
var defaultGeminiModelSpec = GeminiModelSpec{Dimensions: 3072, MaxTokens: 8192}

// getGeminiModelSpec returns the spec for a model, falling back to defaults
func getGeminiModelSpec(model string) GeminiModelSpec {
	if spec, ok := geminiModelDefaults[model]; ok {
		return spec
	}
	return defaultGeminiModelSpec
}

// GeminiEmbeddingProvider implements the EmbeddingProvider interface for Gemini
type GeminiEmbeddingProvider struct {
	apiKey     string
	model      string
	dimensions int
	batchSize  int
	baseURL    string
}

// NewGeminiEmbeddingProvider creates a new Gemini embedding provider
func NewGeminiEmbeddingProvider(apiKey, model string, dimensions int) (*GeminiEmbeddingProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = "gemini-embedding-2-preview"
	}
	if dimensions == 0 {
		// Use model spec defaults
		spec := getGeminiModelSpec(model)
		dimensions = spec.Dimensions
	}

	return &GeminiEmbeddingProvider{
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		batchSize:  20,
		baseURL:    defaultBaseURL,
	}, nil
}

// Name returns the provider name
func (g *GeminiEmbeddingProvider) Name() string {
	return "gemini"
}

// Dimensions returns the embedding vector dimensions
func (g *GeminiEmbeddingProvider) Dimensions() int {
	return g.dimensions
}

// BatchSize returns the maximum batch size
func (g *GeminiEmbeddingProvider) BatchSize() int {
	return g.batchSize
}

// MaxTokens returns the maximum token limit for the model
func (g *GeminiEmbeddingProvider) MaxTokens() int {
	spec := getGeminiModelSpec(g.model)
	return spec.MaxTokens
}

// Validate tests the provider connection
func (g *GeminiEmbeddingProvider) Validate(ctx context.Context) error {
	// Make a test embed call with minimal content
	result := g.EmbedQuery(ctx, "test")
	if result == nil {
		return fmt.Errorf("validation failed: could not embed test query")
	}
	return nil
}

// EmbedChunks embeds multiple chunks in a batch
func (g *GeminiEmbeddingProvider) EmbedChunks(ctx context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
	result := provider.BatchEmbedResult{
		Results: make([]provider.EmbedResult, 0),
		Errors:  0,
	}

	if len(chunks) == 0 {
		return result
	}

	// Build batch request
	requests := make([]embedContentRequest, len(chunks))
	for i, chunk := range chunks {
		requests[i] = embedContentRequest{
			Model: "models/" + g.model,
			Content: contentParts{
				Parts: []textPart{{Text: chunk.Content}},
			},
			TaskType:             "RETRIEVAL_DOCUMENT",
			OutputDimensionality: g.dimensions,
		}
	}

	reqBody := batchEmbedRequest{Requests: requests}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		result.Errors = len(chunks)
		// Return error results for all chunks
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
	url := fmt.Sprintf("%s/models/%s:batchEmbedContents", g.baseURL, g.model)
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
	// Key goes in a header, not the URL, so a *url.Error can't leak it.
	req.Header.Set("x-goog-api-key", g.apiKey)

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

	var embedResp batchEmbedResponse
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

	// Process results
	for i, chunk := range chunks {
		if i >= len(embedResp.Embeddings) {
			result.Errors++
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     fmt.Errorf("no embedding returned from API"),
			})
			continue
		}

		embedding := embedResp.Embeddings[i].Values
		if len(embedding) != g.dimensions {
			result.Errors++
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     fmt.Errorf("invalid dimensions: expected %d, got %d", g.dimensions, len(embedding)),
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
func (g *GeminiEmbeddingProvider) EmbedQuery(ctx context.Context, query string) []float32 {
	reqBody := embedQueryRequest{
		Model: "models/" + g.model,
		Content: contentParts{
			Parts: []textPart{{Text: query}},
		},
		TaskType:             "RETRIEVAL_QUERY",
		OutputDimensionality: g.dimensions,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}

	url := fmt.Sprintf("%s/models/%s:embedContent", g.baseURL, g.model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)

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

	var embedResp embedQueryResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil
	}

	if len(embedResp.Embedding.Values) != g.dimensions {
		return nil
	}

	return embedResp.Embedding.Values
}

// Request/response types for Gemini API

type batchEmbedRequest struct {
	Requests []embedContentRequest `json:"requests"`
}

type embedContentRequest struct {
	Model                string       `json:"model"`
	Content              contentParts `json:"content"`
	TaskType             string       `json:"taskType"`
	OutputDimensionality int          `json:"outputDimensionality,omitempty"`
}

type contentParts struct {
	Parts []textPart `json:"parts"`
}

type textPart struct {
	Text string `json:"text"`
}

type batchEmbedResponse struct {
	Embeddings []embeddingData `json:"embeddings"`
}

type embeddingData struct {
	Values []float32 `json:"values"`
}

type embedQueryRequest struct {
	Model                string       `json:"model"`
	Content              contentParts `json:"content"`
	TaskType             string       `json:"taskType"`
	OutputDimensionality int          `json:"outputDimensionality,omitempty"`
}

type embedQueryResponse struct {
	Embedding embeddingData `json:"embedding"`
}
