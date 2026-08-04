package ollama

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

// OllamaLLMProvider implements the LLMProvider interface for Ollama
type OllamaLLMProvider struct {
	model         string
	fallbackModel string
	baseURL       string
}

// NewOllamaLLMProvider creates a new Ollama LLM provider
func NewOllamaLLMProvider(model, fallbackModel, baseURL string) (*OllamaLLMProvider, error) {
	if model == "" {
		model = "llama3.2" // Default model
	}
	if fallbackModel == "" {
		fallbackModel = "llama2" // Default fallback
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &OllamaLLMProvider{
		model:         model,
		fallbackModel: fallbackModel,
		baseURL:       baseURL,
	}, nil
}

// Name returns the provider name
func (o *OllamaLLMProvider) Name() string {
	return "ollama"
}

// Validate tests the provider connection
func (o *OllamaLLMProvider) Validate(ctx context.Context) error {
	// Make a minimal test call
	req := provider.LLMRequest{
		Prompt:      "test",
		MaxTokens:   10,
		Temperature: 0.3,
	}
	_, err := o.Call(ctx, req)
	if err != nil {
		return fmt.Errorf("validation failed: %s (is Ollama running?)", err.Message)
	}
	return nil
}

// Call makes a non-streaming LLM request
func (o *OllamaLLMProvider) Call(ctx context.Context, req provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	if req.Temperature == 0 {
		req.Temperature = 0.3
	}

	// Build messages array
	messages := []message{}
	if req.SystemPrompt != "" {
		messages = append(messages, message{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}
	messages = append(messages, message{
		Role:    "user",
		Content: req.Prompt,
	})

	// Build request body
	body := chatRequest{
		Model:    o.model,
		Messages: messages,
		Options: map[string]interface{}{
			"temperature": req.Temperature,
			"num_predict": req.MaxTokens,
		},
		Stream: false,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, &provider.LLMError{
			Message:    err.Error(),
			StatusCode: 0,
		}
	}

	// Try primary model, fall back if needed
	models := []string{o.model, o.fallbackModel}
	client := &http.Client{Timeout: 300 * time.Second} // Longer timeout for local processing

	var lastError *provider.LLMError
	for _, model := range models {
		// Update model in body if using fallback
		if model != o.model {
			body.Model = model
			bodyBytes, _ = json.Marshal(body)
		}

		url := fmt.Sprintf("%s/api/chat", o.baseURL)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		httpReq.Header.Set("Content-Type", "application/json")

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

		// If 404 or model not found, try next model
		if resp.StatusCode == 404 {
			lastError = &provider.LLMError{
				Message:    fmt.Sprintf("Model not found: %s", model),
				StatusCode: resp.StatusCode,
			}
			continue
		}

		if resp.StatusCode != 200 {
			return nil, &provider.LLMError{
				Message:    fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
				StatusCode: resp.StatusCode,
			}
		}

		var chatResp chatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		// Extract text from response
		text := chatResp.Message.Content

		// Ollama doesn't provide token counts in the same way, approximate them
		inputTokens := len(strings.Fields(req.SystemPrompt + " " + req.Prompt))
		outputTokens := len(strings.Fields(text))

		return &provider.LLMResult{
			Text:         text,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		}, nil
	}

	// All models failed
	return nil, lastError
}

// Stream makes a streaming LLM request
func (o *OllamaLLMProvider) Stream(ctx context.Context, req provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	if req.Temperature == 0 {
		req.Temperature = 0.3
	}

	// Build messages array
	messages := []message{}
	if req.SystemPrompt != "" {
		messages = append(messages, message{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}
	messages = append(messages, message{
		Role:    "user",
		Content: req.Prompt,
	})

	// Build request body
	body := chatRequest{
		Model:    o.model,
		Messages: messages,
		Options: map[string]interface{}{
			"temperature": req.Temperature,
			"num_predict": req.MaxTokens,
		},
		Stream: true,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, &provider.LLMError{
			Message:    err.Error(),
			StatusCode: 0,
		}
	}

	// Try primary model, fall back if needed
	models := []string{o.model, o.fallbackModel}
	client := &http.Client{Timeout: 600 * time.Second} // Longer timeout for streaming

	var lastError *provider.LLMError
	for _, model := range models {
		// Update model in body if using fallback
		if model != o.model {
			body.Model = model
			bodyBytes, _ = json.Marshal(body)
		}

		url := fmt.Sprintf("%s/api/chat", o.baseURL)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := provider.DoWithRetry(ctx, client, httpReq)
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		// If 404 or model not found, try next model
		if resp.StatusCode == 404 {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastError = &provider.LLMError{
				Message:    fmt.Sprintf("Model not found: %s - %s", model, string(respBody)),
				StatusCode: resp.StatusCode,
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

		// Parse streaming response
		var fullText strings.Builder

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var chunk chatResponse
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				continue
			}

			// Extract text from chunk
			if chunk.Message.Content != "" {
				text := chunk.Message.Content
				fullText.WriteString(text)
				if onChunk != nil {
					onChunk(text)
				}
			}

			// Check if done
			if chunk.Done {
				break
			}
		}

		_ = resp.Body.Close()

		if err := scanner.Err(); err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		// Approximate token counts
		inputTokens := len(strings.Fields(req.SystemPrompt + " " + req.Prompt))
		outputTokens := len(strings.Fields(fullText.String()))

		return &provider.LLMResult{
			Text:         fullText.String(),
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		}, nil
	}

	// All models failed
	return nil, lastError
}

// Request/response types for Ollama Chat API

type chatRequest struct {
	Model    string                 `json:"model"`
	Messages []message              `json:"messages"`
	Options  map[string]interface{} `json:"options,omitempty"`
	Stream   bool                   `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message message `json:"message"`
	Done    bool    `json:"done"`
}
