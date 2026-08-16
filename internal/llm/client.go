package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Provider represents an LLM provider.
type Provider string

const (
	ProviderNone       Provider = "none"
	ProviderOpenAI     Provider = "openai"
	ProviderAnthropic  Provider = "anthropic"
)

// Client defines the interface for LLM clients.
type Client interface {
	Chat(ctx context.Context, messages []Message) (string, error)
	ChatStream(ctx context.Context, messages []Message, callback func(string)) (string, error)
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIClient is an OpenAI API client.
type OpenAIClient struct {
	apiKey string
	model  string
	client *http.Client
}

// AnthropicClient is an Anthropic API client.
type AnthropicClient struct {
	apiKey string
	model  string
	client *http.Client
}

// StandaloneClient is a fallback client for offline mode.
type StandaloneClient struct{}

// NewClient creates a new LLM client based on the provider.
func NewClient(provider Provider, apiKey, model string) (Client, error) {
	if apiKey == "" {
		switch provider {
		case ProviderOpenAI:
			apiKey = os.Getenv("OPENAI_API_KEY")
		case ProviderAnthropic:
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}

	if apiKey == "" {
		return nil, fmt.Errorf("no API key available for provider %s", provider)
	}

	switch provider {
	case ProviderOpenAI:
		return NewOpenAIClient(apiKey, model), nil
	case ProviderAnthropic:
		return NewAnthropicClient(apiKey, model), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

// NewOpenAIClient creates a new OpenAI client.
func NewOpenAIClient(apiKey, model string) *OpenAIClient {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAIClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
	}
}

// NewAnthropicClient creates a new Anthropic client.
func NewAnthropicClient(apiKey, model string) *AnthropicClient {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &AnthropicClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
	}
}

// NewStandaloneClient creates a standalone client for offline mode.
func NewStandaloneClient() *StandaloneClient {
	return &StandaloneClient{}
}

// Chat sends a chat request to OpenAI.
func (c *OpenAIClient) Chat(ctx context.Context, messages []Message) (string, error) {
	return c.ChatStream(ctx, messages, nil)
}

// ChatStream sends a streaming chat request to OpenAI.
func (c *OpenAIClient) ChatStream(ctx context.Context, messages []Message, callback func(string)) (string, error) {
	url := "https://api.openai.com/v1/chat/completions"

	reqBody := map[string]interface{}{
		"model":    c.model,
		"messages": messages,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	content := result.Choices[0].Message.Content
	if callback != nil {
		callback(content)
	}

	return content, nil
}

// Chat sends a chat request to Anthropic.
func (c *AnthropicClient) Chat(ctx context.Context, messages []Message) (string, error) {
	return c.ChatStream(ctx, messages, nil)
}

// ChatStream sends a streaming chat request to Anthropic.
func (c *AnthropicClient) ChatStream(ctx context.Context, messages []Message, callback func(string)) (string, error) {
	url := "https://api.anthropic.com/v1/messages"

	// Convert messages to Anthropic format
	var systemPrompt string
	var anthropicMessages []map[string]interface{}

	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
		} else {
			anthropicMessages = append(anthropicMessages, map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			})
		}
	}

	reqBody := map[string]interface{}{
		"model":         c.model,
		"max_tokens":    4096,
		"messages":      anthropicMessages,
	}

	if systemPrompt != "" {
		reqBody["system"] = systemPrompt
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	content := result.Content[0].Text
	if callback != nil {
		callback(content)
	}

	return content, nil
}

// Chat handles chat in standalone/offline mode.
func (c *StandaloneClient) Chat(ctx context.Context, messages []Message) (string, error) {
	return c.ChatStream(ctx, messages, nil)
}

// ChatStream handles streaming chat in standalone/offline mode.
func (c *StandaloneClient) ChatStream(ctx context.Context, messages []Message, callback func(string)) (string, error) {
	// Provide helpful offline responses
	lastMsg := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastMsg = messages[i].Content
			break
		}
	}

	response := `[OFFLINE MODE]

Mihani Code is running in offline mode. To enable full AI features:

1. Set your API key via environment variable:
   - export OPENAI_API_KEY=your_key_here
   - export ANTHROPIC_API_KEY=your_key_here

2. Or configure in ~/.mihanirc or config file:
   {
     "default_provider": "openai",
     "openai_api_key": "your_key_here"
   }

Offline capabilities available:
- /read, /write - File operations
- /scan - Codebase scanning
- /snippets - Code templates
- /history - Command history

Your query: "` + lastMsg + `"

Please configure an API key for AI-powered assistance.`

	if callback != nil {
		callback(response)
	}

	return response, nil
}

// IsOnline checks if the client can connect to the API.
func IsOnline(client Client) bool {
	_, ok := client.(*StandaloneClient)
	return !ok
}

// GetProviderName returns the provider name from a client.
func GetProviderName(client Client) string {
	switch client.(type) {
	case *OpenAIClient:
		return "openai"
	case *AnthropicClient:
		return "anthropic"
	case *StandaloneClient:
		return "offline"
	default:
		return "unknown"
	}
}

// SanitizeInput removes potentially problematic characters from user input.
func SanitizeInput(input string) string {
	// Remove null bytes and other control characters
	input = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, input)
	return strings.TrimSpace(input)
}
