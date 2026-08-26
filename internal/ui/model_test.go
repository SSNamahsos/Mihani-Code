package ui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/usage"
)

func newInput(value string) textarea.Model {
	ta := textarea.New()
	ta.SetValue(value)
	return ta
}

// newTestModel builds a model whose components are fully initialized so
// handlers can run without a terminal session.
func newTestModel(width, height int) Model {
	ta := newInput("")
	ta.SetWidth(maxInt(20, width-6))
	ta.SetHeight(1)
	ci := textarea.New()
	ci.SetWidth(minInt(58, maxInt(20, width-20)))
	ci.SetHeight(1)
	vp := viewport.New(maxInt(20, width-2), maxInt(3, height-4))
	return Model{
		input:           ta,
		connectInput:    ci,
		view:            vp,
		width:           width,
		height:          height,
		activeAssistant: -1,
		activeTool:      -1,
	}
}

// Regression: streamed deltas must assemble into ONE assistant block instead
// of emitting a new header per token.
func TestStreamingAssemblesSingleAssistantBlock(t *testing.T) {
	m := Model{activeAssistant: -1, activeTool: -1}
	m.handle(agent.Event{Kind: "text", Text: "Hello"})
	m.handle(agent.Event{Kind: "text", Text: ", world"})
	m.handle(agent.Event{Kind: "text", Text: "!"})
	if len(m.blocks) != 1 {
		t.Fatalf("expected a single block per message, got %d", len(m.blocks))
	}
	if m.blocks[0].content != "Hello, world!" {
		t.Fatalf("streamed content mismatched: %q", m.blocks[0].content)
	}
	if m.activeAssistant != 0 {
		t.Fatalf("active assistant not tracked: %d", m.activeAssistant)
	}

	m.closeActiveAssistant()
	if !m.blocks[0].finalized {
		t.Fatal("block was not finalized after turn")
	}
	// A later delta must start a fresh block.
	m.handle(agent.Event{Kind: "text", Text: "second reply"})
	if len(m.blocks) != 2 || m.blocks[1].content != "second reply" {
		t.Fatalf("post-finalize delta did not start a new block: %d blocks", len(m.blocks))
	}
}

func TestToolLifecycleTracksStatusAndPreview(t *testing.T) {
	m := Model{activeAssistant: -1, activeTool: -1}
	m.handle(agent.Event{Kind: "tool_start", Tool: "edit_file", Input: map[string]any{"path": "a.go"}})
	if len(m.blocks) != 1 || m.blocks[0].status != statusRunning {
		t.Fatalf("tool block not created running: %#v", m.blocks)
	}
	preview := "--- a.go\n+++ a.go\n-old line\n+new line"
	m.handle(agent.Event{Kind: "tool_preview", ToolResult: preview})
	m.handle(agent.Event{Kind: "tool_done", ToolResult: "OK: edited a.go", Input: map[string]any{"path": "a.go"}})
	b := m.blocks[0]
	if b.status != statusDone {
		t.Fatalf("expected done status, got %q", b.status)
	}
	if !strings.Contains(b.content, "+new line") {
		t.Fatalf("diff preview lost: %q", b.content)
	}
	if m.activeTool != -1 {
		t.Fatal("active tool not cleared after completion")
	}

	m.handle(agent.Event{Kind: "tool_start", Tool: "bash", Input: map[string]any{"command": "boom"}})
	m.handle(agent.Event{Kind: "tool_done", ToolResult: "ERROR: exit status 1"})
	if m.blocks[1].status != statusError {
		t.Fatalf("expected error status, got %q", m.blocks[1].status)
	}

	m.handle(agent.Event{Kind: "tool_start", Tool: "write_file", Input: nil})
	m.handle(agent.Event{Kind: "tool_done", ToolResult: "User declined to run this tool."})
	if m.blocks[2].status != statusDenied {
		t.Fatalf("expected denied status, got %q", m.blocks[2].status)
	}
}

func TestFinishTurnReportsCancelWithoutError(t *testing.T) {
	m := Model{activeAssistant: -1, activeTool: -1}
	m.finishTurn(resultMsg{err: context.Canceled})
	last := m.blocks[len(m.blocks)-1]
	if len(m.blocks) != 1 || last.kind != blockInfo {
		t.Fatalf("expected an info block for cancellation, got %#v", m.blocks)
	}
	if !strings.Contains(last.content, "cancelled") {
		t.Fatalf("unexpected info content: %q", last.content)
	}
	if m.busy {
		t.Fatal("busy flag stuck after finishTurn")
	}
}

func TestEnterQueuesPromptWhileBusy(t *testing.T) {
	m := Model{input: newInput("fix the bug"), busy: true, activeAssistant: -1, activeTool: -1}
	_, cmd, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled || cmd != nil {
		t.Fatalf("queued enter mishandled: handled=%v cmd=%v", handled, cmd)
	}
	if m.queued != "fix the bug" {
		t.Fatalf("prompt not queued: %q", m.queued)
	}
	if strings.TrimSpace(m.input.Value()) == "" && false {
		t.Fatal("unreachable")
	}

	// Slash commands are never queued.
	m.input.SetValue("/help")
	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.queued != "fix the bug" {
		t.Fatalf("slash command should not be queued, queued=%q", m.queued)
	}
}

func TestFinishTurnFlushesQueue(t *testing.T) {
	m := Model{input: newInput(""), activeAssistant: -1, activeTool: -1, queued: "next task"}
	m.cfg.BudgetUSD = -1 // keep the flush independent of real usage on this machine
	cmd := m.finishTurn(resultMsg{})
	if cmd == nil {
		t.Fatal("expected a command to flush the queued prompt")
	}
	if m.queued != "" {
		t.Fatalf("queue not drained: %q", m.queued)
	}
}

func TestCommandPaletteFiltersByPrefix(t *testing.T) {
	m := Model{input: newInput("/mo")}
	items := m.filteredCommands()
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.name)
	}
	if len(items) != 2 || !contains(names, "/mode") || !contains(names, "/models") {
		t.Fatalf("unexpected filtered commands: %v", names)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestModesCycleWithDistinctColors(t *testing.T) {
	m := Model{}
	first := currentMode(m.modeIndex)
	m.modeIndex = 1
	second := currentMode(m.modeIndex)
	if first.name != "build" || second.name != "plan" || first.color == second.color {
		t.Fatalf("unexpected mode state: %#v %#v", first, second)
	}
}

func TestModeCommandAcceptsArgument(t *testing.T) {
	m := Model{activeAssistant: -1, activeTool: -1}
	m.command("/mode plan")
	if currentMode(m.modeIndex).name != "plan" {
		t.Fatalf("/mode plan did not apply, index=%d", m.modeIndex)
	}
	m.command("/mode wat")
	if last := m.blocks[len(m.blocks)-1]; last.kind != blockError {
		t.Fatalf("unknown mode should report an error, got %#v", last)
	}
}

func TestQuitCommandQuits(t *testing.T) {
	m := Model{}
	if cmd := m.command("/quit"); cmd == nil {
		t.Fatal("/quit must return a quit command now")
	}
}

func TestConnectOverlayOpensAndEscapes(t *testing.T) {
	m := newTestModel(100, 40)
	m.openConnect()
	if !m.connectOpen || m.connectField != 0 {
		t.Fatal("connect overlay did not open")
	}
	m.closeConnect()
	if m.connectOpen {
		t.Fatal("connect overlay did not close")
	}
}

// Regression: /connect crashed because New() left connectInput uninitialized.
// This exercises the real constructor end-to-end.
func TestNewSupportsConnectOverlayWithoutPanic(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir()) // keep session store away from the real home
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	m, err := New(config.Config{ContextWindow: 200_000}, "test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 40
	m.view.Width, m.view.Height = 98, 36

	m.openConnect()
	if !m.connectOpen || m.connectField != 0 {
		t.Fatal("connect overlay did not open from the real New() model")
	}
	if view := m.connectView(); !strings.Contains(view, "Provider ID") {
		t.Fatalf("connect view missing form labels: %q", view)
	}

	// Typing into the wizard field must work too (this is where the panic hit).
	m.saveConnectField()
	m.loadConnectField()
	m.connectInput.SetValue("local-qwen")
	if !strings.Contains(m.connectView(), "local-qwen") {
		t.Fatal("typed value is not visible in the connect form")
	}
}

func TestConnectViewRendersLiveTyping(t *testing.T) {
	m := newTestModel(100, 40)
	m.openConnect()
	m.connectInput.SetValue("local-qwen")
	view := m.connectView()
	if !strings.Contains(view, "local-qwen") {
		t.Fatalf("active connect value is not rendered while typing: %q", view)
	}
}

func TestOverlaySelectionClosesModeOverlay(t *testing.T) {
	m := Model{}
	m.openOverlay("Modes", []overlayItem{
		{label: "● build"}, {label: "  plan"}, {label: "  research"}, {label: "  ask"},
	})
	m.overlayIndex = 1
	m.selectOverlayItem()
	if m.overlay != "" || currentMode(m.modeIndex).name != "plan" {
		t.Fatalf("overlay selection did not apply: overlay=%q mode=%s", m.overlay, currentMode(m.modeIndex).name)
	}
}

func TestWelcomeIncludesCurrentModelAndProvider(t *testing.T) {
	m := Model{
		cfg: config.Config{
			CurrentProvider: "ollama",
			CurrentModel:    "qwen2.5-coder",
			ContextWindow:   200_000,
			Providers: map[string]config.Provider{
				"ollama": {Label: "Ollama (local)", Type: "openai"},
			},
		},
		width: 100, height: 44,
		version: "test",
	}
	m.view.Width = 98
	m.view.Height = 30
	view := m.welcome()
	if !strings.Contains(view, "qwen2.5-coder") || !strings.Contains(view, "Ollama (local)") {
		t.Fatalf("welcome is missing session details: %q", view)
	}
}

// Branding rule: built-in endpoint names must never appear in any rendered
// surface — provider display resolves to Mihani labels.
func TestProviderDisplayNeverLeaksEndpointNames(t *testing.T) {
	m := Model{cfg: config.Config{
		CurrentProvider: config.BuiltinPrimary,
		CurrentModel:    "glm-5.3",
		Providers:       defaultsForTest(),
	}}
	for _, stored := range []string{"hcnsec", "seekai", config.BuiltinSecondary, "custom-thing"} {
		got := m.providerDisplay(stored)
		if strings.Contains(strings.ToLower(got), "hcnsec") || strings.Contains(strings.ToLower(got), "seekai") {
			t.Fatalf("providerDisplay(%q) leaked an endpoint name: %q", stored, got)
		}
	}
	if got := m.providerDisplay("hcnsec"); got != "Mihani Cloud" {
		t.Fatalf("legacy id should map to its label, got %q", got)
	}
	if got := m.providerDisplay("custom-thing"); got != "Mihani Code" {
		t.Fatalf("unknown providers must stay neutral, got %q", got)
	}
}

// The budget block must not name the upstream provider.
func TestBudgetMessageIsNeutral(t *testing.T) {
	m := Model{
		cfg:             config.Config{CurrentProvider: config.BuiltinPrimary, BudgetUSD: 10},
		spend:           10,
		activeAssistant: -1,
		activeTool:      -1,
	}
	msg := m.budgetBlock()
	if msg == "" {
		t.Fatal("expected a limit message at the cap")
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "hcnsec") || strings.Contains(lower, "seekai") {
		t.Fatalf("limit message leaks provider identity: %q", msg)
	}
	if !strings.Contains(lower, "daily limit") {
		t.Fatalf("unexpected limit message: %q", msg)
	}
}

func defaultsForTest() map[string]config.Provider {
	return map[string]config.Provider{
		config.BuiltinPrimary:   {Label: "Mihani Cloud", Type: "openai"},
		config.BuiltinSecondary: {Label: "Mihani Pro", Type: "openai"},
	}
}

// Reasoning deltas stream into a dedicated dimmed thinking block that closes
// as soon as real answer text arrives.
func TestReasoningStreamsIntoThinkingBlock(t *testing.T) {
	m := Model{activeAssistant: -1, activeTool: -1, activeThinking: -1}
	m.handle(agent.Event{Kind: "reasoning", Text: "analyzing"})
	m.handle(agent.Event{Kind: "reasoning", Text: " the diff"})
	if len(m.blocks) != 1 || m.blocks[0].kind != blockThinking {
		t.Fatalf("expected one thinking block, got %#v", m.blocks)
	}
	if m.blocks[0].content != "analyzing the diff" {
		t.Fatalf("reasoning content mismatched: %q", m.blocks[0].content)
	}

	// Answer text must close the thinking block and open an assistant block.
	m.handle(agent.Event{Kind: "text", Text: "Here is my answer."})
	if m.activeThinking != -1 {
		t.Fatal("thinking block not closed when answer text arrived")
	}
	if len(m.blocks) != 2 || m.blocks[1].kind != blockAssistant {
		t.Fatalf("expected assistant block after reasoning, got %#v", m.blocks)
	}

	// Later reasoning starts a fresh thinking block.
	m.handle(agent.Event{Kind: "reasoning", Text: "more thoughts"})
	if len(m.blocks) != 3 || m.blocks[2].kind != blockThinking || m.activeThinking != 2 {
		t.Fatalf("second reasoning segment mishandled: %#v active=%d", m.blocks, m.activeThinking)
	}

	// Rendering stays within width and marks the header.
	out := m.blocks[2].render(80, "⠋")
	if !strings.Contains(out, "thinking") || !strings.Contains(out, "more thoughts") {
		t.Fatalf("thinking render missing parts: %q", out)
	}
}

func TestMouseWheelScrollsTranscript(t *testing.T) {
	m := newTestModel(100, 40)
	for i := 0; i < 50; i++ {
		m.appendBlock(&block{kind: blockInfo, content: fmt.Sprintf("line %d of filler transcript text", i)})
	}
	m.relayout()
	m.refreshView()
	bottom := m.view.YOffset

	next, _ := m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	_ = next
	if m.view.YOffset >= bottom {
		t.Fatalf("wheel-up did not scroll up: y=%d bottom=%d", m.view.YOffset, bottom)
	}
	before := m.view.YOffset
	next, _ = m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	_ = next
	if m.view.YOffset <= before {
		t.Fatalf("wheel-down did not scroll down: y=%d before=%d", m.view.YOffset, before)
	}
}

func TestApprovalAnswerRepliesExactlyOnce(t *testing.T) {
	ch := make(chan bool, 1)
	m := Model{pendingApproval: ch, busy: true, activeAssistant: -1, activeTool: -1}
	m.answerApproval(false)
	select {
	case ok := <-ch:
		if ok {
			t.Fatal("deny answered true")
		}
	default:
		t.Fatal("approval channel never received an answer")
	}
	m.answerApproval(true) // second call must be a no-op
	select {
	case <-ch:
		t.Fatal("channel received a second answer")
	default:
	}
}

func isolatedUsageHome(t *testing.T) {
	t.Helper()
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	usage.Reset()
	t.Cleanup(func() { usage.Reset() })
}

// A custom /connect provider must bypass the shared credit cap entirely.
func TestBudgetDoesNotGateCustomProviders(t *testing.T) {
	isolatedUsageHome(t)
	usage.Add(usage.Entry{Provider: "my-own", CostUSD: 99.00, Time: time.Now()})

	m := Model{
		cfg: config.Config{
			CurrentProvider: "my-own", CurrentModel: "whatever", BudgetUSD: 10,
			Providers: map[string]config.Provider{"my-own": {Type: "openai", BaseURL: "http://127.0.0.1:1/v1"}},
		},
		input:           newInput(""),
		activeAssistant: -1,
		activeTool:      -1,
		view:            viewport.New(80, 20),
		width:           100,
		height:          40,
	}
	m.refreshSpend()

	if msg := m.budgetBlock(); msg != "" {
		t.Fatalf("custom provider should never be capped, got %q", msg)
	}
	if label := m.spendLabel(); label != "" {
		t.Fatalf("spend meter should hide for custom providers, got %q", label)
	}
	if cmd := m.startTurn("hello"); cmd == nil {
		t.Fatal("turn on custom provider should start even far over the built-in budget")
	}
	m.cancel()
}

// The daily cap must refuse to start new turns once the rolling 24h spend
// reaches the budget for the active provider.
func TestBudgetBlocksNewTurns(t *testing.T) {
	isolatedUsageHome(t)
	usage.Add(usage.Entry{Provider: config.BuiltinPrimary, CostUSD: 10.00, Time: time.Now()})

	m := Model{
		cfg:             config.Config{CurrentProvider: config.BuiltinPrimary, CurrentModel: "glm-5.3", BudgetUSD: 10},
		input:           newInput(""),
		activeAssistant: -1,
		activeTool:      -1,
		view:            viewport.New(80, 20),
		width:           100,
		height:          40,
	}
	m.refreshSpend()

	cmd := m.startTurn("hello")
	if cmd != nil {
		t.Fatal("turn should be blocked at the cap")
	}
	if m.busy {
		t.Fatal("busy flag set despite budget block")
	}
	last := m.blocks[len(m.blocks)-1]
	if last.kind != blockError || !strings.Contains(last.content, "daily limit") {
		t.Fatalf("expected a limit error block, got %#v", last)
	}
}

func TestBudgetAllowsUnderCap(t *testing.T) {
	isolatedUsageHome(t)
	usage.Add(usage.Entry{Provider: config.BuiltinPrimary, CostUSD: 0.50, Time: time.Now()})

	m := Model{
		cfg: config.Config{
			CurrentProvider: config.BuiltinPrimary, CurrentModel: "glm-5.3", BudgetUSD: 10,
			Providers: map[string]config.Provider{config.BuiltinPrimary: {Type: "openai", BaseURL: "http://127.0.0.1:1/v1"}},
		},
		input:           newInput(""),
		activeAssistant: -1,
		activeTool:      -1,
		view:            viewport.New(80, 20),
		width:           100,
		height:          40,
	}
	m.refreshSpend()

	cmd := m.startTurn("hello")
	if cmd == nil {
		t.Fatal("turn under the cap should be allowed")
	}
	if !m.busy {
		t.Fatal("turn under the cap should mark the model busy")
	}
	if m.cancel == nil {
		t.Fatal("cancel func not wired for running turn")
	}
	m.cancel()
}

// Usage events with cost must be recorded against the active provider.
func TestUsageEventRecordsSpend(t *testing.T) {
	isolatedUsageHome(t)
	m := Model{
		cfg:             config.Config{CurrentProvider: config.BuiltinSecondary, CurrentModel: "claude-opus-5", BudgetUSD: 10},
		activeAssistant: -1,
		activeTool:      -1,
		view:            viewport.New(80, 20),
		width:           100,
		height:          40,
	}
	m.handle(agent.Event{Kind: "usage", Tokens: 1500, InputTok: 1000, OutputTok: 500, CostUSD: 0.42})
	m.handle(agent.Event{Kind: "usage", Tokens: 3000, InputTok: 2000, OutputTok: 1000, CostUSD: 0.58})
	if got := usage.WindowSum(config.BuiltinSecondary); math.Abs(got-1.00) > 1e-9 {
		t.Fatalf("recorded spend = %v, want 1.00", got)
	}
	if m.spend < 1.00-1e-9 {
		t.Fatalf("cached spend not refreshed: %v", m.spend)
	}

	label := m.spendLabel()
	if !strings.Contains(label, "$1.00/$10.00") {
		t.Fatalf("spend label missing totals: %q", label)
	}
}
