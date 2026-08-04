package anthropic

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

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/provider"
)

func init() {
	// Register Anthropic LLM provider
	provider.RegisterLLMProvider("anthropic", func(cfg config.ProviderConfig, apiKey string) (provider.LLMProvider, error) {
		return NewAnthropicLLMProvider(apiKey, cfg.Model, cfg.Fallback)
	})
}

const defaultBaseURL = "https://api.anthropic.com/v1"
const anthropicVersion = "2023-06-01"

// AnthropicLLMProvider implements the LLMProvider interface for Anthropic Claude
type AnthropicLLMProvider struct {
	apiKey        string
	model         string
	fallbackModel string
	baseURL       string
}

// NewAnthropicLLMProvider creates a new Anthropic LLM provider
func NewAnthropicLLMProvider(apiKey, model, fallbackModel string) (*AnthropicLLMProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = "claude-3-5-haiku-20241022" // Latest Haiku (fast, cheap)
	}
	if fallbackModel == "" {
		fallbackModel = "claude-3-5-sonnet-20241022" // Latest Sonnet (powerful)
	}

	return &AnthropicLLMProvider{
		apiKey:        apiKey,
		model:         model,
		fallbackModel: fallbackModel,
		baseURL:       defaultBaseURL,
	}, nil
}

// Name returns the provider name
func (a *AnthropicLLMProvider) Name() string {
	return "anthropic"
}

// Validate tests the provider connection
func (a *AnthropicLLMProvider) Validate(ctx context.Context) error {
	// Make a minimal test call
	req := provider.LLMRequest{
		Prompt:      "test",
		MaxTokens:   10,
		Temperature: 0.3,
	}
	_, err := a.Call(ctx, req)
	if err != nil {
		return fmt.Errorf("validation failed: %s", err.Message)
	}
	return nil
}

// Call makes a non-streaming LLM request
func (a *AnthropicLLMProvider) Call(ctx context.Context, req provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	if req.Temperature == 0 {
		req.Temperature = 0.3
	}

	// Build messages array - Anthropic format
	messages := make([]message, 0, 1)

	// User message with the prompt
	messages = append(messages, message{
		Role:    "user",
		Content: req.Prompt,
	})

	// Build request body
	body := messagesRequest{
		Model:       a.model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      false,
	}

	// Add system prompt if provided
	if req.SystemPrompt != "" {
		body.System = req.SystemPrompt
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, &provider.LLMError{
			Message:    err.Error(),
			StatusCode: 0,
		}
	}

	// Try primary model, fall back if needed
	models := []string{a.model, a.fallbackModel}
	client := &http.Client{Timeout: 120 * time.Second}

	var lastError *provider.LLMError
	for _, model := range models {
		// Update model in body if using fallback
		if model != a.model {
			body.Model = model
			bodyBytes, _ = json.Marshal(body)
		}

		url := fmt.Sprintf("%s/messages", a.baseURL)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", a.apiKey)
		httpReq.Header.Set("anthropic-version", anthropicVersion)

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

		var msgResp messagesResponse
		if err := json.Unmarshal(respBody, &msgResp); err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		// Extract text from response
		var text strings.Builder
		for _, content := range msgResp.Content {
			if content.Type == "text" {
				text.WriteString(content.Text)
			}
		}

		// Extract token counts
		var inputTokens, outputTokens int
		if msgResp.Usage != nil {
			inputTokens = msgResp.Usage.InputTokens
			outputTokens = msgResp.Usage.OutputTokens
		}

		return &provider.LLMResult{
			Text:         text.String(),
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		}, nil
	}

	// All models failed
	return nil, lastError
}

// Stream makes a streaming LLM request
func (a *AnthropicLLMProvider) Stream(ctx context.Context, req provider.LLMRequest, onChunk func(string)) (*provider.LLMResult, *provider.LLMError) {
	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	if req.Temperature == 0 {
		req.Temperature = 0.3
	}

	// Build messages array
	messages := []message{
		{
			Role:    "user",
			Content: req.Prompt,
		},
	}

	// Build request body
	body := messagesRequest{
		Model:       a.model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}

	// Add system prompt if provided
	if req.SystemPrompt != "" {
		body.System = req.SystemPrompt
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, &provider.LLMError{
			Message:    err.Error(),
			StatusCode: 0,
		}
	}

	// Try primary model, fall back if needed
	models := []string{a.model, a.fallbackModel}
	client := &http.Client{Timeout: 300 * time.Second}

	var lastError *provider.LLMError
	for _, model := range models {
		// Update model in body if using fallback
		if model != a.model {
			body.Model = model
			bodyBytes, _ = json.Marshal(body)
		}

		url := fmt.Sprintf("%s/messages", a.baseURL)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, &provider.LLMError{
				Message:    err.Error(),
				StatusCode: 0,
			}
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", a.apiKey)
		httpReq.Header.Set("anthropic-version", anthropicVersion)

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

			// Anthropic uses different event types
			var event streamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			// Handle different event types
			switch event.Type {
			case "content_block_delta":
				if event.Delta.Type == "text_delta" {
					text := event.Delta.Text
					fullText.WriteString(text)
					if onChunk != nil {
						onChunk(text)
					}
				}
			case "message_delta":
				if event.Usage != nil {
					outputTokens = event.Usage.OutputTokens
				}
			case "message_start":
				if event.Message != nil && event.Message.Usage != nil {
					inputTokens = event.Message.Usage.InputTokens
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

	// All models failed
	return nil, lastError
}

// Request/response types for Anthropic Messages API

type messagesRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float32   `json:"temperature,omitempty"`
	System      string    `json:"system,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
	Usage   *usageInfo     `json:"usage"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type usageInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Streaming event types
type streamEvent struct {
	Type    string            `json:"type"`
	Delta   *delta            `json:"delta,omitempty"`
	Usage   *usageInfo        `json:"usage,omitempty"`
	Message *messagesResponse `json:"message,omitempty"`
}

type delta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
