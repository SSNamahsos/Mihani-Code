// Package llm provides LLM API client implementations.
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
	"time"
)

// Provider represents an LLM provider type.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderNone      Provider = "none"
)

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client is the interface for LLM providers.
type Client interface {
	Chat(ctx context.Context, messages []Message) (string, error)
	CodeExplain(ctx context.Context, code string) (string, error)
	CodeRefactor(ctx context.Context, code string, instruction string) (string, error)
	CodeGenerate(ctx context.Context, prompt string) (string, error)
}

// OpenAIClient implements the Client interface for OpenAI.
type OpenAIClient struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIClient creates a new OpenAI client.
func NewOpenAIClient(apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Chat sends messages to OpenAI and returns the response.
func (c *OpenAIClient) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := map[string]interface{}{
		"model": c.model,
		"messages": func() []map[string]string {
			result := make([]map[string]string, len(messages))
			for i, m := range messages {
				result[i] = map[string]string{"role": m.Role, "content": m.Content}
			}
			return result
		}(),
		"max_tokens": 4096,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// CodeExplain explains Go code.
func (c *OpenAIClient) CodeExplain(ctx context.Context, code string) (string, error) {
	messages := []Message{
		{Role: "system", Content: "You are a Go programming expert. Explain the provided Go code clearly and concisely."},
		{Role: "user", Content: "Explain this Go code:\n\n```go\n" + code + "\n```"},
	}
	return c.Chat(ctx, messages)
}

// CodeRefactor refactors Go code based on instructions.
func (c *OpenAIClient) CodeRefactor(ctx context.Context, code string, instruction string) (string, error) {
	messages := []Message{
		{Role: "system", Content: "You are a Go programming expert. Refactor code according to user instructions while maintaining functionality."},
		{Role: "user", Content: fmt.Sprintf("Refactor this Go code: %s\n\nInstruction: %s\n\nProvide only the refactored code in a go code block.", code, instruction)},
	}
	return c.Chat(ctx, messages)
}

// CodeGenerate generates Go code from a prompt.
func (c *OpenAIClient) CodeGenerate(ctx context.Context, prompt string) (string, error) {
	messages := []Message{
		{Role: "system", Content: "You are a Go programming expert. Generate clean, idiomatic Go code based on user requirements."},
		{Role: "user", Content: "Generate Go code for: " + prompt},
	}
	return c.Chat(ctx, messages)
}

// AnthropicClient implements the Client interface for Anthropic.
type AnthropicClient struct {
	apiKey string
	model  string
	client *http.Client
}

// NewAnthropicClient creates a new Anthropic client.
func NewAnthropicClient(apiKey, model string) *AnthropicClient {
	return &AnthropicClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Chat sends messages to Anthropic and returns the response.
func (c *AnthropicClient) Chat(ctx context.Context, messages []Message) (string, error) {
	// Convert messages to Anthropic format
	var systemPrompt string
	var anthropicMessages []map[string]interface{}

	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
		} else {
			anthropicMessages = append(anthropicMessages, map[string]interface{}{
				"role":    m.Role,
				"content": m.Content,
			})
		}
	}

	reqBody := map[string]interface{}{
		"model":       c.model,
		"max_tokens":  4096,
		"messages":    anthropicMessages,
	}
	
	if systemPrompt != "" {
		reqBody["system"] = systemPrompt
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	return strings.TrimSpace(result.Content[0].Text), nil
}

// CodeExplain explains Go code.
func (c *AnthropicClient) CodeExplain(ctx context.Context, code string) (string, error) {
	messages := []Message{
		{Role: "system", Content: "You are a Go programming expert. Explain the provided Go code clearly and concisely."},
		{Role: "user", Content: "Explain this Go code:\n\n```go\n" + code + "\n```"},
	}
	return c.Chat(ctx, messages)
}

// CodeRefactor refactors Go code based on instructions.
func (c *AnthropicClient) CodeRefactor(ctx context.Context, code string, instruction string) (string, error) {
	messages := []Message{
		{Role: "system", Content: "You are a Go programming expert. Refactor code according to user instructions while maintaining functionality."},
		{Role: "user", Content: fmt.Sprintf("Refactor this Go code: %s\n\nInstruction: %s\n\nProvide only the refactored code in a go code block.", code, instruction)},
	}
	return c.Chat(ctx, messages)
}

// CodeGenerate generates Go code from a prompt.
func (c *AnthropicClient) CodeGenerate(ctx context.Context, prompt string) (string, error) {
	messages := []Message{
		{Role: "system", Content: "You are a Go programming expert. Generate clean, idiomatic Go code based on user requirements."},
		{Role: "user", Content: "Generate Go code for: " + prompt},
	}
	return c.Chat(ctx, messages)
}

// NewClient creates an appropriate LLM client based on configuration.
func NewClient(provider Provider, apiKey, model string) (Client, error) {
	// Check environment variables if apiKey not provided
	if apiKey == "" {
		switch provider {
		case ProviderOpenAI:
			apiKey = os.Getenv("OPENAI_API_KEY")
		case ProviderAnthropic:
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}

	if apiKey == "" {
		return nil, fmt.Errorf("no API key provided for provider %s", provider)
	}

	if model == "" {
		switch provider {
		case ProviderOpenAI:
			model = "gpt-4o-mini"
		case ProviderAnthropic:
			model = "claude-sonnet-4-5-20250929"
		}
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

// StandaloneClient provides offline capabilities.
type StandaloneClient struct{}

// NewStandaloneClient creates a standalone client for offline use.
func NewStandaloneClient() *StandaloneClient {
	return &StandaloneClient{}
}

// Chat provides canned responses for offline mode.
func (c *StandaloneClient) Chat(ctx context.Context, messages []Message) (string, error) {
	return "[Offline Mode] Mihani Code is running in standalone mode. Configure an API key to enable full AI assistance.\n\nAvailable offline features:\n- File viewing and editing\n- Code scanning\n- Snippet templates\n- Git integration\n\nUse /help to see all commands.", nil
}

// CodeExplain provides basic static analysis hints.
func (c *StandaloneClient) CodeExplain(ctx context.Context, code string) (string, error) {
	return "[Offline Mode] AI explanation unavailable. The code appears to be Go source code. Configure an API key for detailed explanations.", nil
}

// CodeRefactor provides basic suggestions.
func (c *StandaloneClient) CodeRefactor(ctx context.Context, code string, instruction string) (string, error) {
	return "[Offline Mode] AI refactoring unavailable. Configure an API key for intelligent code refactoring.", nil
}

// CodeGenerate provides template-based generation.
func (c *StandaloneClient) CodeGenerate(ctx context.Context, prompt string) (string, error) {
	return "[Offline Mode] AI generation unavailable. Configure an API key for intelligent code generation.\n\nSee /snippets command for available templates.", nil
}
