package llm

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolDefinition defines a tool available to the model
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall represents a tool call from the model
type ToolCall struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Arguments ToolArguments `json:"arguments"`
}

// ToolArguments holds the arguments for a tool call
type ToolArguments map[string]interface{}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolCallID string      `json:"tool_call_id"`
	Success    bool        `json:"success"`
	Result     interface{} `json:"result,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// ChatRequest represents a request to the LLM
type ChatRequest struct {
	Messages    []Message      `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	Model       string         `json:"model"`
}

// ChatResponse represents a response from the LLM
type ChatResponse struct {
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Done      bool       `json:"done"`
}

// Provider interface for LLM providers
type Provider interface {
	Chat(request *ChatRequest) (*ChatResponse, error)
	ChatStream(request *ChatRequest, callback func(string)) (*ChatResponse, error)
}
