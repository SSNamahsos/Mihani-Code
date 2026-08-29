package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/secrets"
	"github.com/SSNamahsos/Mihani-Code/internal/session"
	"github.com/SSNamahsos/Mihani-Code/internal/usage"
)

// uiToolCall is the replay-side form of a parsed prompt-tool request.
type uiToolCall struct {
	Name  string
	Input map[string]any
}

var (
	uiTaggedCallRe = regexp.MustCompile(`(?s)<tool_call>\s*(\{.*?\})\s*</tool_call>`)
	uiFencedCallRe = regexp.MustCompile("(?s)```(?:tool_call|json)?\\s*\n(\\{.*?\\})\n```")
)

func uiToolCalls(text string) ([]uiToolCall, error) {
	blocks := func(re *regexp.Regexp) []string {
		out := []string{}
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 {
				out = append(out, m[1])
			}
		}
		return out
	}(uiTaggedCallRe)
	if len(blocks) == 0 {
		blocks = func(re *regexp.Regexp) []string {
			out := []string{}
			for _, m := range re.FindAllStringSubmatch(text, -1) {
				if len(m) > 1 {
					out = append(out, m[1])
				}
			}
			return out
		}(uiFencedCallRe)
	}
	var out []uiToolCall
	for _, b := range blocks {
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if json.Unmarshal([]byte(b), &p) != nil || p.Name == "" {
			continue
		}
		out = append(out, uiToolCall{Name: p.Name, Input: p.Arguments})
	}
	return out, nil
}

func uiStripToolCalls(text string) string {
	text = uiTaggedCallRe.ReplaceAllString(text, "")
	return uiFencedCallRe.ReplaceAllString(text, "")
}

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
	{name: "/resume", description: "Resume a previous conversation"},
	{name: "/seasons", description: "Switch between conversations in this folder"},
	{name: "/copy", description: "Copy Mihani's last reply to the clipboard"},
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
	activeAssistant int             // index of the assistant block receiving stream deltas
	activeTool      int             // index of the running tool block
	activeThinking  int             // index of the live reasoning block
	activeRaw       strings.Builder // raw stream buffer for the active assistant

	focusActive bool // keyboard action cursor over user messages
	focusPos    int  // position within the user-message index list

	stickBottom bool // auto-follow transcript; released when the user scrolls up

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
	msgMenuIndex  int // transcript index of the user message opened from the action menu

	connectOpen   bool
	connecting    bool
	connectField  int
	connectInput  textarea.Model
	connectFields [3]string
	connectName   string
	connectURL    string
	connectError  string

	keyEditOpen   bool
	keyEditTarget string

	sessionID    string
	modeIndex    int
	commandIndex int
	quitting     bool

	spend         float64 // rolling 24h shared-key spend for the current provider
	personalSpend float64 // rolling 24h personal-key spend
	keyKind       string  // "" / usage.Embedded shared, usage.Personal when failed over
	toast         string  // transient notification shown in the status bar
	toastUntil    time.Time
	toastTTL      time.Duration // test hook; zero means defaultToastTTL

	escArmed   bool // double-press confirmation for interrupting a request
	escArmedAt time.Time

	blockLines []int // rendered row count per transcript block (for hit-testing)
}

const defaultToastTTL = 2500 * time.Millisecond

func (m *Model) notify(msg string) {
	m.toast = msg
	m.toastUntil = time.Now().Add(m.toastTTLor())
}

func (m *Model) toastTTLor() time.Duration {
	if m.toastTTL > 0 {
		return m.toastTTL
	}
	return defaultToastTTL
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
		activeThinking:  -1,
		stickBottom:     true,
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
	// Personal keys are user secrets: scrub them from every tool result too.
	for _, id := range []string{config.BuiltinPrimary, config.BuiltinSecondary} {
		secrets.Register(cfg.Providers[id].PersonalKey)
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
		role := fmt.Sprint(msg["role"])
		if role == "tool" {
			m.replayToolResult(msg)
			continue
		}
		if role == "user" {
			m.replayUser(msg)
			continue
		}
		if role == "assistant" {
			m.replayAssistant(msg)
		}
	}
}

func (m *Model) replayUser(msg map[string]any) {
	content := msg["content"]
	switch c := content.(type) {
	case string:
		// Prompt-based tool results are fed back as a user message containing
		// <tool_result> blocks to a chat-only endpoint — render those as tool
		// cards, never as user prose.
		if strings.Contains(c, "<tool_result") {
			m.replayPromptToolResults(c)
			return
		}
		if strings.TrimSpace(c) != "" {
			m.blocks = append(m.blocks, &block{kind: blockUser, content: c})
		}
	case []map[string]any:
		m.replayAnthropicToolResults(c)
	case []any:
		m.replayAnthropicToolResults(mapsOf(c))
	}
}

func mapsOf(items []any) []map[string]any {
	var out []map[string]any
	for _, item := range items {
		if mm, ok := item.(map[string]any); ok {
			out = append(out, mm)
		}
	}
	return out
}

// replayAnthropicToolResults turns tool_result content blocks into tool cards.
func (m *Model) replayAnthropicToolResults(parts []map[string]any) {
	for _, part := range parts {
		if fmt.Sprint(part["type"]) != "tool_result" {
			continue
		}
		m.appendToolDoneCard(fmt.Sprint(part["tool_use_id"]), "", fmt.Sprint(part["content"]))
	}
}

func (m *Model) replayPromptToolResults(text string) {
	for _, res := range promptToolResultBlocks(text) {
		m.appendToolDoneCard(res.id, res.name, res.output)
	}
}

type promptResult struct{ id, name, output, status string }

func promptToolResultBlocks(text string) []promptResult {
	var out []promptResult
	re := regexp.MustCompile(`(?s)<tool_result name="([^"]*)" id="([^"]*)" status="([^"]*)"[^>]*>\s*(.*?)\s*</tool_result>`)
	for _, match := range re.FindAllStringSubmatch(text, -1) {
		out = append(out, promptResult{name: match[1], id: match[2], status: match[3], output: match[4]})
	}
	return out
}

// replayToolResult handles "\"role\": \"tool\"" entries (OpenAI native).
func (m *Model) replayToolResult(msg map[string]any) {
	m.appendToolDoneCard(fmt.Sprint(msg["tool_call_id"]), "", fmt.Sprint(msg["content"]))
}

// appendToolDoneCard adds a settled tool card; a best-effort name is derived
// from the stored content when the id does not carry one.
func (m *Model) appendToolDoneCard(id, name, output string) {
	if name == "" {
		name = id
	}
	b := &block{kind: blockTool, label: name, status: statusDone, finalized: true}
	if strings.HasPrefix(output, "ERROR") || strings.Contains(output, "declined") {
		b.status = statusError
	}
	b.content = truncatePreview(output)
	m.blocks = append(m.blocks, b)
}

func (m *Model) replayAssistant(msg map[string]any) {
	m.replayOpenAIToolCalls(msg)
	switch content := msg["content"].(type) {
	case string:
		if strings.TrimSpace(content) != "" {
			// Prompt-based replies embed <tool_call> blocks inline.
			calls, _ := uiToolCalls(content)
			clean := uiStripToolCalls(content)
			if strings.TrimSpace(clean) != "" {
				m.blocks = append(m.blocks, &block{kind: blockAssistant, content: clean, finalized: true})
			}
			for _, call := range calls {
				m.blocks = append(m.blocks, &block{
					kind:      blockTool,
					label:     call.Name,
					detail:    summarizeInput(call.Name, call.Input),
					status:    statusDone,
					finalized: true,
				})
			}
		}
	case []map[string]any:
		m.replayAnthropicAssistant(content)
	case []any:
		m.replayAnthropicAssistant(mapsOf(content))
	}
}

func (m *Model) replayAnthropicAssistant(parts []map[string]any) {
	var texts []string
	for _, part := range parts {
		switch fmt.Sprint(part["type"]) {
		case "text":
			texts = append(texts, fmt.Sprint(part["text"]))
		case "tool_use":
			name := fmt.Sprint(part["name"])
			input := map[string]any{}
			if raw, ok := part["input"].(map[string]any); ok {
				input = raw
			}
			m.blocks = append(m.blocks, &block{
				kind:      blockTool,
				label:     name,
				detail:    summarizeInput(name, input),
				status:    statusDone,
				finalized: true,
			})
		}
	}
	if joined := strings.TrimSpace(strings.Join(texts, "\n\n")); joined != "" {
		m.blocks = append(m.blocks, &block{kind: blockAssistant, content: joined, finalized: true})
	}
}

// replayOpenAIToolCalls turns an assistant message containing native
// tool_calls into tool cards (used when content carries tool_calls directly).
func (m *Model) replayOpenAIToolCalls(msg map[string]any) {
	raw, ok := msg["tool_calls"]
	if !ok {
		return
	}
	for _, entry := range mapsOfToList(raw) {
		fn, _ := entry["function"].(map[string]any)
		name := fmt.Sprint(fn["name"])
		input := map[string]any{}
		if args, ok := fn["arguments"].(string); ok {
			_ = json.Unmarshal([]byte(args), &input)
		}
		m.blocks = append(m.blocks, &block{
			kind:      blockTool,
			label:     name,
			detail:    summarizeInput(name, input),
			status:    statusDone,
			finalized: true,
		})
	}
}

func mapsOfToList(raw any) []map[string]any {
	var out []map[string]any
	switch items := raw.(type) {
	case []map[string]any:
		out = items
	case []any:
		out = mapsOf(items)
	}
	return out
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

// Run starts the interactive TUI. Mouse capture is deliberately OFF so native
// text selection works everywhere; terminal emulators translate the physical
// wheel into up/down keypresses in alternate-screen mode, which scroll here.
func Run(cfg config.Config, version, resumeID, initialPrompt string) error {
	m, err := New(cfg, version, resumeID, initialPrompt)
	if err != nil {
		return err
	}
	// Mouse reporting lets you click a user message for its action menu
	// (revert/fork/copy). Text selection still works with Shift+drag (or
	// Ctrl+Shift+drag) in Windows Terminal, VS Code, iTerm2, kitty, etc.
	p := tea.NewProgram(&m, tea.WithAltScreen(), tea.WithMouseCellMotion())
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

	case tea.MouseMsg:
		if m.overlay != "" || m.connectOpen || m.keyEditOpen || m.pendingApproval != nil {
			break
		}
		if x.Action == tea.MouseActionPress && x.Button == tea.MouseButtonLeft {
			if b := m.blockAtScreenY(x.Y); b >= 0 && b < len(m.blocks) && m.blocks[b].kind == blockUser {
				m.openMessageMenu(b)
				return m, nil
			}
		}

	case eventMsg:
		m.handle(agent.Event(x))

	case tickMsg:
		if !m.toastUntil.IsZero() && !time.Now().Before(m.toastUntil) {
			m.toast = ""
			m.toastUntil = time.Time{}
		}
		if m.busy || m.pendingApproval != nil || m.toast != "" {
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
		if m.keyEditOpen {
			m.closeKeyEditor()
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

	case m.keyEditOpen:
		return m, m.updateKeyEditor(x), true

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

	case m.focusActive:
		switch key {
		case "esc", "enter", "q":
			m.clearFocus()
		case "[", "k":
			m.moveFocus(-1)
		case "]", "j":
			m.moveFocus(1)
		case "y", "Y":
			if idx, ok := m.focusedBlockIndex(); ok {
				if err := clipboard.WriteAll(m.blocks[idx].content); err != nil {
					m.notify("Clipboard unavailable: " + err.Error())
				} else {
					m.notify("Message copied to clipboard")
				}
			}
		case "r", "R":
			m.revertToComposer()
		case "f", "F":
			m.forkFromMessage()
		}
		return m, tick(), true

	case key == "esc":
		if m.busy {
			// Double-press to confirm: first Esc arms, second Esc terminates.
			if m.escArmed && time.Since(m.escArmedAt) < 1500*time.Millisecond {
				m.interrupt()
				m.escArmed = false
				m.notify("Request stopped")
			} else {
				m.escArmed = true
				m.escArmedAt = time.Now()
				m.notify("press esc again to terminate")
			}
			return m, tick(), true
		}
		m.escArmed = false
		if m.showCommands() {
			m.input.Reset()
			m.commandIndex = 0
		} else if m.input.Value() != "" {
			m.input.Reset()
		}
		return m, nil, true

	case key == "pgup":
		m.scrollUp(m.view.Height / 2)
		return m, nil, true
	case key == "pgdown":
		m.scrollDown(m.view.Height / 2)
		return m, nil, true

	// With mouse capture off, terminals deliver the physical wheel as plain
	// up/down keypresses. In a single-line composer those keys are free, so
	// they scroll the transcript; a grown (multiline) composer keeps them for
	// cursor movement instead.
	case key == "up", key == "down":
		if m.showCommands() || m.input.Height() > 1 {
			return m, nil, false // palette navigation / multiline cursor keys
		}
		if key == "up" {
			m.scrollUp(3)
		} else {
			m.scrollDown(3)
		}
		return m, nil, true

	case key == "ctrl+y":
		if cmd := m.copyLastReply(); cmd != nil {
			return m, cmd, true
		}
		return m, nil, true

	case key == "ctrl+j", key == "alt+enter", key == "shift+enter":
		m.input.InsertString("\n")
		m.resizeComposer()
		return m, nil, true

	case key == "[", key == "]":
		if !m.showCommands() && strings.TrimSpace(m.input.Value()) == "" {
			m.startFocus()
			return m, tick(), true
		}
		return m, nil, false

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

// keyKindOf resolves the credential variant currently in force for billing
// attribution. Non-built-in providers are personal by definition.
func (m *Model) keyKindOf() string {
	if m.keyKind == usage.Personal {
		return usage.Personal
	}
	if !m.cfg.IsBuiltinProvider(m.cfg.CurrentProvider) {
		return usage.Personal
	}
	return usage.Embedded
}

// refreshSpend re-reads the rolling 24h totals for the active provider.
func (m *Model) refreshSpend() {
	m.spend = usage.WindowSumFor(m.cfg.CurrentProvider, usage.Embedded)
	m.personalSpend = usage.WindowSumFor(m.cfg.CurrentProvider, usage.Personal)
}

// budgetBlock returns a user-facing message when the daily cap is exhausted,
// or "" when the turn may proceed. Only built-in Mihani providers are capped;
// when the user stored a personal key for that endpoint, Mihani fails over to
// it instead of blocking.
func (m *Model) budgetBlock() string {
	budget := m.cfg.BudgetEnforced(m.cfg.CurrentProvider)
	if budget <= 0 {
		return ""
	}
	if m.embeddedSpend() < budget {
		return ""
	}
	if p := m.cfg.Providers[m.cfg.CurrentProvider]; p.PersonalKey != "" {
		return "" // failover handles it
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
			"Budget resets in %s — add your own key via /settings → Personal API key, switch provider with /providers, use a free model, or raise budget_usd in settings.",
		m.spend, budget, when)
}

// embeddedSpend is shared-key spend only — personal keys have their own quota.
func (m *Model) embeddedSpend() float64 {
	return usage.WindowSumFor(m.cfg.CurrentProvider, usage.Embedded)
}

// pickKeyKind decides which credential variant serves this turn and prepares
// an effective config carrying it. Returns "" for the shared embedded key.
func (m *Model) pickKeyKind() string {
	budget := m.cfg.BudgetEnforced(m.cfg.CurrentProvider)
	if budget <= 0 || m.embeddedSpend() < budget {
		m.keyKind = ""
		return ""
	}
	p := m.cfg.Providers[m.cfg.CurrentProvider]
	if p.PersonalKey == "" {
		return "" // caller blocks via budgetBlock
	}
	m.keyKind = usage.Personal
	return usage.Personal
}

// effectiveCfg clones the config with the personal key promoted to APIKey so
// every request this turn authenticates as the user.
func (m *Model) effectiveCfg(kind string) config.Config {
	if kind != usage.Personal {
		return m.cfg
	}
	eff := m.cfg
	eff.Providers = make(map[string]config.Provider, len(m.cfg.Providers))
	for k, v := range m.cfg.Providers {
		eff.Providers[k] = v
	}
	p := eff.Providers[m.cfg.CurrentProvider]
	p.APIKey = p.PersonalKey
	eff.Providers[m.cfg.CurrentProvider] = p
	return eff
}

// scrollUp pins the transcript to its position: new output will no longer
// auto-follow until the user scrolls back to the bottom.
func (m *Model) scrollUp(n int) {
	m.view.LineUp(n)
	m.stickBottom = false
}

func (m *Model) scrollDown(n int) {
	m.view.LineDown(n)
	if m.view.AtBottom() {
		m.stickBottom = true
	}
}

// copyLastReply puts the most recent assistant message on the clipboard and
// raises a confirmation toast. Returns a tea.Cmd so the toast renders.
func (m *Model) copyLastReply() tea.Cmd {
	for i := len(m.blocks) - 1; i >= 0; i-- {
		b := m.blocks[i]
		if b.kind != blockAssistant || strings.TrimSpace(b.content) == "" {
			continue
		}
		if err := clipboard.WriteAll(b.content); err != nil {
			m.notify("Clipboard unavailable: " + err.Error())
			return tick()
		}
		lines := strings.Count(strings.TrimSpace(b.content), "\n") + 1
		m.notify(fmt.Sprintf("Copied last reply (%d lines) to clipboard", lines))
		return tick()
	}
	m.notify("No reply to copy yet")
	return tick()
}

// userMsgIndexes lists transcript positions of user messages, oldest first.
func (m *Model) userMsgIndexes() []int {
	var out []int
	for i, b := range m.blocks {
		if b.kind == blockUser {
			out = append(out, i)
		}
	}
	return out
}

func (m *Model) startFocus() {
	idx := m.userMsgIndexes()
	if len(idx) == 0 {
		m.notify("No messages to inspect yet")
		return
	}
	m.setFocus(idx[len(idx)-1])
}

func (m *Model) setFocus(blockIdx int) {
	m.clearFocusFlags()
	m.focusActive = true
	m.focusPos = blockIdx
	m.blocks[blockIdx].focused = true
	m.blocks[blockIdx].invalidate()
	m.refreshView()
}

func (m *Model) clearFocusFlags() {
	for _, b := range m.blocks {
		if b.focused {
			b.focused = false
			b.invalidate()
		}
	}
}

func (m *Model) clearFocus() {
	m.clearFocusFlags()
	m.focusActive = false
	m.focusPos = 0
	m.refreshView()
}

func (m *Model) moveFocus(delta int) {
	users := m.userMsgIndexes()
	if len(users) == 0 {
		return
	}
	pos := 0
	for i, bi := range users {
		if bi == m.focusPos {
			pos = i
			break
		}
	}
	next := pos + delta
	if next < 0 {
		next = 0
	}
	if next >= len(users) {
		next = len(users) - 1
	}
	m.setFocus(users[next])
}

func (m *Model) focusedBlockIndex() (int, bool) {
	if !m.focusActive || m.focusPos >= len(m.blocks) || !m.blocks[m.focusPos].focused {
		return 0, false
	}
	return m.focusPos, true
}

// applyMessageAction executes Revert/Fork/Copy from the message menu.
// action: 0 = revert, 1 = fork, 2 = copy.
func (m *Model) applyMessageAction(blockIdx, action int) {
	m.closeOverlay()
	m.clearFocus()
	if blockIdx < 0 || blockIdx >= len(m.blocks) || m.blocks[blockIdx].kind != blockUser {
		return
	}
	text := m.blocks[blockIdx].content
	switch action {
	case 0: // revert: prefill composer to edit + resend
		m.input.Reset()
		m.input.SetValue(text)
		m.resizeComposer()
		m.notify("Loaded into composer — edit and press enter to resend")
	case 1: // fork: rewind transcript + history, prefill composer
		m.doFork(blockIdx)
	case 2: // copy
		if err := clipboard.WriteAll(text); err != nil {
			m.notify("Clipboard unavailable: " + err.Error())
		} else {
			m.notify("Message copied to clipboard")
		}
	}
	m.relayout()
	m.refreshView()
}

// revertToComposer loads the focused prompt back into the editor for a resend.
func (m *Model) revertToComposer() {
	idx, ok := m.focusedBlockIndex()
	if !ok {
		return
	}
	m.applyMessageAction(idx, 0)
}

// forkFromMessage rewinds the conversation to just before the focused prompt:
// later transcript and history are dropped, and the text returns to the
// composer so an alternate path can be taken.
func (m *Model) forkFromMessage() {
	idx, ok := m.focusedBlockIndex()
	m.clearFocus()
	if !ok {
		return
	}
	m.doFork(idx)
}

func (m *Model) doFork(idx int) {
	text := m.blocks[idx].content

	ordinal := 0 // which user message (1-based) this block represents
	for i := 0; i <= idx && i < len(m.blocks); i++ {
		if m.blocks[i].kind == blockUser {
			ordinal++
		}
	}
	seen := 0
	cut := len(m.agent.History())
	for h, msg := range m.agent.History() {
		if fmt.Sprint(msg["role"]) != "user" {
			continue
		}
		if _, isText := msg["content"].(string); !isText {
			continue // tool_result batches are not real prompts
		}
		seen++
		if seen == ordinal {
			cut = h
			break
		}
	}

	history := m.agent.History()
	if cut <= len(history) {
		m.agent.Restore(history[:cut])
	}
	m.blocks = append([]*block{}, m.blocks[:idx]...)
	m.activeAssistant, m.activeTool, m.activeThinking = -1, -1, -1
	m.activeRaw.Reset()
	m.stickBottom = true

	m.input.Reset()
	m.input.SetValue(text)
	m.resizeComposer()
	m.saveSession()
	m.relayout()
	m.notify("Forked — earlier context kept, edit and resend")
}

func (m *Model) submit(s string) tea.Cmd {
	if strings.HasPrefix(s, "/") {
		return m.command(s)
	}
	return m.startTurn(s)
}

func (m *Model) startTurn(prompt string) tea.Cmd {
	kind := m.pickKeyKind()
	if blocked := m.budgetBlock(); blocked != "" {
		m.appendBlock(&block{kind: blockError, content: blocked})
		m.refreshView()
		return nil
	}
	if kind == usage.Personal {
		m.notify("Shared limit reached — using your personal API key")
	}
	m.closeActiveAssistant()
	m.blocks = append(m.blocks, &block{kind: blockUser, content: prompt})
	m.stickBottom = true
	m.busy = true
	m.approveAll = false
	m.status = "working"
	m.activity = "thinking"
	m.refreshView()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	modeName := currentMode(m.modeIndex).name
	cfg := m.effectiveCfg(kind)
	a := m.agent
	a.Cfg = cfg
	a.Root = m.root
	return tea.Sequence(tick(), func() tea.Msg {
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
	m.keyKind = "" // next turn re-evaluates the credential variant
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
	case "reasoning", "thinking":
		if e.Text == "" {
			return
		}
		if m.activeThinking < 0 {
			m.blocks = append(m.blocks, &block{kind: blockThinking})
			m.activeThinking = len(m.blocks) - 1
		}
		b := m.blocks[m.activeThinking]
		b.content += e.Text
		b.invalidate()
		m.refreshView()

	case "text":
		if e.Text == "" {
			return
		}
		m.closeActiveThinking()
		if m.activeAssistant < 0 {
			m.blocks = append(m.blocks, &block{kind: blockAssistant})
			m.activeAssistant = len(m.blocks) - 1
			m.activeRaw.Reset()
		}
		m.activeRaw.WriteString(e.Text)
		clean, _ := sanitizeStream(m.activeRaw.String())
		b := m.blocks[m.activeAssistant]
		b.content = clean
		b.invalidate()
		m.refreshView()

	case "activity":
		m.activity = e.Text
		m.status = e.Text

	case "tool_start", "tool":
		m.closeActiveAssistant()
		m.closeActiveThinking()
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
				KeyKind:  m.keyKindOf(),
			})
		}
		m.refreshSpend()

	case "done":
		m.closeActiveAssistant()
		m.closeActiveThinking()
		m.activity = ""
		m.refreshView()
	}
}

// closeActiveThinking freezes the live reasoning block.
func (m *Model) closeActiveThinking() {
	if m.activeThinking < 0 || m.activeThinking >= len(m.blocks) {
		m.activeThinking = -1
		return
	}
	b := m.blocks[m.activeThinking]
	if b.kind == blockThinking && !b.finalized {
		b.finalized = true
		b.invalidate()
	}
	m.activeThinking = -1
}

// closeActiveAssistant freezes the streaming assistant message so its markdown
// is rendered once, and detaches so later deltas start a fresh block.
func (m *Model) closeActiveAssistant() {
	if m.activeAssistant < 0 || m.activeAssistant >= len(m.blocks) {
		m.activeAssistant = -1
		return
	}
	b := m.blocks[m.activeAssistant]
	if b.kind == blockAssistant && !b.finalized {
		clean, _ := sanitizeStream(m.activeRaw.String())
		b.content = clean
		if strings.TrimSpace(b.content) != "" {
			b.finalized = true
		}
		b.invalidate()
	}
	m.activeRaw.Reset()
	m.activeAssistant = -1
	m.closeActiveThinking()
}

func (m *Model) invalidateAnimated() {
	for _, b := range m.blocks {
		if b.status == statusRunning ||
			(b.kind == blockAssistant && !b.finalized) ||
			(b.kind == blockThinking && !b.finalized) {
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
	m.blockLines = m.blockLines[:0]
	prevKind := blockKind(-1)
	for _, b := range m.blocks {
		text := b.render(w, m.spinnerGlyph())
		if len(rendered) > 0 && !(prevKind == blockTool && b.kind == blockTool) {
			// every block is separated by exactly one blank line
			rendered = append(rendered, "")
			m.blockLines = append(m.blockLines, 1) // the separator row itself
		}
		m.blockLines = append(m.blockLines, strings.Count(text, "\n")+1)
		rendered = append(rendered, text)
		prevKind = b.kind
	}
	m.view.SetContent(strings.Join(rendered, "\n"))
	if m.stickBottom {
		m.view.GotoBottom()
	}
}

// blockAtScreenY maps a terminal row inside the transcript to a block index,
// accounting for the header offset and the viewport's scroll position.
func (m *Model) blockAtScreenY(y int) int {
	contentRow := y - 1 // 1 header row
	if contentRow < 0 {
		return -1
	}
	target := m.view.YOffset + contentRow
	cum := 0
	for i, lines := range m.blockLines {
		if target < cum+lines {
			return i
		}
		cum += lines
	}
	return -1
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
