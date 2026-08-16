package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SSNamahsos/Mihani-Code/internal/llm"
)

// Session represents a chat session with conversation history.
type Session struct {
	client    llm.Client
	messages  []llm.Message
	systemMsg string
	createdAt time.Time
	updatedAt time.Time
}

// NewSession creates a new chat session.
func NewSession(client llm.Client) *Session {
	return &Session{
		client:    client,
		messages:  make([]llm.Message, 0),
		systemMsg: "You are Mihani Code, an expert Go programming assistant created by Faz Pad Studio. You specialize in Go development, providing code examples, explanations, refactoring advice, and best practices. Always write idiomatic Go code following effective Go principles. When showing code, use proper Go formatting. Be concise but thorough.",
		createdAt: time.Now(),
		updatedAt: time.Now(),
	}
}

// Chat sends a message and gets a response.
func (s *Session) Chat(ctx context.Context, userMessage string) (string, error) {
	userMessage = llm.SanitizeInput(userMessage)
	
	// Build messages array with system prompt
	messages := make([]llm.Message, 0, len(s.messages)+2)
	
	if s.systemMsg != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: s.systemMsg,
		})
	}
	
	messages = append(messages, s.messages...)
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userMessage,
	})

	response, err := s.client.ChatStream(ctx, messages, nil)
	if err != nil {
		return "", err
	}

	// Add to conversation history
	s.messages = append(s.messages, llm.Message{
		Role:    "user",
		Content: userMessage,
	})
	s.messages = append(s.messages, llm.Message{
		Role:    "assistant",
		Content: response,
	})
	
	s.updatedAt = time.Now()

	return response, nil
}

// ChatWithSystem sends a message with a custom system prompt.
func (s *Session) ChatWithSystem(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	originalSystem := s.systemMsg
	s.systemMsg = systemPrompt
	defer func() { s.systemMsg = originalSystem }()
	
	return s.Chat(ctx, userMessage)
}

// ExplainCode explains code in the given content.
func (s *Session) ExplainCode(ctx context.Context, code string) (string, error) {
	prompt := fmt.Sprintf(`Please explain this Go code clearly and concisely:

%s

Provide:
1. A brief overview of what the code does
2. Key components and their purposes
3. Any notable Go patterns or best practices used
4. Potential improvements if applicable`, code)

	return s.Chat(ctx, prompt)
}

// RefactorCode suggests refactoring for the given code.
func (s *Session) RefactorCode(ctx context.Context, code, instruction string) (string, error) {
	prompt := fmt.Sprintf(`Please refactor this Go code with the following instruction: %s

Original code:
%s

Provide:
1. The refactored code in a Go code block
2. A brief explanation of the changes made
3. Why these changes improve the code

Focus on:
- Idiomatic Go patterns
- Readability and maintainability
- Performance improvements if applicable
- Error handling best practices`, instruction, code)

	return s.Chat(ctx, prompt)
}

// GenerateCode generates Go code from a description.
func (s *Session) GenerateCode(ctx context.Context, description string) (string, error) {
	prompt := fmt.Sprintf(`Generate Go code based on this description: %s

Requirements:
- Write idiomatic, production-ready Go code
- Include proper error handling
- Add comments for complex logic
- Follow Go naming conventions
- Include example usage if helpful

Provide the complete, working code in a Go code block.`, description)

	return s.Chat(ctx, prompt)
}

// ReviewCode provides a code review for the given code.
func (s *Session) ReviewCode(ctx context.Context, code string) (string, error) {
	prompt := fmt.Sprintf(`Please review this Go code and provide constructive feedback:

%s

Review aspects:
1. Code correctness and potential bugs
2. Adherence to Go best practices and idioms
3. Error handling completeness
4. Performance considerations
5. Readability and maintainability
6. Security concerns if any
7. Suggestions for improvement

Be specific and provide code examples for suggested changes.`, code)

	return s.Chat(ctx, prompt)
}

// DebugHelp helps debug an issue.
func (s *Session) DebugHelp(ctx context.Context, problem, code string) (string, error) {
	prompt := fmt.Sprintf(`Help me debug this Go code issue:

Problem description:
%s

Related code:
%s

Please:
1. Identify the likely cause of the issue
2. Explain why it's happening
3. Provide a fix with corrected code
4. Suggest how to prevent similar issues in the future`, problem, code)

	return s.Chat(ctx, prompt)
}

// TestGeneration generates tests for the given code.
func (s *Session) GenerateTests(ctx context.Context, code string) (string, error) {
	prompt := fmt.Sprintf(`Generate comprehensive unit tests for this Go code:

%s

Requirements:
- Use Go's testing package
- Cover normal cases, edge cases, and error conditions
- Follow Go testing conventions
- Include table-driven tests where appropriate
- Add clear test names that describe what's being tested
- Ensure tests are deterministic and don't have side effects

Provide the complete test file content.`, code)

	return s.Chat(ctx, prompt)
}

// Clear resets the conversation history.
func (s *Session) Clear() {
	s.messages = make([]llm.Message, 0)
	s.updatedAt = time.Now()
}

// GetHistory returns the conversation history.
func (s *Session) GetHistory() []llm.Message {
	return s.messages
}

// GetMessagesCount returns the number of messages in history.
func (s *Session) GetMessagesCount() int {
	return len(s.messages)
}

// SetMaxHistory limits the conversation history to n messages.
func (s *Session) SetMaxHistory(n int) {
	if n <= 0 {
		return
	}
	
	if len(s.messages) > n {
		s.messages = s.messages[len(s.messages)-n:]
	}
}

// ExportConversation exports the conversation as a string.
func (s *Session) ExportConversation() string {
	var sb strings.Builder
	
	sb.WriteString("# Mihani Code Conversation\n")
	sb.WriteString(fmt.Sprintf("Started: %s\n\n", s.createdAt.Format(time.RFC3339)))
	
	for _, msg := range s.messages {
		role := strings.Title(msg.Role)
		sb.WriteString(fmt.Sprintf("## %s\n\n", role))
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}
	
	return sb.String()
}
