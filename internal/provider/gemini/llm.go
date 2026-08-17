package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hackastak/repog/internal/provider"
)

// GeminiLLMProvider implements the LLMProvider interface for Gemini
type GeminiLLMProvider struct {
	apiKey        string
	model         string
	fallbackModel string
	baseURL       string
}

// NewGeminiLLMProvider creates a new Gemini LLM provider
func NewGeminiLLMProvider(apiKey, model, fallbackModel string) (*GeminiLLMProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}
	if fallbackModel == "" {
		// The fallback is tried only when the primary returns 404 (model name not
		// found), so it must be an older, widely-available model, not a newer one.
		fallbackModel = "gemini-2.0-flash"
	}

	return &GeminiLLMProvider{
		apiKey:        apiKey,
		model:         model,
		fallbackModel: fallbackModel,
		baseURL:       defaultBaseURL,
	}, nil
}

// Name returns the provider name
func (g *GeminiLLMProvider) Name() string {
	return "gemini"
}

// Validate tests the provider connection
func (g *GeminiLLMProvider) Validate(ctx context.Context) error {
	// Make a minimal test call
	req := provider.LLMRequest{
		Prompt:      "test",
		MaxTokens:   10,
		Temperature: 0.3,
	}
	_, err := g.Call(ctx, req)
	if err != nil {
		return fmt.Errorf("validation failed: %s", err.Message)
	}
	return nil
}

// Call makes a non-streaming LLM request
func (g *GeminiLLMProvider) Call(ctx context.Context, req provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	if req.Temperature == 0 {
		req.Temperature = 0.3
	}

	// Build request body
	body := generateContentRequest{
		Contents: []content{
			{
				Role:  "user",
				Parts: []textPart{{Text: req.Prompt}},
			},
		},
		GenerationConfig: generationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
		},
	}

	if req.SystemPrompt != "" {
		body.SystemInstruction = &systemInstruction{
			Parts: []textPart{{Text: req.SystemPrompt}},
		}
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, &provider.LLMError{
			Message:    err.Error(),
			StatusCode: 0,
		}
	}

	// Try primary model, fall back if 404
	models := []string{g.model, g.fallbackModel}
	client := &http.Client{Timeout: 120 * time.Second}

	var lastError *provider.LLMError
	for _, model := range models {
		url := fmt.Sprintf("%s/models/%s:generateContent", g.baseURL, model)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}
		httpReq.Header.Set("Content-Type", "application/json")
		// Pass the key as a header, not a query param, so it can't leak into a
		// *url.Error message that gets rendered to the terminal or TUI.
		httpReq.Header.Set("x-goog-api-key", g.apiKey)

		resp, err := provider.DoWithRetry(ctx, client, httpReq)
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: resp.StatusCode,
			}
		}

		// If 404, try next model
		if resp.StatusCode == 404 {
			lastError = &provider.LLMError{
				Message:    fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
				StatusCode: 404,
			}
			continue
		}

		if resp.StatusCode != 200 {
			return nil, &provider.LLMError{
				Message:    fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
				StatusCode: resp.StatusCode,
			}
		}

		var genResp generateContentResponse
		if err := json.Unmarshal(respBody, &genResp); err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		// Extract text from response
		var text string
		if len(genResp.Candidates) > 0 && len(genResp.Candidates[0].Content.Parts) > 0 {
			text = genResp.Candidates[0].Content.Parts[0].Text
		}

		return &provider.LLMResult{
			Text:         text,
			InputTokens:  genResp.UsageMetadata.PromptTokenCount,
			OutputTokens: genResp.UsageMetadata.CandidatesTokenCount,
		}, nil
	}

	// All models returned 404
	return nil, lastError
}

// Stream makes a streaming LLM request
func (g *GeminiLLMProvider) Stream(ctx context.Context, req provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	if req.Temperature == 0 {
		req.Temperature = 0.3
	}

	// Build request body
	body := generateContentRequest{
		Contents: []content{
			{
				Role:  "user",
				Parts: []textPart{{Text: req.Prompt}},
			},
		},
		GenerationConfig: generationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
		},
	}

	if req.SystemPrompt != "" {
		body.SystemInstruction = &systemInstruction{
			Parts: []textPart{{Text: req.SystemPrompt}},
		}
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, &provider.LLMError{
			Message:    err.Error(),
			StatusCode: 0,
		}
	}

	// Try primary model, fall back if 404
	models := []string{g.model, g.fallbackModel}
	client := &http.Client{Timeout: 300 * time.Second}

	var lastError *provider.LLMError
	for _, model := range models {
		url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", g.baseURL, model)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-goog-api-key", g.apiKey)

		resp, err := provider.DoWithRetry(ctx, client, httpReq)
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		// If 404, try next model
		if resp.StatusCode == 404 {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastError = &provider.LLMError{
				Message:    fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
				StatusCode: 404,
			}
			continue
		}

		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return nil, &provider.LLMError{
				Message:    fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
				StatusCode: resp.StatusCode,
			}
		}

		// Parse SSE stream
		var fullText strings.Builder
		var inputTokens, outputTokens int

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				jsonData := strings.TrimPrefix(line, "data: ")
				if jsonData == "" {
					continue
				}

				var chunk streamChunk
				if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
					continue
				}

				// Extract text from chunk
				if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
					text := chunk.Candidates[0].Content.Parts[0].Text
					if text != "" {
						fullText.WriteString(text)
						if onChunk != nil {
							onChunk(text)
						}
					}
				}

				// Update token counts (they accumulate in the stream)
				if chunk.UsageMetadata.PromptTokenCount > 0 {
					inputTokens = chunk.UsageMetadata.PromptTokenCount
				}
				if chunk.UsageMetadata.CandidatesTokenCount > 0 {
					outputTokens = chunk.UsageMetadata.CandidatesTokenCount
				}
			}
		}

		_ = resp.Body.Close()

		if err := scanner.Err(); err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		return &provider.LLMResult{
			Text:         fullText.String(),
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		}, nil
	}

	// All models returned 404
	return nil, lastError
}

// Request/response types for Gemini API

type generateContentRequest struct {
	SystemInstruction *systemInstruction `json:"system_instruction,omitempty"`
	Contents          []content          `json:"contents"`
	GenerationConfig  generationConfig   `json:"generationConfig"`
}

type systemInstruction struct {
	Parts []textPart `json:"parts"`
}

type content struct {
	Role  string     `json:"role"`
	Parts []textPart `json:"parts"`
}

type generationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens"`
	Temperature     float32 `json:"temperature"`
}

type generateContentResponse struct {
	Candidates    []candidate   `json:"candidates"`
	UsageMetadata usageMetadata `json:"usageMetadata"`
}

type candidate struct {
	Content content `json:"content"`
}

type usageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
}

type streamChunk struct {
	Candidates    []candidate   `json:"candidates"`
	UsageMetadata usageMetadata `json:"usageMetadata"`
}
