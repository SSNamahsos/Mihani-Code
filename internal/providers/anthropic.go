package providers

import (
"bytes"
"encoding/json"
"fmt"
"io"
"net/http"

"github.com/SSNamahsos/Mihani-Code/internal/llm"
)

// AnthropicProvider implements the Provider interface for Anthropic API
type AnthropicProvider struct {
APIKey string
Model  string
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
if model == "" {
model = "claude-3-5-sonnet-20241022"
}
return &AnthropicProvider{
APIKey: apiKey,
Model:  model,
}
}

// Chat sends a chat request and returns the response
func (p *AnthropicProvider) Chat(request *llm.ChatRequest) (*llm.ChatResponse, error) {
var messages []map[string]interface{}
var systemPrompt string

for _, msg := range request.Messages {
if msg.Role == "system" {
systemPrompt = msg.Content
continue
}
messages = append(messages, map[string]interface{}{
"role":    msg.Role,
"content": msg.Content,
})
}

reqBody := map[string]interface{}{
"model":       p.Model,
"max_tokens":  4096,
"messages":    messages,
"tool_choice": map[string]string{"type": "auto"},
}

if systemPrompt != "" {
reqBody["system"] = systemPrompt
}

if len(request.Tools) > 0 {
var tools []map[string]interface{}
for _, tool := range request.Tools {
tools = append(tools, map[string]interface{}{
"name":        tool.Name,
"description": tool.Description,
"input_schema": tool.Parameters,
})
}
reqBody["tools"] = tools
}

jsonBody, err := json.Marshal(reqBody)
if err != nil {
return nil, fmt.Errorf("failed to marshal request: %w", err)
}

url := "https://api.anthropic.com/v1/messages"
httpReq, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
if err != nil {
return nil, fmt.Errorf("failed to create request: %w", err)
}

httpReq.Header.Set("Content-Type", "application/json")
httpReq.Header.Set("x-api-key", p.APIKey)
httpReq.Header.Set("anthropic-version", "2023-06-01")

client := &http.Client{}
resp, err := client.Do(httpReq)
if err != nil {
return nil, fmt.Errorf("request failed: %w", err)
}
defer resp.Body.Close()

body, err := io.ReadAll(resp.Body)
if err != nil {
return nil, fmt.Errorf("failed to read response: %w", err)
}

if resp.StatusCode != http.StatusOK {
return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
}

var apiResp struct {
Content []struct {
Type  string                 `json:"type"`
Text  string                 `json:"text,omitempty"`
ID    string                 `json:"id,omitempty"`
Name  string                 `json:"name,omitempty"`
Input map[string]interface{} `json:"input,omitempty"`
} `json:"content"`
}

if err := json.Unmarshal(body, &apiResp); err != nil {
return nil, fmt.Errorf("failed to parse response: %w", err)
}

response := &llm.ChatResponse{Done: true}

for _, c := range apiResp.Content {
if c.Type == "text" {
response.Content += c.Text
} else if c.Type == "tool_use" {
response.ToolCalls = append(response.ToolCalls, llm.ToolCall{
ID:        c.ID,
Name:      c.Name,
Arguments: c.Input,
})
}
}

return response, nil
}

// ChatStream sends a streaming chat request
func (p *AnthropicProvider) ChatStream(request *llm.ChatRequest, callback func(string)) (*llm.ChatResponse, error) {
resp, err := p.Chat(request)
if err != nil {
return nil, err
}

if callback != nil && resp.Content != "" {
callback(resp.Content)
}

return resp, nil
}
