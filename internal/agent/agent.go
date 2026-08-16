package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/providers"
	"github.com/SSNamahsos/Mihani-Code/internal/tools"
)

// AgentMode represents the mode of operation for the agent
type AgentMode string

const (
	ModeBuild AgentMode = "build" // Can inspect, edit, run commands
	ModePlan  AgentMode = "plan"  // Can only inspect and plan
)

// TaskState represents the state of a task item
type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskInProgress TaskState = "in_progress"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
)

// TaskItem represents a single task in the task list
type TaskItem struct {
	Description string    `json:"description"`
	State       TaskState `json:"state"`
}

// ToolActivity represents an ongoing tool execution
type ToolActivity struct {
	Name      string      `json:"name"`
	Args      interface{} `json:"args"`
	Status    string      `json:"status"`
	Result    interface{} `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// ConversationMessage represents a message in the conversation
type ConversationMessage struct {
	Role       string                  `json:"role"`
	Content    string                  `json:"content,omitempty"`
	ToolCalls  []providers.ToolCallParam `json:"tool_calls,omitempty"`
	ToolCallID string                  `json:"tool_call_id,omitempty"`
	Name       string                  `json:"name,omitempty"`
}

// AgentState represents the current state of the agent
type AgentState struct {
	Mode           AgentMode             `json:"mode"`
	CurrentTask    string                `json:"current_task"`
	Tasks          []TaskItem            `json:"tasks"`
	Iteration      int                   `json:"iteration"`
	ToolCallsCount int                   `json:"tool_calls_count"`
	ActiveTools    []ToolActivity        `json:"active_tools"`
	Messages       []ConversationMessage `json:"messages"`
}

// Agent represents the coding agent
type Agent struct {
	Config      *config.Config
	Provider    providers.Provider
	ToolRegistry *tools.Registry
	State       *AgentState
	Model       string
}

// NewAgent creates a new agent instance
func NewAgent(cfg *config.Config, provider providers.Provider, toolRegistry *tools.Registry) *Agent {
	return &Agent{
		Config:       cfg,
		Provider:     provider,
		ToolRegistry: toolRegistry,
		State: &AgentState{
			Mode:        ModeBuild,
			Tasks:       make([]TaskItem, 0),
			Messages:    make([]ConversationMessage, 0),
			ActiveTools: make([]ToolActivity, 0),
		},
		Model: cfg.Provider.Model,
	}
}

// SetMode sets the agent mode
func (a *Agent) SetMode(mode AgentMode) {
	a.State.Mode = mode
}

// AddTask adds a task to the task list
func (a *Agent) AddTask(description string) {
	a.State.Tasks = append(a.State.Tasks, TaskItem{
		Description: description,
		State:       TaskPending,
	})
}

// UpdateTaskState updates the state of a task
func (a *Agent) UpdateTaskState(index int, state TaskState) {
	if index >= 0 && index < len(a.State.Tasks) {
		a.State.Tasks[index].State = state
	}
}

// AddMessage adds a message to the conversation
func (a *Agent) AddMessage(role string, content string, toolCalls []providers.ToolCallParam, toolCallID string) {
	msg := ConversationMessage{
		Role:       role,
		Content:    content,
		ToolCalls:  toolCalls,
		ToolCallID: toolCallID,
	}
	a.State.Messages = append(a.State.Messages, msg)
}

// ClearMessages clears all messages
func (a *Agent) ClearMessages() {
	a.State.Messages = make([]ConversationMessage, 0)
}

// GetSystemPrompt returns the system prompt for the agent
func (a *Agent) GetSystemPrompt() string {
	projectType := a.Config.DetectProjectType()
	
	prompt := `You are Mihani Code, an expert AI coding assistant operating directly inside the user's project directory.

CORE BEHAVIORS:
- You have access to powerful tools that can read files, edit code, run commands, and interact with git
- You should actively use these tools to accomplish tasks, not just describe what could be done
- Always inspect the project before making assumptions about its structure
- Make actual changes when asked to implement something
- Verify your work by running tests, builds, or linting when appropriate
- If errors occur, inspect them and fix the issues iteratively
- Preserve existing project conventions and coding styles
- Never claim a change was made if you didn't actually execute the tool

WORKFLOW:
1. Understand the user's request
2. Explore the project to find relevant files and understand context
3. Plan your approach (you can maintain a task list for complex requests)
4. Use tools to read files, search code, and gather information
5. Make targeted edits using the edit_file tool
6. Run appropriate validation (tests, builds, linting) based on the project type
7. Fix any issues discovered during validation
8. Review your changes using git diff
9. Provide a clear summary of what was accomplished

PROJECT CONTEXT:
- Current working directory: ` + a.Config.WorkDir + `
- Detected project type: ` + projectType + `
- Operating system: ` + config.GetOS() + `

TOOL USAGE GUIDELINES:
- Use read_file to examine file contents before editing
- Use grep to search for specific code patterns or symbols
- Use list_directory to explore unfamiliar directories
- Use edit_file with targeted search/replace operations rather than rewriting entire files
- Use shell to run tests, builds, linting, or other validation commands
- Use git_status and git_diff to review changes before completing a task
- When running commands, choose appropriate commands for the project type:
  * Go projects: go test ./..., go build ./..., go vet ./...
  * Node.js: npm test, npm run build
  * Python: pytest, python -m pytest
  * Rust: cargo test, cargo build
  * etc.

SAFETY:
- Do not make unnecessary changes
- Do not delete files unless explicitly requested
- Do not run destructive commands without explicit user approval
- If unsure about a change, ask the user for clarification

When you complete a task, provide a concise summary of:
- What files were modified
- What changes were made
- What validation was performed
- Any remaining issues or recommendations`

	return prompt
}

// Run executes the agent loop for a given user prompt
func (a *Agent) Run(prompt string, streamChan chan<- AgentEvent) (*AgentState, error) {
	// Reset iteration counter
	a.State.Iteration = 0
	a.State.ToolCallsCount = 0

	// Add user message
	a.AddMessage("user", prompt, nil, "")

	// Send initial event
	streamChan <- AgentEvent{
		Type: EventTypeUserMessage,
		Data: prompt,
	}

	for {
		// Check limits
		if a.State.Iteration >= a.Config.Limits.MaxIterations {
			streamChan <- AgentEvent{
				Type: EventTypeError,
				Data: fmt.Sprintf("Maximum iterations (%d) reached", a.Config.Limits.MaxIterations),
			}
			return a.State, fmt.Errorf("max iterations reached")
		}

		if a.State.ToolCallsCount >= a.Config.Limits.MaxToolCalls {
			streamChan <- AgentEvent{
				Type: EventTypeError,
				Data: fmt.Sprintf("Maximum tool calls (%d) reached", a.Config.Limits.MaxToolCalls),
			}
			return a.State, fmt.Errorf("max tool calls reached")
		}

		a.State.Iteration++

		// Build request
		messages := a.buildMessages()
		toolDefs := a.buildToolDefinitions()

		req := &providers.ChatRequest{
			Model:    a.Model,
			Messages: messages,
			Tools:    toolDefs,
		}

		// Call the model
		resp, err := a.Provider.Chat(req)
		if err != nil {
			streamChan <- AgentEvent{
				Type: EventTypeError,
				Data: fmt.Sprintf("Model error: %v", err),
			}
			return a.State, err
		}

		if len(resp.Choices) == 0 {
			streamChan <- AgentEvent{
				Type: EventTypeError,
				Data: "Empty response from model",
			}
			return a.State, fmt.Errorf("empty response")
		}

		choice := resp.Choices[0]
		message := choice.Message

		// Check for tool calls
		if len(message.ToolCalls) > 0 {
			// Convert ToolCallResponse to ToolCallParam
			toolCallParams := make([]providers.ToolCallParam, len(message.ToolCalls))
			for i, tc := range message.ToolCalls {
				toolCallParams[i] = providers.ToolCallParam{
					ID:   tc.ID,
					Type: tc.Type,
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}

			// Add assistant message with tool calls
			a.AddMessage("assistant", "", toolCallParams, "")

			// Process each tool call
			for _, tc := range message.ToolCalls {
				a.State.ToolCallsCount++

				// Parse arguments
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					streamChan <- AgentEvent{
						Type: EventTypeToolError,
						Data: ToolErrorEvent{
							Name:  tc.Function.Name,
							Error: fmt.Sprintf("Failed to parse arguments: %v", err),
						},
					}
					continue
				}

				// Check permissions
				permissionResult := a.checkPermission(tc.Function.Name, args)
				if permissionResult == PermissionDeny {
					streamChan <- AgentEvent{
						Type: EventTypePermissionDenied,
						Data: PermissionEvent{
							Tool: tc.Function.Name,
							Args: args,
						},
					}
					// Add tool result as error
					a.addToolResult(tc.ID, tc.Function.Name, nil, "Operation denied by permission policy")
					continue
				}

				if permissionResult == PermissionAsk {
					// Request user approval
					streamChan <- AgentEvent{
						Type: EventTypePermissionRequest,
						Data: PermissionEvent{
							Tool: tc.Function.Name,
							Args: args,
						},
					}
					// Wait for approval (handled by TUI)
					// For now, continue with denial
					a.addToolResult(tc.ID, tc.Function.Name, nil, "Awaiting user approval")
					continue
				}

				// Execute tool
				streamChan <- AgentEvent{
					Type: EventTypeToolStart,
					Data: ToolEvent{
						Name: tc.Function.Name,
						Args: args,
					},
				}

				result, err := a.ToolRegistry.Execute(tc.Function.Name, args, a.Config)
				
				if err != nil {
					streamChan <- AgentEvent{
						Type: EventTypeToolError,
						Data: ToolErrorEvent{
							Name:  tc.Function.Name,
							Error: err.Error(),
						},
					}
					a.addToolResult(tc.ID, tc.Function.Name, nil, err.Error())
				} else {
					streamChan <- AgentEvent{
						Type: EventTypeToolComplete,
						Data: ToolEvent{
							Name:   tc.Function.Name,
							Args:   args,
							Result: result,
						},
					}
					a.addToolResult(tc.ID, tc.Function.Name, result, "")
				}
			}

			// Continue loop to get next model response
			continue
		}

		// No tool calls - we have a final response
		if message.Content != "" {
			a.AddMessage("assistant", message.Content, nil, "")
			streamChan <- AgentEvent{
				Type: EventTypeAssistantMessage,
				Data: message.Content,
			}
		}

		// Check finish reason
		if choice.FinishReason == "stop" || choice.FinishReason == "end_turn" {
			break
		}
	}

	return a.State, nil
}

// buildMessages converts internal messages to provider format
func (a *Agent) buildMessages() []providers.Message {
	messages := make([]providers.Message, 0, len(a.State.Messages)+1)

	// Add system prompt
	messages = append(messages, providers.Message{
		Role:    providers.RoleSystem,
		Content: a.GetSystemPrompt(),
	})

	// Add conversation messages
	for _, msg := range a.State.Messages {
		provMsg := providers.Message{
			Role:    providers.MessageRole(msg.Role),
			Content: msg.Content,
		}

		if len(msg.ToolCalls) > 0 {
			provMsg.ToolCalls = msg.ToolCalls
		}

		if msg.ToolCallID != "" {
			provMsg.ToolCallID = msg.ToolCallID
			provMsg.Name = msg.Name
		}

		messages = append(messages, provMsg)
	}

	return messages
}

// buildToolDefinitions converts tool registry to provider format
func (a *Agent) buildToolDefinitions() []providers.ToolDefinition {
	defs := make([]providers.ToolDefinition, 0)

	for _, name := range a.ToolRegistry.List() {
		tool, ok := a.ToolRegistry.Get(name)
		if !ok {
			continue
		}

		// Parse schema
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.Schema, &schema); err != nil {
			continue
		}

		defs = append(defs, providers.ToolDefinition{
			Type: "function",
			Function: &providers.FunctionSchema{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  schema,
			},
		})
	}

	return defs
}

// addToolResult adds a tool result message to the conversation
func (a *Agent) addToolResult(toolCallID, toolName string, result *tools.ToolResult, errorMsg string) {
	content := ""
	if result != nil {
		data, _ := json.Marshal(result.Data)
		content = string(data)
	} else {
		content = errorMsg
	}

	a.AddMessage("tool", content, nil, toolCallID)
}

// PermissionLevel constants
const (
	PermissionAllow config.PermissionLevel = "allow"
	PermissionAsk   config.PermissionLevel = "ask"
	PermissionDeny  config.PermissionLevel = "deny"
)

// checkPermission checks if a tool operation is allowed
func (a *Agent) checkPermission(toolName string, args map[string]interface{}) config.PermissionLevel {
	// Auto-approve mode
	if a.Config.Permissions.AutoApprove {
		return PermissionAllow
	}

	// Check specific tool permissions
	switch toolName {
	case "read_file", "list_directory", "glob", "grep":
		return a.checkPathPermission(args, a.Config.Permissions.Read, PermissionAllow)
	case "write_file", "edit_file":
		return a.checkPathPermission(args, a.Config.Permissions.Write, PermissionAsk)
	case "delete_file":
		if a.Config.Permissions.Delete != "" {
			return a.Config.Permissions.Delete
		}
		return PermissionAsk
	case "shell":
		return a.checkShellPermission(args, PermissionAsk)
	case "git_status", "git_diff", "git_log":
		return PermissionAllow
	}

	return PermissionAsk
}

// checkPathPermission checks permissions for path-based operations
func (a *Agent) checkPathPermission(args map[string]interface{}, rules []config.PermissionRule, defaultLevel config.PermissionLevel) config.PermissionLevel {
	pathArg, ok := args["path"].(string)
	if !ok {
		return defaultLevel
	}

	for _, rule := range rules {
		// Simple pattern matching (could be enhanced with glob matching)
		if strings.Contains(rule.Pattern, "*") {
			// Glob pattern
			matched, _ := matchPattern(rule.Pattern, pathArg)
			if matched {
				return rule.Level
			}
		} else if strings.Contains(pathArg, rule.Pattern) {
			return rule.Level
		}
	}

	return defaultLevel
}

// checkShellPermission checks permissions for shell commands
func (a *Agent) checkShellPermission(args map[string]interface{}, defaultLevel config.PermissionLevel) config.PermissionLevel {
	cmdArg, ok := args["command"].(string)
	if !ok {
		return defaultLevel
	}

	for _, rule := range a.Config.Permissions.Shell {
		if strings.Contains(cmdArg, rule.Pattern) {
			return rule.Level
		}
	}

	// Check for dangerous commands
	dangerousPatterns := []string{"rm -rf", "del /s", "format", "mkfs"}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(cmdArg, pattern) {
			return PermissionDeny
		}
	}

	return defaultLevel
}

// matchPattern performs simple glob-style pattern matching
func matchPattern(pattern, text string) (bool, error) {
	// Convert glob pattern to regex
	regex := strings.ReplaceAll(pattern, ".", "\\.")
	regex = strings.ReplaceAll(regex, "*", ".*")
	regex = strings.ReplaceAll(regex, "?", ".")
	regex = "^" + regex + "$"

	return regexpMatch(regex, text)
}

// regexpMatch is a simplified regex match helper
func regexpMatch(pattern, text string) (bool, error) {
	// In production, use regexp.Compile
	return strings.Contains(text, strings.TrimPrefix(strings.TrimSuffix(pattern, "$"), "^")), nil
}

// AgentEvent types
const (
	EventTypeUserMessage         = "user_message"
	EventTypeAssistantMessage    = "assistant_message"
	EventTypeToolStart           = "tool_start"
	EventTypeToolComplete        = "tool_complete"
	EventTypeToolError           = "tool_error"
	EventTypePermissionRequest   = "permission_request"
	EventTypePermissionDenied    = "permission_denied"
	EventTypeError               = "error"
	EventTypeStatus              = "status"
)

// AgentEvent represents an event from the agent
type AgentEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ToolEvent represents a tool execution event
type ToolEvent struct {
	Name   string                 `json:"name"`
	Args   map[string]interface{} `json:"args"`
	Result *tools.ToolResult      `json:"result,omitempty"`
}

// ToolErrorEvent represents a tool error event
type ToolErrorEvent struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// PermissionEvent represents a permission request event
type PermissionEvent struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}
