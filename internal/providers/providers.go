package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MessageRole represents the role of a message
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// ToolCallParam represents a tool call in a message
type ToolCallParam struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Message represents a chat message
type Message struct {
	Role       MessageRole     `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []ToolCallParam `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

// ToolDefinition represents a tool/function definition
type ToolDefinition struct {
	Type     string          `json:"type"`
	Function *FunctionSchema `json:"function,omitempty"`
}

// FunctionSchema represents a function schema
type FunctionSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ChatRequest represents a chat completion request
type ChatRequest struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Stream      bool        `json:"stream,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
}

// ToolCallResponse represents a tool call from the model response
type ToolCallResponse struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int               `json:"index"`
	Message      ResponseMessage   `json:"message"`
	FinishReason string            `json:"finish_reason"`
	Delta        ResponseMessage   `json:"delta,omitempty"`
}

// ResponseMessage represents a response message
type ResponseMessage struct {
	Role       MessageRole        `json:"role"`
	Content    string             `json:"content,omitempty"`
	ToolCalls  []ToolCallResponse `json:"tool_calls,omitempty"`
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
	Error *APIError `json:"error,omitempty"`
}

// APIError represents an API error
type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// StreamChunk represents a streaming response chunk
type StreamChunk struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

// Provider defines the interface for LLM providers
type Provider interface {
	Chat(req *ChatRequest) (*ChatResponse, error)
	ChatStream(req *ChatRequest) (<-chan StreamChunk, <-chan error, error)
	FormatTools(tools []ToolDefinition) interface{}
}

// OpenAICompatibleProvider implements Provider for OpenAI-compatible APIs
type OpenAICompatibleProvider struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

// NewOpenAICompatibleProvider creates a new OpenAI-compatible provider
func NewOpenAICompatibleProvider(baseURL, apiKey, model string) *OpenAICompatibleProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAICompatibleProvider{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		Client:  &http.Client{},
	}
}

// FormatTools formats tools for OpenAI API
func (p *OpenAICompatibleProvider) FormatTools(tools []ToolDefinition) interface{} {
	result := make([]map[string]interface{}, len(tools))
	for i, tool := range tools {
		result[i] = map[string]interface{}{
			"type":     "function",
			"function": tool.Function,
		}
	}
	return result
}

// Chat sends a chat completion request
func (p *OpenAICompatibleProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = p.Model
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	url := strings.TrimSuffix(p.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil {
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, apiErr.Message)
		}
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, err
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	return &chatResp, nil
}

// ChatStream sends a streaming chat completion request
func (p *OpenAICompatibleProvider) ChatStream(req *ChatRequest) (<-chan StreamChunk, <-chan error, error) {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}

	url := strings.TrimSuffix(p.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}

	chunks := make(chan StreamChunk, 100)
	errors := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		defer close(chunks)
		defer close(errors)

		buf := make([]byte, 4096)
		var line []byte
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				for i := 0; i < n; i++ {
					b := buf[i]
					if b == '\n' {
						str := strings.TrimSpace(string(line))
						line = line[:0]
						if !strings.HasPrefix(str, "data: ") {
							continue
						}
						data := strings.TrimPrefix(str, "data: ")
						if data == "[DONE]" {
							return
						}
						var chunk StreamChunk
						if err := json.Unmarshal([]byte(data), &chunk); err != nil {
							continue
						}
						chunks <- chunk
					} else {
						line = append(line, b)
					}
				}
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				errors <- err
				return
			}
		}
	}()

	return chunks, errors, nil
}

// AnthropicProvider implements Provider for Anthropic API
type AnthropicProvider struct {
	APIKey  string
	Model   string
	Client  *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	return &AnthropicProvider{
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{},
	}
}

// FormatTools formats tools for Anthropic API
func (p *AnthropicProvider) FormatTools(tools []ToolDefinition) interface{} {
	result := make([]map[string]interface{}, len(tools))
	for i, tool := range tools {
		result[i] = map[string]interface{}{
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"input_schema": tool.Function.Parameters,
		}
	}
	return result
}

// Chat sends a chat completion request to Anthropic
func (p *AnthropicProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	// Anthropic has a different API structure
	// This is a simplified implementation
	return nil, fmt.Errorf("Anthropic provider requires special message format - full implementation needed")
}

// ChatStream sends a streaming chat completion request to Anthropic
func (p *AnthropicProvider) ChatStream(req *ChatRequest) (<-chan StreamChunk, <-chan error, error) {
	return nil, nil, fmt.Errorf("Anthropic streaming requires special implementation")
}

// CreateProvider creates a provider based on the provider type
func CreateProvider(providerType, baseURL, apiKey, model string) (Provider, error) {
	switch strings.ToLower(providerType) {
	case "anthropic":
		return NewAnthropicProvider(apiKey, model), nil
	case "openai-compatible", "openai", "":
		return NewOpenAICompatibleProvider(baseURL, apiKey, model), nil
	default:
		return NewOpenAICompatibleProvider(baseURL, apiKey, model), nil
	}
}
