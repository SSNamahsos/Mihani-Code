package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF0000")). // Red for "Mihani"
			MarginBottom(1)

	codeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	subtitleStyle = lipgloss.NewStyle().
			Faint(true).
			MarginBottom(1)

	userMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF00")).
				Bold(true).
				MarginTop(1).
				PaddingLeft(2)

	assistantMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				MarginTop(1).
				PaddingLeft(2)

	toolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00")).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true)

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#333333")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FFFF")).
			PaddingLeft(2)

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")). // Red for "Mihani"
			Bold(true)
)

// Message types
type (
	errMsg error
)

// ConversationEntry represents an entry in the conversation
type ConversationEntry struct {
	Type    string
	Content string
	Extra   interface{}
}

// Model represents the TUI state
type Model struct {
	config         *Config
	conversation   []ConversationEntry
	input          string
	cursorBlink    bool
	quitting       bool
	width          int
	height         int
	scrollOffset   int
	currentModel   string
	currentAgent   string
	sessionID      string
	workingDir     string
	permissionMode string
	tasks          []TaskItem
	isProcessing   bool
}

// Config holds TUI configuration
type Config struct {
	ShowToolCalls bool
	CompactMode   bool
	Theme         string
}

// TaskItem represents a task in the task list
type TaskItem struct {
	Description string
	State       string
}

// NewModel creates a new TUI model
func NewModel(cfg *Config, workDir, model, sessionID string) *Model {
	return &Model{
		config:         cfg,
		conversation:   make([]ConversationEntry, 0),
		input:          "",
		cursorBlink:    true,
		quitting:       false,
		currentModel:   model,
		sessionID:      sessionID,
		workingDir:     workDir,
		currentAgent:   "build",
		permissionMode: "ask",
		tasks:          make([]TaskItem, 0),
		isProcessing:   false,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if m.input != "" && !m.isProcessing {
				// Send user message
				cmd := m.handleInput()
				return m, cmd
			}
		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		case tea.KeyRunes:
			m.input += msg.String()
		case tea.KeyEsc:
			// Cancel current operation
			m.addEntry("system", "Operation cancelled by user")
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case errMsg:
		m.addEntry("error", msg.Error())
	case cursorTickMsg:
		m.cursorBlink = !m.cursorBlink
		return m, cursorBlinkCmd()
	}

	return m, nil
}

// handleInput processes user input
func (m *Model) handleInput() tea.Cmd {
	input := strings.TrimSpace(m.input)
	if input == "" {
		return nil
	}

	// Check for slash commands
	if strings.HasPrefix(input, "/") {
		return m.handleCommand(input)
	}

	// Add user message to conversation
	m.addEntry("user", input)
	m.input = ""

	// Signal that we're processing
	m.isProcessing = true

	// Return command to process with agent (handled by main)
	return func() tea.Msg {
		return agentRequestMsg{prompt: input}
	}
}

// handleCommand handles slash commands
func (m *Model) handleCommand(cmd string) tea.Cmd {
	parts := strings.SplitN(cmd, " ", 2)
	command := parts[0]
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	m.input = ""

	switch command {
	case "/help", "/h":
		m.showHelp()
	case "/clear", "/c":
		m.conversation = make([]ConversationEntry, 0)
	case "/exit", "/quit", "/q":
		m.quitting = true
		return tea.Quit
	case "/model":
		if args != "" {
			m.currentModel = args
			m.addEntry("system", fmt.Sprintf("Model changed to: %s", args))
		} else {
			m.addEntry("system", fmt.Sprintf("Current model: %s", m.currentModel))
		}
	case "/agent":
		if args != "" {
			m.currentAgent = args
			m.addEntry("system", fmt.Sprintf("Agent mode changed to: %s", args))
		} else {
			m.addEntry("system", fmt.Sprintf("Current agent mode: %s", m.currentAgent))
		}
	case "/status":
		m.showStatus()
	case "/tasks":
		m.showTasks()
	case "/compact":
		m.addEntry("system", "Compacting conversation history...")
	default:
		m.addEntry("error", fmt.Sprintf("Unknown command: %s", command))
	}

	return nil
}

// showHelp displays available commands
func (m *Model) showHelp() {
	helpText := `
Available Commands:
  /help, /h          Show this help message
  /clear, /c         Clear conversation history
  /exit, /quit, /q   Exit Mihani Code
  /model <name>      Change or show current model
  /agent <mode>      Change or show agent mode (build/plan)
  /status            Show current status
  /tasks             Show task list
  /compact           Compact conversation history

Just type your request to start a coding task.
Examples:
  "Fix the authentication bug"
  "Add a login endpoint"
  "Why are the tests failing?"
  "Refactor the database layer"`

	m.addEntry("system", strings.TrimSpace(helpText))
}

// showStatus displays current status
func (m *Model) showStatus() {
	status := fmt.Sprintf(`
Status:
  Model: %s
  Agent Mode: %s
  Working Directory: %s
  Session: %s
  Permission Mode: %s`,
		m.currentModel,
		m.currentAgent,
		m.workingDir,
		m.sessionID,
		m.permissionMode)

	m.addEntry("system", strings.TrimSpace(status))
}

// showTasks displays the task list
func (m *Model) showTasks() {
	if len(m.tasks) == 0 {
		m.addEntry("system", "No active tasks")
		return
	}

	var sb strings.Builder
	sb.WriteString("Tasks:\n")
	for i, task := range m.tasks {
		icon := "○"
		if task.State == "in_progress" {
			icon = "●"
		} else if task.State == "completed" {
			icon = "✓"
		} else if task.State == "failed" {
			icon = "✗"
		}
		sb.WriteString(fmt.Sprintf("  %d. %s %s\n", i+1, icon, task.Description))
	}

	m.addEntry("system", sb.String())
}

// addEntry adds a conversation entry
func (m *Model) addEntry(entryType, content string) {
	m.conversation = append(m.conversation, ConversationEntry{
		Type:    entryType,
		Content: content,
	})

	// Auto-scroll to bottom
	m.scrollOffset = len(m.conversation) - 1
}

// addToolEntry adds a tool-related entry
func (m *Model) addToolEntry(toolName, status string, result interface{}) {
	content := fmt.Sprintf("[%s] %s", toolName, status)
	if result != nil {
		content += fmt.Sprintf(" - %v", result)
	}
	m.addEntry("tool", content)
}

// View renders the UI
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader())

	// Conversation
	b.WriteString(m.renderConversation())

	// Input area
	b.WriteString(m.renderInput())

	// Status bar
	b.WriteString(m.renderStatusBar())

	return b.String()
}

// renderHeader renders the header section
func (m Model) renderHeader() string {
	header := fmt.Sprintf("%s%s",
		titleStyle.Render("Mihani"),
		codeStyle.Render(" Code"))

	subtitle := subtitleStyle.Render("Made by Faz Pad Studio")

	modelInfo := lipgloss.NewStyle().
		Align(lipgloss.Right).
		Render(fmt.Sprintf("Model: %s | Agent: %s", m.currentModel, m.currentAgent))

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		subtitle,
		modelInfo,
		"\n")
}

// renderConversation renders the conversation history
func (m Model) renderConversation() string {
	if len(m.conversation) == 0 {
		welcomeMsg := `Welcome to Mihani Code!

I'm your AI coding assistant. I can help you with:
• Fixing bugs and errors
• Adding new features
• Refactoring code
• Writing tests
• Explaining code
• Running commands and analyzing output

Just type your request below to get started!`

		return lipgloss.NewStyle().
			Margin(1, 2).
			Render(welcomeMsg) + "\n\n"
	}

	var b strings.Builder
	start := m.scrollOffset
	if start < 0 {
		start = 0
	}

	for i := start; i < len(m.conversation); i++ {
		entry := m.conversation[i]
		rendered := m.renderEntry(entry)
		b.WriteString(rendered)
		b.WriteString("\n")
	}

	return b.String()
}

// renderEntry renders a single conversation entry
func (m Model) renderEntry(entry ConversationEntry) string {
	switch entry.Type {
	case "user":
		return userMessageStyle.Render("> " + entry.Content)
	case "assistant":
		return assistantMessageStyle.Render(entry.Content)
	case "tool":
		return toolStyle.Render("  → " + entry.Content)
	case "error":
		return errorStyle.Render("  ✗ " + entry.Content)
	case "system":
		return lipgloss.NewStyle().
			Faint(true).
			Render("  • " + entry.Content)
	default:
		return entry.Content
	}
}

// renderInput renders the input area
func (m Model) renderInput() string {
	cursor := "▋"
	if !m.cursorBlink || m.isProcessing {
		cursor = " "
	}

	prompt := promptStyle.Render("Mihani") + "> "

	if m.isProcessing {
		return inputStyle.Render(prompt + "Processing...")
	}

	return inputStyle.Render(prompt + m.input + cursor)
}

// renderStatusBar renders the status bar
func (m Model) renderStatusBar() string {
	status := fmt.Sprintf(" %s | %s | %s ",
		m.currentModel,
		m.workingDir,
		m.sessionID)

	return statusBarStyle.Render(status)
}

// Message types for agent communication
type agentRequestMsg struct {
	prompt string
}

type agentResponseMsg struct {
	content string
	done    bool
}

type toolEventMsg struct {
	name   string
	status string
	result interface{}
}

// Cursor blink command
func cursorBlinkCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return cursorTickMsg{}
	})
}

type cursorTickMsg struct{}

// Import handled at top of file
