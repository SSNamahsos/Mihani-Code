package agent

import (
"fmt"
"strings"
"time"

"github.com/SSNamahsos/Mihani-Code/internal/config"
"github.com/SSNamahsos/Mihani-Code/internal/filesystem"
"github.com/SSNamahsos/Mihani-Code/internal/git"
"github.com/SSNamahsos/Mihani-Code/internal/llm"
"github.com/SSNamahsos/Mihani-Code/internal/providers"
"github.com/SSNamahsos/Mihani-Code/internal/shell"
"github.com/SSNamahsos/Mihani-Code/internal/tools"
)

// Agent represents the coding agent
type Agent struct {
Config      *config.Config
Provider    llm.Provider
FileSystem  *filesystem.FileSystem
Shell       *shell.Shell
Git         *git.Git
ToolRegistry *tools.Registry
Messages    []llm.Message
MaxIterations int
MaxToolCalls  int
}

// ToolCallEvent represents a tool execution event
type ToolCallEvent struct {
Name      string
Arguments map[string]interface{}
Result    interface{}
Error     string
Duration  time.Duration
}

// AgentCallback defines callbacks for agent events
type AgentCallback interface {
OnToolStart(name string, args map[string]interface{})
OnToolComplete(event *ToolCallEvent)
OnMessage(content string)
}

// NewAgent creates a new coding agent
func NewAgent(cfg *config.Config) (*Agent, error) {
wd, _ := filesystem.Getwd()

fsys := filesystem.NewFileSystem(wd, int64(cfg.MaxFileSize))
sh := shell.NewShell(wd, time.Duration(cfg.CommandTimeout)*time.Second)
gitClient := git.NewGit(wd)
toolRegistry := tools.NewRegistry(fsys, sh, gitClient)

var provider llm.Provider
switch strings.ToLower(cfg.Provider) {
case "anthropic":
provider = providers.NewAnthropicProvider(cfg.APIKey, cfg.Model)
default:
provider = providers.NewOpenAICompatibleProvider(cfg.BaseURL, cfg.APIKey, cfg.Model)
}

return &Agent{
Config:       cfg,
Provider:     provider,
FileSystem:   fsys,
Shell:        sh,
Git:          gitClient,
ToolRegistry: toolRegistry,
Messages:     make([]llm.Message, 0),
MaxIterations: cfg.MaxIterations,
MaxToolCalls:  cfg.MaxToolCalls,
}, nil
}

// Run executes the agent loop for a given task
func (a *Agent) Run(task string, callback AgentCallback) (string, error) {
a.Messages = append(a.Messages, llm.Message{
Role:    "user",
Content: task,
})

toolDefinitions := a.ToolRegistry.ToDefinitions()
toolCallCount := 0
iteration := 0

for iteration < a.MaxIterations {
iteration++

req := &llm.ChatRequest{
Messages: a.buildMessages(),
Tools:    toolDefinitions,
Model:    a.Config.Model,
}

resp, err := a.Provider.Chat(req)
if err != nil {
return "", fmt.Errorf("LLM request failed: %w", err)
}

if resp.Content != "" && callback != nil {
callback.OnMessage(resp.Content)
}

if len(resp.ToolCalls) == 0 {
return resp.Content, nil
}

for _, tc := range resp.ToolCalls {
toolCallCount++
if toolCallCount > a.MaxToolCalls {
return "", fmt.Errorf("exceeded maximum tool calls (%d)", a.MaxToolCalls)
}

tool, ok := a.ToolRegistry.Get(tc.Name)
if !ok {
a.Messages = append(a.Messages, llm.Message{
Role:    "tool",
Content: fmt.Sprintf("Error: Unknown tool '%s'", tc.Name),
})
continue
}

if callback != nil {
callback.OnToolStart(tc.Name, tc.Arguments)
}

startTime := time.Now()
result, err := tool.Handler(tc.Arguments)
duration := time.Since(startTime)

event := &ToolCallEvent{
Name:      tc.Name,
Arguments: tc.Arguments,
Result:    result,
Duration:  duration,
}

if err != nil {
event.Error = err.Error()
result = map[string]interface{}{"success": false, "error": err.Error()}
}

if callback != nil {
callback.OnToolComplete(event)
}

resultJSON := formatToolResult(tc.ID, result)
a.Messages = append(a.Messages, llm.Message{
Role:    "tool",
Content: resultJSON,
})
}
}

return "", fmt.Errorf("exceeded maximum iterations (%d)", a.MaxIterations)
}

func (a *Agent) buildMessages() []llm.Message {
systemPrompt := `You are Mihani Code, an AI coding assistant that operates directly in the user's project directory.

Your capabilities:
- You can read, write, edit, and delete files
- You can list directories and search for files
- You can execute shell commands (tests, builds, linting, etc.)
- You can check git status, diffs, and history

When given a task:
1. First understand the project structure by exploring relevant directories
2. Read existing code to understand the current implementation
3. Make targeted changes using edit_file or write_file
4. Run tests or validation commands to verify your changes
5. If tests fail, analyze the errors and fix them
6. Continue iterating until the task is complete
7. Summarize what you changed at the end

Important guidelines:
- Always verify file contents before editing
- Use edit_file for small changes, write_file for new files or major rewrites
- Run tests after making code changes
- Check git diff to review your changes
- Preserve existing code style and conventions
- Never claim you made changes without actually executing the tool
- If unsure about something, explore the codebase first

Be methodical and thorough. Your goal is to actually complete the task, not just talk about it.`

messages := []llm.Message{{Role: "system", Content: systemPrompt}}
messages = append(messages, a.Messages...)
return messages
}

func formatToolResult(toolCallID string, result interface{}) string {
// Simple JSON-like formatting for tool results
return fmt.Sprintf(`{"tool_call_id": "%s", "result": %#v}`, toolCallID, result)
}
