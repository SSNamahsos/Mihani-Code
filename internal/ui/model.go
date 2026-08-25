package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/session"
	"github.com/SSNamahsos/Mihani-Code/internal/usage"
)

type eventMsg agent.Event
type resultMsg struct{ err error }
type tickMsg time.Time
type modelsMsg struct {
	models []string
	err    error
}

type commandItem struct {
	name        string
	description string
}

var commands = []commandItem{
	{name: "/help", description: "Show keyboard shortcuts and commands"},
	{name: "/clear", description: "Clear the visible conversation"},
	{name: "/new", description: "Start a new session"},
	{name: "/resume", description: "Resume a previous session"},
	{name: "/mode", description: "Switch between build, plan, research, and ask"},
	{name: "/providers", description: "Show configured AI providers"},
	{name: "/models", description: "Show models for the active provider"},
	{name: "/connect", description: "Connect a provider and discover its models"},
	{name: "/git", description: "Show git status or diff"},
	{name: "/status", description: "Show workspace and session status"},
	{name: "/session", description: "Show the current session ID"},
	{name: "/mcp", description: "Show configured MCP servers"},
	{name: "/undo", description: "Restore the latest Mihani file snapshot"},
	{name: "/settings", description: "Open Mihani settings"},
	{name: "/quit", description: "Exit Mihani Code"},
}

type Model struct {
	cfg     config.Config
	root    string
	version string

	input textarea.Model
	view  viewport.Model

	agent *agent.Agent

	blocks          []*block
	activeAssistant int // index of the assistant block receiving stream deltas
	activeTool      int // index of the running tool block

	busy   bool
	queued string

	status   string
	activity string
	spinner  int
	tokens   int

	width, height int

	cancel  context.CancelFunc
	program *tea.Program

	pendingApproval chan bool
	approvalTool    string
	approvalDetail  string
	approvalPreview string
	approveAll      bool

	overlay       string
	overlayItems  []overlayItem
	overlayIndex  int
	resumeRecords []session.Record

	connectOpen   bool
	connecting    bool
	connectField  int
	connectInput  textarea.Model
	connectFields [3]string
	connectName   string
	connectURL    string
	connectError  string

	sessionID    string
	modeIndex    int
	commandIndex int
	quitting     bool

	spend float64 // rolling 24h spend for the current provider
}

// New builds the TUI model. resumeID optionally restores a specific session;
// otherwise the most recent session for this workspace is restored. An empty
// initialPrompt leaves the composer untouched.
func New(cfg config.Config, version, resumeID, initialPrompt string) (Model, error) {
	root, err := os.Getwd()
	if err != nil {
		return Model{}, err
	}
	ta := textarea.New()
	ta.Placeholder = "Ask Mihani to inspect, build, fix, or explain…"
	ta.Focus()
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = -1

	connectInput := textarea.New()
	connectInput.SetHeight(1)

	m := Model{
		cfg:             cfg,
		root:            root,
		version:         version,
		input:           ta,
		view:            viewport.New(0, 10),
		connectInput:    connectInput,
		agent:           &agent.Agent{Cfg: cfg, Root: root},
		activeAssistant: -1,
		activeTool:      -1,
		status:          "ready",
		sessionID:       session.NewID(),
		modeIndex:       0,
	}
	m.agent.MaxIterations = cfg.MaxIterations
	m.refreshSpend()

	resumed := false
	if resumeID != "" {
		if record, e := session.Load(resumeID); e == nil {
			resumed = m.restore(record)
		}
	} else if record, e := session.LatestForWorkspace(root); e == nil && len(record.History) > 0 {
		resumed = m.restore(record)
	}
	if resumed {
		m.appendBlock(&block{kind: blockInfo, content: "resumed previous session " + shortID(m.sessionID)})
	}
	if initialPrompt != "" {
		m.input.SetValue(initialPrompt)
		m.input.Focus()
	}
	return m, nil
}

func (m *Model) restore(record session.Record) bool {
	if record.Workspace != m.root || len(record.History) == 0 {
		return false
	}
	m.sessionID = firstNonEmptyStr(record.ID, m.sessionID)
	m.agent.Restore(record.History)
	replayTranscript(m, record.History)
	return true
}

func replayTranscript(m *Model, history []map[string]any) {
	for _, msg := range history {
		switch fmt.Sprint(msg["role"]) {
		case "user":
			if s, ok := msg["content"].(string); ok && s != "" {
				m.blocks = append(m.blocks, &block{kind: blockUser, content: s})
			}
		case "assistant":
			m.replayAssistant(msg)
		}
	}
}

func (m *Model) replayAssistant(msg map[string]any) {
	switch content := msg["content"].(type) {
	case string:
		if strings.TrimSpace(content) != "" {
			m.blocks = append(m.blocks, &block{kind: blockAssistant, content: content, finalized: true})
		}
	case []map[string]any:
		var texts []string
		for _, part := range content {
			if fmt.Sprint(part["type"]) == "text" {
				texts = append(texts, fmt.Sprint(part["text"]))
			}
		}
		if joined := strings.Join(texts, "\n\n"); strings.TrimSpace(joined) != "" {
			m.blocks = append(m.blocks, &block{kind: blockAssistant, content: joined, finalized: true})
		}
	case []any:
		var texts []string
		for _, item := range content {
			part, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if fmt.Sprint(part["type"]) == "text" {
				texts = append(texts, fmt.Sprint(part["text"]))
			}
		}
		if joined := strings.Join(texts, "\n\n"); strings.TrimSpace(joined) != "" {
			m.blocks = append(m.blocks, &block{kind: blockAssistant, content: joined, finalized: true})
		}
	}
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// Run starts the interactive TUI.
func Run(cfg config.Config, version, resumeID, initialPrompt string) error {
	m, err := New(cfg, version, resumeID, initialPrompt)
	if err != nil {
		return err
	}
	p := tea.NewProgram(&m, tea.WithAltScreen())
	m.program = p
	_, runErr := p.Run()
	m.agent.Close()
	return runErr
}

func (m *Model) Init() tea.Cmd { return textarea.Blink }

func tick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = x.Width, x.Height
		m.input.SetWidth(maxInt(20, x.Width-6))
		m.relayout()
		return m, nil

	case tea.KeyMsg:
		if next, cmd, handled := m.handleKey(x); handled {
			return next, cmd
		}

	case eventMsg:
		m.handle(agent.Event(x))

	case tickMsg:
		if m.busy || m.pendingApproval != nil {
			m.spinner++
			m.invalidateAnimated()
			m.refreshView()
			return m, tick()
		}

	case modelsMsg:
		return m, m.finishConnect(x)

	case resultMsg:
		cmd := m.finishTurn(x)
		return m, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.resizeComposer()
	return m, cmd
}

// handleKey processes global key bindings; ok reports whether the key was consumed.
func (m *Model) handleKey(x tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	key := x.String()

	switch {
	case key == "ctrl+c":
		if m.connectOpen {
			m.closeConnect()
			return m, nil, true
		}
		if m.overlay != "" {
			m.closeOverlay()
			return m, nil, true
		}
		if m.pendingApproval != nil {
			m.answerApproval(false)
			m.interrupt()
			return m, nil, true
		}
		if m.busy {
			m.interrupt()
			return m, nil, true
		}
		m.quitting = true
		m.saveSession()
		return m, tea.Quit, true

	case m.connectOpen:
		return m, m.updateConnectKey(x), true

	case m.overlay != "":
		return m, m.updateOverlayKey(x), true

	case m.pendingApproval != nil:
		switch key {
		case "y", "Y", "enter":
			m.answerApproval(true)
		case "a", "A":
			m.approveAll = true
			m.answerApproval(true)
		case "n", "N", "esc":
			m.answerApproval(false)
		}
		return m, nil, true

	case key == "esc":
		if m.showCommands() {
			m.input.Reset()
			m.commandIndex = 0
		} else if m.busy {
			m.interrupt()
		} else if m.input.Value() != "" {
			m.input.Reset()
		}
		return m, nil, true

	case key == "pgup":
		m.view.LineUp(m.view.Height / 2)
		return m, nil, true
	case key == "pgdown":
		m.view.LineDown(m.view.Height / 2)
		return m, nil, true

	case key == "ctrl+j", key == "alt+enter", key == "shift+enter":
		m.input.InsertString("\n")
		m.resizeComposer()
		return m, nil, true

	case m.showCommands():
		items := m.filteredCommands()
		switch key {
		case "tab", "down":
			if len(items) > 0 {
				m.commandIndex = (m.commandIndex + 1) % len(items)
			}
			return m, nil, true
		case "shift+tab", "up":
			if len(items) > 0 {
				m.commandIndex = (m.commandIndex - 1 + len(items)) % len(items)
			}
			return m, nil, true
		case "enter":
			if len(items) > 0 {
				m.input.SetValue(items[m.commandIndex].name + " ")
				m.commandIndex = 0
			}
			return m, nil, true
		}

	case key == "tab":
		m.modeIndex = (m.modeIndex + 1) % len(modes)
		return m, nil, true
	case key == "shift+tab":
		m.modeIndex = (m.modeIndex - 1 + len(modes)) % len(modes)
		return m, nil, true

	case key == "enter":
		value := strings.TrimSpace(m.input.Value())
		if value == "" {
			return m, nil, true
		}
		if m.busy {
			if !strings.HasPrefix(value, "/") {
				m.queued = value
				m.input.Reset()
				m.status = "queued"
				return m, nil, true
			}
			return m, nil, true
		}
		m.input.Reset()
		return m, m.submit(value), true
	}
	return m, nil, false
}

func (m *Model) interrupt() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.pendingApproval != nil {
		m.answerApproval(false)
	}
	m.status = "cancelling…"
}

// answerApproval replies to a pending permission request exactly once.
func (m *Model) answerApproval(ok bool) {
	if ch := m.pendingApproval; ch != nil {
		select {
		case ch <- ok:
		default:
		}
	}
	m.pendingApproval = nil
	m.approvalPreview = ""
	if m.busy {
		m.status = "working"
	} else {
		m.status = "ready"
	}
}

// refreshSpend re-reads the rolling 24h total for the active provider.
func (m *Model) refreshSpend() {
	m.spend = usage.WindowSum(m.cfg.CurrentProvider)
}

// budgetBlock returns a user-facing message when the daily cap is exhausted,
// or "" when the turn may proceed.
func (m *Model) budgetBlock() string {
	budget := m.cfg.Budget()
	if budget <= 0 {
		return ""
	}
	if m.spend < budget {
		return ""
	}
	reset := usage.NextReset(m.cfg.CurrentProvider)
	when := "soon"
	if !reset.IsZero() {
		d := time.Until(reset).Round(time.Minute)
		if d > 0 {
			when = d.String()
		}
	}
	return fmt.Sprintf(
		"Mihani daily limit reached: $%.2f of $%.2f used in the last 24h. "+
			"Budget resets in %s — switch provider with /providers, use a free model, or raise budget_usd in settings.",
		m.spend, budget, when)
}

func (m *Model) submit(s string) tea.Cmd {
	if strings.HasPrefix(s, "/") {
		return m.command(s)
	}
	return m.startTurn(s)
}

func (m *Model) startTurn(prompt string) tea.Cmd {
	if blocked := m.budgetBlock(); blocked != "" {
		m.appendBlock(&block{kind: blockError, content: blocked})
		m.refreshView()
		return nil
	}
	m.closeActiveAssistant()
	m.blocks = append(m.blocks, &block{kind: blockUser, content: prompt})
	m.busy = true
	m.approveAll = false
	m.status = "working"
	m.activity = "thinking"
	m.refreshView()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	modeName := currentMode(m.modeIndex).name
	cfg := m.cfg
	return tea.Sequence(tick(), func() tea.Msg {
		a := m.agent
		a.Cfg = cfg
		a.Root = m.root
		err := a.Send(ctx, prompt, modeName,
			func(name string, input map[string]any) bool {
				if m.approveAll || cfg.AutoConfirm {
					return true
				}
				if modeName == "plan" || modeName == "research" || modeName == "ask" {
					switch name {
					case "write_file", "edit_file", "delete_file", "bash":
						m.program.Send(eventMsg(agent.Event{
							Kind: "activity",
							Text: modeName + " mode blocked " + name,
						}))
						return false
					}
				}
				approval := make(chan bool, 1)
				if m.program != nil {
					m.program.Send(eventMsg(agent.Event{Kind: "permission", Tool: name, Input: input, Approval: approval}))
				}
				return <-approval
			},
			func(ev agent.Event) {
				if m.program != nil {
					m.program.Send(eventMsg(ev))
				}
			})
		return resultMsg{err}
	})
}

func (m *Model) finishTurn(x resultMsg) tea.Cmd {
	m.closeActiveAssistant()
	m.busy = false
	m.activity = ""
	m.status = "ready"
	m.saveSession()
	err := x.err
	cancelled := err != nil && (errors.Is(err, context.Canceled) ||
		strings.Contains(err.Error(), "context canceled") ||
		strings.Contains(err.Error(), "declined"))
	switch {
	case err == nil:
		// completed normally
	case cancelled:
		m.appendBlock(&block{kind: blockInfo, content: "request cancelled"})
	default:
		m.appendBlock(&block{kind: blockError, content: err.Error()})
	}
	m.refreshView()
	if queued := m.queued; queued != "" {
		m.queued = ""
		return m.submit(queued)
	}
	return nil
}

func (m *Model) handle(e agent.Event) {
	switch e.Kind {
	case "text":
		if e.Text == "" {
			return
		}
		if m.activeAssistant < 0 {
			m.blocks = append(m.blocks, &block{kind: blockAssistant})
			m.activeAssistant = len(m.blocks) - 1
		}
		b := m.blocks[m.activeAssistant]
		b.content += e.Text
		b.invalidate()
		m.refreshView()

	case "thinking":
		m.activity = "thinking"
		m.status = "thinking"

	case "activity":
		m.activity = e.Text
		m.status = e.Text

	case "tool_start", "tool":
		m.closeActiveAssistant()
		m.blocks = append(m.blocks, &block{
			kind:   blockTool,
			label:  e.Tool,
			detail: summarizeInput(e.Tool, e.Input),
			status: statusRunning,
		})
		m.activeTool = len(m.blocks) - 1
		m.status = "running tool"
		m.activity = e.Tool
		m.refreshView()

	case "tool_preview":
		if m.activeTool >= 0 && m.activeTool < len(m.blocks) {
			b := m.blocks[m.activeTool]
			b.content = truncatePreview(e.ToolResult)
			b.invalidate()
			m.refreshView()
		}

	case "tool_done":
		if m.activeTool >= 0 && m.activeTool < len(m.blocks) {
			b := m.blocks[m.activeTool]
			switch {
			case strings.Contains(e.ToolResult, "User declined"):
				b.status = statusDenied
				b.content = ""
			case strings.HasPrefix(e.ToolResult, "ERROR"), strings.HasPrefix(e.ToolResult, "error"):
				b.status = statusError
				b.detail = summarizeInput(b.label, e.Input)
				if b.content == "" {
					b.content = firstLine(e.ToolResult)
				}
			default:
				b.status = statusDone
			}
			b.invalidate()
			m.activeTool = -1
			m.refreshView()
		}

	case "permission":
		m.pendingApproval = e.Approval
		m.approvalTool = e.Tool
		m.approvalDetail = summarizeInput(e.Tool, e.Input)
		m.approvalPreview = ""
		if m.activeTool >= 0 && m.activeTool < len(m.blocks) {
			m.approvalPreview = m.blocks[m.activeTool].content
		}
		m.status = "approval needed"

	case "usage":
		if e.Tokens > 0 {
			m.tokens = e.Tokens
		}
		if e.CostUSD > 0 {
			usage.Add(usage.Entry{
				Time:     time.Now(),
				Provider: m.cfg.CurrentProvider,
				Model:    m.cfg.CurrentModel,
				Input:    e.InputTok,
				Output:   e.OutputTok,
				CostUSD:  e.CostUSD,
			})
		}
		m.refreshSpend()

	case "done":
		m.closeActiveAssistant()
		m.activity = ""
		m.refreshView()
	}
}

// closeActiveAssistant freezes the streaming assistant message so its markdown
// is rendered once, and detaches so later deltas start a fresh block.
func (m *Model) closeActiveAssistant() {
	if m.activeAssistant < 0 || m.activeAssistant >= len(m.blocks) {
		m.activeAssistant = -1
		return
	}
	b := m.blocks[m.activeAssistant]
	if b.kind == blockAssistant && !b.finalized && strings.TrimSpace(b.content) != "" {
		b.finalized = true
		b.invalidate()
	}
	m.activeAssistant = -1
}

func (m *Model) invalidateAnimated() {
	for _, b := range m.blocks {
		if b.status == statusRunning || (b.kind == blockAssistant && !b.finalized) {
			b.invalidate()
		}
	}
}

func (m *Model) appendBlock(b *block) {
	m.closeActiveAssistant()
	m.blocks = append(m.blocks, b)
}

func truncatePreview(preview string) string {
	preview = strings.TrimRight(preview, "\n")
	lines := strings.Split(preview, "\n")
	const maxLines = 40
	if len(lines) > maxLines {
		return strings.Join(lines[:maxLines], "\n") + fmt.Sprintf("\n… %d more line(s)", len(lines)-maxLines)
	}
	return preview
}

func (m *Model) refreshView() {
	w := maxInt(20, m.width-2)
	rendered := make([]string, 0, len(m.blocks))
	prevKind := blockKind(-1)
	for _, b := range m.blocks {
		text := b.render(w, m.spinnerGlyph())
		if len(rendered) > 0 && !(prevKind == blockTool && b.kind == blockTool) {
			rendered = append(rendered, "")
		}
		rendered = append(rendered, text)
		prevKind = b.kind
	}
	m.view.SetContent(strings.Join(rendered, "\n"))
	m.view.GotoBottom()
}

func (m *Model) spinnerGlyph() string {
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}[m.spinner%10]
}

func (m *Model) saveSession() {
	if m.agent == nil {
		return
	}
	history := m.agent.History()
	title := ""
	for _, b := range m.blocks {
		if b.kind == blockUser && strings.TrimSpace(b.content) != "" {
			title = firstLine(strings.TrimSpace(b.content))
			break
		}
	}
	if len(title) > 80 {
		title = title[:77] + "…"
	}
	_ = session.Save(session.Record{
		ID:        m.sessionID,
		Workspace: m.root,
		Title:     title,
		Provider:  m.cfg.CurrentProvider,
		Model:     m.cfg.CurrentModel,
		Mode:      currentMode(m.modeIndex).name,
		History:   history,
	})
}

func (m *Model) showCommands() bool {
	value := m.input.Value()
	return strings.HasPrefix(value, "/") && !strings.ContainsAny(value, " \n")
}

func (m *Model) filteredCommands() []commandItem {
	query := strings.ToLower(m.input.Value())
	items := make([]commandItem, 0, len(commands))
	for _, item := range commands {
		if query == "/" || strings.HasPrefix(item.name, query) {
			items = append(items, item)
		}
	}
	return items
}
