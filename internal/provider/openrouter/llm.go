package openrouter

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

// OpenRouterLLMProvider implements the LLMProvider interface for OpenRouter
type OpenRouterLLMProvider struct {
	apiKey        string
	model         string
	fallbackModel string
	baseURL       string
}

// NewOpenRouterLLMProvider creates a new OpenRouter LLM provider
func NewOpenRouterLLMProvider(apiKey, model, fallbackModel string) (*OpenRouterLLMProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = "openai/gpt-4o" // Default model
	}
	if fallbackModel == "" {
		fallbackModel = "openai/gpt-3.5-turbo" // Default fallback
	}

	return &OpenRouterLLMProvider{
		apiKey:        apiKey,
		model:         model,
		fallbackModel: fallbackModel,
		baseURL:       defaultBaseURL,
	}, nil
}

// Name returns the provider name
func (o *OpenRouterLLMProvider) Name() string {
	return "openrouter"
}

// Validate tests the provider connection
func (o *OpenRouterLLMProvider) Validate(ctx context.Context) error {
	// Make a minimal test call
	req := provider.LLMRequest{
		Prompt:      "test",
		MaxTokens:   10,
		Temperature: 0.3,
	}
	_, err := o.Call(ctx, req)
	if err != nil {
		return fmt.Errorf("validation failed: %s", err.Message)
	}
	return nil
}

// Call makes a non-streaming LLM request
func (o *OpenRouterLLMProvider) Call(ctx context.Context, req provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
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
	body := chatCompletionRequest{
		Model:       o.model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      false,
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
	client := &http.Client{Timeout: 120 * time.Second}

	var lastError *provider.LLMError
	for _, model := range models {
		// Update model in body if using fallback
		if model != o.model {
			body.Model = model
			bodyBytes, _ = json.Marshal(body)
		}

		url := fmt.Sprintf("%s/chat/completions", o.baseURL)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
		httpReq.Header.Set("X-Title", "RepoG")

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
		if resp.StatusCode == 404 || resp.StatusCode == 400 {
			lastError = &provider.LLMError{
				Message:    fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
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

		var chatResp chatCompletionResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		// Extract text from response
		var text string
		if len(chatResp.Choices) > 0 {
			text = chatResp.Choices[0].Message.Content
		}

		return &provider.LLMResult{
			Text:         text,
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
		}, nil
	}

	// All models failed
	return nil, lastError
}

// Stream makes a streaming LLM request
func (o *OpenRouterLLMProvider) Stream(ctx context.Context, req provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
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
	body := chatCompletionRequest{
		Model:       o.model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
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
	client := &http.Client{Timeout: 300 * time.Second}

	var lastError *provider.LLMError
	for _, model := range models {
		// Update model in body if using fallback
		if model != o.model {
			body.Model = model
			bodyBytes, _ = json.Marshal(body)
		}

		url := fmt.Sprintf("%s/chat/completions", o.baseURL)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
		httpReq.Header.Set("X-Title", "RepoG")

		resp, err := provider.DoWithRetry(ctx, client, httpReq)
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		// If 404 or model not found, try next model
		if resp.StatusCode == 404 || resp.StatusCode == 400 {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastError = &provider.LLMError{
				Message:    fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
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

		// Parse SSE stream
		var fullText strings.Builder
		var inputTokens, outputTokens int

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk streamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			// Extract text from chunk
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				text := chunk.Choices[0].Delta.Content
				fullText.WriteString(text)
				if onChunk != nil {
					onChunk(text)
				}
			}

			// Update token counts if provided
			if chunk.Usage != nil {
				inputTokens = chunk.Usage.PromptTokens
				outputTokens = chunk.Usage.CompletionTokens
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

	// All models failed
	return nil, lastError
}

// Request/response types for OpenRouter Chat API

type chatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float32   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []choice  `json:"choices"`
	Usage   usageInfo `json:"usage"`
}

type choice struct {
	Message      message      `json:"message"`
	Delta        messageDelta `json:"delta,omitempty"`
	FinishReason string       `json:"finish_reason"`
}

type messageDelta struct {
	Content string `json:"content"`
}

type streamChunk struct {
	Choices []choice   `json:"choices"`
	Usage   *usageInfo `json:"usage,omitempty"`
}
