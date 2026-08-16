// Package chat provides the interactive chat/REPL functionality.
package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SSNamahsos/Mihani-Code/internal/llm"
)

// Session represents an interactive chat session.
type Session struct {
	Messages   []llm.Message
	LLMClient  llm.Client
	SystemPrompt string
}

// NewSession creates a new chat session.
func NewSession(client llm.Client) *Session {
	return &Session{
		Messages:  make([]llm.Message, 0),
		LLMClient: client,
		SystemPrompt: `You are Mihani Code, a specialized Go programming assistant created by Faz Pad Studio.
You help developers with Go code including:
- Writing clean, idiomatic Go code
- Explaining Go concepts and patterns
- Refactoring and optimizing Go code
- Debugging Go applications
- Best practices for Go development
- Testing in Go
- Concurrency patterns in Go

Always provide clear, concise answers with working Go code examples when relevant.
Format code in markdown code blocks with the "go" language identifier.`,
	}
}

// AddMessage adds a message to the conversation history.
func (s *Session) AddMessage(role, content string) {
	s.Messages = append(s.Messages, llm.Message{
		Role:    role,
		Content: content,
	})
}

// Clear clears the conversation history.
func (s *Session) Clear() {
	s.Messages = make([]llm.Message, 0)
}

// GetContext returns the current conversation context.
func (s *Session) GetContext() []llm.Message {
	// Include system prompt as first message
	messages := make([]llm.Message, 0, len(s.Messages)+1)
	messages = append(messages, llm.Message{
		Role:    "system",
		Content: s.SystemPrompt,
	})
	messages = append(messages, s.Messages...)
	return messages
}

// Chat sends a message and gets a response from the LLM.
func (s *Session) Chat(ctx context.Context, userMessage string) (string, error) {
	// Add user message to history
	s.AddMessage("user", userMessage)

	// Get full context
	messages := s.GetContext()

	// Send to LLM
	response, err := s.LLMClient.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("chat failed: %w", err)
	}

	// Add assistant response to history
	s.AddMessage("assistant", response)

	return response, nil
}

// ExplainCode explains the provided Go code.
func (s *Session) ExplainCode(ctx context.Context, code string) (string, error) {
	return s.LLMClient.CodeExplain(ctx, code)
}

// RefactorCode refactors Go code based on instructions.
func (s *Session) RefactorCode(ctx context.Context, code, instruction string) (string, error) {
	return s.LLMClient.CodeRefactor(ctx, code, instruction)
}

// GenerateCode generates Go code from a description.
func (s *Session) GenerateCode(ctx context.Context, prompt string) (string, error) {
	return s.LLMClient.CodeGenerate(ctx, prompt)
}

// WithSystemPrompt sets a custom system prompt.
func (s *Session) WithSystemPrompt(prompt string) *Session {
	s.SystemPrompt = prompt
	return s
}

// TrimHistory trims the conversation history to prevent token limits.
func (s *Session) TrimHistory(maxMessages int) {
	if len(s.Messages) <= maxMessages {
		return
	}
	// Keep the most recent messages
	s.Messages = s.Messages[len(s.Messages)-maxMessages:]
}

// GetStats returns session statistics.
func (s *Session) GetStats() SessionStats {
	userMsgs := 0
	assistantMsgs := 0
	totalChars := 0

	for _, m := range s.Messages {
		totalChars += len(m.Content)
		switch m.Role {
		case "user":
			userMsgs++
		case "assistant":
			assistantMsgs++
		}
	}

	return SessionStats{
		UserMessages:      userMsgs,
		AssistantMessages: assistantMsgs,
		TotalCharacters:   totalChars,
		MessageCount:      len(s.Messages),
	}
}

// SessionStats contains statistics about a chat session.
type SessionStats struct {
	UserMessages      int
	AssistantMessages int
	TotalCharacters   int
	MessageCount      int
}

// SimpleChat performs a one-off chat without maintaining history.
func SimpleChat(ctx context.Context, client llm.Client, systemPrompt, userMessage string) (string, error) {
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}
	return client.Chat(ctx, messages)
}

// ChatWithTimeout performs a chat operation with a timeout.
func ChatWithTimeout(ctx context.Context, client llm.Client, messages []llm.Message, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.Chat(ctx, messages)
}

// ExtractCodeBlocks extracts Go code blocks from a response.
func ExtractCodeBlocks(response string) []string {
	var blocks []string
	
	lines := strings.Split(response, "\n")
	var currentBlock strings.Builder
	inBlock := false
	blockLang := ""

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if !inBlock {
				// Starting a code block
				inBlock = true
				blockLang = strings.TrimPrefix(line, "```")
				currentBlock.Reset()
			} else {
				// Ending a code block
				if blockLang == "go" || blockLang == "" {
					blocks = append(blocks, currentBlock.String())
				}
				inBlock = false
				blockLang = ""
			}
		} else if inBlock {
			currentBlock.WriteString(line)
			currentBlock.WriteString("\n")
		}
	}

	return blocks
}
