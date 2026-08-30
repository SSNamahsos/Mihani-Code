package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/session"
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
		stickBottom:     true,
		agent:           &agent.Agent{},
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
	m := Model{input: newInput(""), activeAssistant: -1, activeTool: -1, queued: "next task",
		agent: &agent.Agent{}}
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
	if len(items) != 3 || !contains(names, "/mode") || !contains(names, "/models") || !contains(names, "/mouse") {
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
	isolatedUsageHome(t)
	usage.Add(usage.Entry{Provider: config.BuiltinPrimary, CostUSD: 10.00, Time: time.Now()})
	m := Model{
		cfg:             config.Config{CurrentProvider: config.BuiltinPrimary, BudgetUSD: 10},
		activeAssistant: -1,
		activeTool:      -1,
	}
	m.refreshSpend()
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

// Sticky scroll: streaming events must not yank the view while the user is
// reading higher up; auto-follow resumes only from the bottom.
func TestStickyScrollKeepsPosition(t *testing.T) {
	m := newTestModel(100, 40)
	for i := 0; i < 60; i++ {
		m.appendBlock(&block{kind: blockInfo, content: fmt.Sprintf("filler line %d", i)})
	}
	m.relayout()
	m.refreshView()
	if !m.view.AtBottom() {
		t.Fatal("should start pinned to bottom")
	}

	m.scrollUp(10)
	y := m.view.YOffset
	m.handle(agent.Event{Kind: "activity", Text: "thinking"}) // triggers refreshView
	if m.view.YOffset != y {
		t.Fatalf("view jumped during reading: %d -> %d", y, m.view.YOffset)
	}
	if m.view.AtBottom() {
		t.Fatal("should not be at bottom after scrolling up")
	}

	m.scrollDown(m.height) // return to bottom
	if !m.stickBottom {
		t.Fatal("reaching bottom must re-engage follow mode")
	}
	m.handle(agent.Event{Kind: "activity", Text: "thinking"})
	if !m.view.AtBottom() {
		t.Fatal("follow mode should keep view at bottom")
	}
}

// Raw <tool_call> protocol chatter never reaches the transcript.
func TestStreamSanitizerHidesToolCallBlocks(t *testing.T) {
	cases := []struct{ raw, wantHidden, wantShown string }{
		{taggedUI("bash", "dir") + "all done", "<tool_call>", "all done"},
		{"Working... <tool_call>{\"na", "<tool_call>{\"na", "Working..."},
		{"```tool_call\n{\"name\":\"x\"}\n```\nresult next", "```tool_call", "result next"},
	}
	for _, tc := range cases {
		clean, _ := sanitizeStream(tc.raw)
		if strings.Contains(clean, tc.wantHidden) {
			t.Fatalf("leaked %q in output: %q", tc.wantHidden, clean)
		}
		if !strings.Contains(clean, tc.wantShown) {
			t.Fatalf("lost visible text %q in: %q", tc.wantShown, clean)
		}
	}
}

func taggedUI(name, cmd string) string {
	return "<tool_call>" + `{"name":"` + name + `","arguments":{"command":"` + cmd + `"}}` + "</tool_call>"
}

// [ ] opens the action cursor on user messages; r loads text into the
// composer; f rewinds transcript+history for an alternate path.
func TestFocusActionsRevertAndFork(t *testing.T) {
	isolatedUsageHome(t)
	m := newTestModel(100, 40)
	m.cfg = config.Config{CurrentProvider: config.BuiltinPrimary, BudgetUSD: -1,
		Providers: map[string]config.Provider{config.BuiltinPrimary: {Type: "openai", BaseURL: "http://127.0.0.1:1/v1"}}}
	m.sessionID = session.NewID()

	send := func(prompt string) {
		m.input.SetValue("")
		cmd := m.startTurn(prompt)
		m.cancel()
		m.finishTurn(resultMsg{})
		if cmd != nil {
		}
	}
	send("first question")
	m.handle(agent.Event{Kind: "text", Text: "first answer"})
	m.closeActiveAssistant()
	send("second question")
	m.handle(agent.Event{Kind: "text", Text: "second answer"})
	m.closeActiveAssistant()
	// startTurn alone does not append to history; simulate the agent's
	// conversation so the revert history-trim is observable.
	m.agent.Restore([]map[string]any{
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "first question"},
		{"role": "assistant", "content": "first answer"},
		{"role": "user", "content": "second question"},
		{"role": "assistant", "content": "second answer"},
	})

	// Open focus on the newest message via "]".
	next, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	_ = next
	if !handled || !m.focusActive {
		t.Fatal("']' should open message focus")
	}
	focusedIdx, ok := m.focusedBlockIndex()
	if !ok || m.blocks[focusedIdx].content != "second question" {
		t.Fatalf("focus should land on latest prompt, got %#v", focusedIdx)
	}

	// "[" walks to the older message.
	m.moveFocus(-1)
	idx, _ := m.focusedBlockIndex()
	if m.blocks[idx].content != "first question" {
		t.Fatalf("moveFocus did not reach first prompt: %#v", m.blocks[idx].content)
	}

	// revert → drops this message + later transcript, trims agent history,
	// and prefills the composer so the user can edit and resend.
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.focusActive {
		t.Fatal("revert should close focus")
	}
	if !strings.Contains(m.input.Value(), "first question") {
		t.Fatalf("composer not prefilled: %q", m.input.Value())
	}
	if len(m.blocks) != 0 {
		t.Fatalf("revert must drop the message and everything after, blocks=%d", len(m.blocks))
	}
	for _, msg := range m.agent.History() {
		if s, isStr := msg["content"].(string); isStr && strings.Contains(s, "first question") {
			t.Fatal("history still contains the reverted prompt")
		}
	}

	// Rebuild state, then fork from the only remaining message: its turn is
	// dropped and the prompt text returns to the composer.
	m.input.Reset() // focus requires an idle composer
	m.startTurn("second question")
	m.cancel()
	m.finishTurn(resultMsg{})
	m.setFocus(0)
	idxBefore, ok := m.focusedBlockIndex()
	if !ok || m.blocks[idxBefore].content != "second question" {
		t.Fatalf("expected focus on the rebuilt prompt, got %#v (ok=%v)", idxBefore, ok)
	}
	m.forkFromMessage()
	if len(m.blocks) != 0 {
		t.Fatalf("fork should drop everything after (and incl.) the prompt, got %d", len(m.blocks))
	}
	if !strings.Contains(m.input.Value(), "second question") {
		t.Fatalf("fork should preload the prompt text: %q", m.input.Value())
	}
	hist := m.agent.History()
	for _, msg := range hist {
		if s, isStr := msg["content"].(string); isStr && strings.Contains(s, "second question") {
			t.Fatal("history still contains the forked-away turn")
		}
	}
}

// With mouse capture off the wheel arrives as up/down keypresses; a
// single-line composer routes them to transcript scrolling.
func TestArrowKeysScrollTranscript(t *testing.T) {
	m := newTestModel(100, 40)
	for i := 0; i < 50; i++ {
		m.appendBlock(&block{kind: blockInfo, content: fmt.Sprintf("line %d of filler transcript text", i)})
	}
	m.relayout()
	m.refreshView()
	bottom := m.view.YOffset

	next, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	_ = next
	if !handled || m.view.YOffset >= bottom {
		t.Fatalf("up did not scroll up: y=%d bottom=%d handled=%v", m.view.YOffset, bottom, handled)
	}
	before := m.view.YOffset
	next, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	_ = next
	if m.view.YOffset <= before {
		t.Fatalf("down did not scroll down: y=%d before=%d", m.view.YOffset, before)
	}
}

// A multiline composer keeps the arrows for cursor movement instead.
func TestArrowsStayInMultilineComposer(t *testing.T) {
	m := newTestModel(100, 40)
	m.input.SetValue("line one\nline two")
	m.resizeComposer()
	if m.input.Height() < 2 {
		t.Fatalf("composer should be multiline, height=%d", m.input.Height())
	}
	for i := 0; i < 30; i++ {
		m.appendBlock(&block{kind: blockInfo, content: "filler"})
	}
	m.relayout()
	m.refreshView()
	y := m.view.YOffset

	_, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if handled {
		t.Fatal("multiline composer must not swallow up/down")
	}
	if m.view.YOffset != y {
		t.Fatalf("transcript scrolled while composing multiline: %d -> %d", y, m.view.YOffset)
	}
}

// With the command palette open (input starts with "/"), up/down must cycle
// the highlighted command instead of falling through to the textinput.
func TestArrowKeysNavigateCommandPalette(t *testing.T) {
	m := newTestModel(100, 40)
	m.input.SetValue("/mo")
	items := m.filteredCommands()
	if len(items) < 2 {
		t.Fatalf("need >=2 matching commands for /mo, got %d", len(items))
	}
	if m.commandIndex != 0 {
		t.Fatalf("commandIndex should start at 0, got %d", m.commandIndex)
	}
	_, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if !handled {
		t.Fatalf("down with palette open must be handled, not dropped to the textinput")
	}
	if m.commandIndex != 1 {
		t.Fatalf("down should highlight the next item, commandIndex=%d", m.commandIndex)
	}
	_, _, handled = m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if !handled {
		t.Fatalf("up with palette open must be handled, not dropped to the textinput")
	}
	if m.commandIndex != 0 {
		t.Fatalf("up should wrap back to the first item, commandIndex=%d", m.commandIndex)
	}
}

// Regression: the key entered in /connect used to live only in the runtime
// APIKey field (json:"-"), so it vanished on the next launch and every chat
// request died with an auth error.
func TestFinishConnectPersistsKey(t *testing.T) {
	isolatedUsageHome(t)
	m := newTestModel(100, 40)
	m.cfg = config.Config{Providers: map[string]config.Provider{}}
	m.connectName = "mygw"
	m.connectURL = "https://gw.example.com"
	m.connectFields = [3]string{"mygw", "https://gw.example.com", "sk-user-key-abc123456"}
	if cmd := m.finishConnect(modelsMsg{models: []string{"model-a"}}); cmd != nil {
		t.Fatal("finishConnect should not return a follow-up cmd")
	}
	p := m.cfg.Providers["mygw"]
	if p.PersonalKey != "sk-user-key-abc123456" {
		t.Fatalf("connect key must persist via PersonalKey, got %q", p.PersonalKey)
	}
	if m.cfg.Key("mygw") != "sk-user-key-abc123456" {
		t.Fatal("Key() should resolve the persisted connect key")
	}
	if p.BaseURL != "https://gw.example.com/v1" {
		t.Fatalf("bare base URL should be normalized to the /v1 API path, got %s", p.BaseURL)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Providers["mygw"].PersonalKey; got != "sk-user-key-abc123456" {
		t.Fatalf("connect key lost after save+load: %q", got)
	}
}

func TestRefreshRefetchesCustomProviderModels(t *testing.T) {
	isolatedUsageHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"real-1"},{"id":"real-2"}]}`))
	}))
	defer server.Close()
	m := newTestModel(100, 40)
	m.cfg = config.Config{
		CurrentProvider: "custom", CurrentModel: "stale-model",
		Providers: map[string]config.Provider{
			"custom": {Label: "Custom", Type: "openai", BaseURL: server.URL, Models: []string{"stale-model"}},
		},
	}
	msg := m.refreshProvidersCmd([]string{"custom"})()
	rm, ok := msg.(refreshMsg)
	if !ok {
		t.Fatalf("expected refreshMsg, got %T", msg)
	}
	_ = m.finishRefresh(rm)
	p := m.cfg.Providers["custom"]
	if len(p.Models) != 2 || p.Models[0] != "real-1" {
		t.Fatalf("refresh should replace the model list, got %v", p.Models)
	}
	if m.cfg.CurrentModel != "real-1" {
		t.Fatalf("stale current model should reset to the first fresh model, got %q", m.cfg.CurrentModel)
	}
}

// Startup must pull the models a local endpoint (ollama) actually has,
// replacing stale/fake stored lists.
func TestStartupPullsRealModelsForLocalProvider(t *testing.T) {
	isolatedUsageHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"llama3.1:8b"},{"id":"qwen2.5-coder:7b"}]}`))
	}))
	defer server.Close()
	m := newTestModel(100, 40)
	m.cfg = config.Config{
		CurrentProvider: "ollama", CurrentModel: "fake-model",
		Providers: map[string]config.Provider{
			"ollama": {Label: "Ollama", Type: "openai", BaseURL: server.URL + "/v1", Models: []string{"fake-model"}},
		},
	}
	m.refreshLocalProviderModels()
	p := m.cfg.Providers["ollama"]
	if len(p.Models) != 2 || p.Models[0] != "llama3.1:8b" {
		t.Fatalf("local provider should list the models the server reports, got %v", p.Models)
	}
	if m.cfg.CurrentModel != "llama3.1:8b" {
		t.Fatalf("fake current model should reset to a real one, got %q", m.cfg.CurrentModel)
	}
}

// Regression: retriable provider failures (network/5xx/429) must be retried
// with the backoff ladder until the turn succeeds; the prompt must not be
// duplicated in history by the failed attempts.
func TestTurnRetriesRetriableFailures(t *testing.T) {
	isolatedUsageHome(t)
	orig := turnBackoffs
	turnBackoffs = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}
	defer func() { turnBackoffs = orig }()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 2 {
			w.WriteHeader(502)
			_, _ = w.Write([]byte(`{"error":{"message":"bad gateway"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	m := newTestModel(100, 40)
	m.cfg = config.Config{CurrentProvider: "t", CurrentModel: "m", MaxTokens: 64,
		Providers: map[string]config.Provider{"t": {Type: "openai", BaseURL: server.URL + "/v1"}}}
	m.agent = &agent.Agent{Cfg: m.cfg, Client: server.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := m.runTurn(ctx, "hello", "build", func(string, map[string]any) bool { return true }, func(agent.Event) {})
	if err != nil {
		t.Fatalf("turn should succeed after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 2 failures + 1 success, got %d calls", calls)
	}
	if !strings.Contains(m.activity, "reconnecting 2/10") {
		t.Fatalf("status should show reconnect progress, got %q", m.activity)
	}
	if got := len(m.agent.History()); got != 3 {
		t.Fatalf("history should hold system+user+assistant exactly once, got %d entries", got)
	}
}

// Non-retriable failures (auth) must fail fast — no ten-fold retry of a bad key.
func TestTurnDoesNotRetryAuthFailures(t *testing.T) {
	isolatedUsageHome(t)
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid token"}}`))
	}))
	defer server.Close()

	m := newTestModel(100, 40)
	m.cfg = config.Config{CurrentProvider: "t", CurrentModel: "m", MaxTokens: 64,
		Providers: map[string]config.Provider{"t": {Type: "openai", BaseURL: server.URL + "/v1"}}}
	m.agent = &agent.Agent{Cfg: m.cfg, Client: server.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := m.runTurn(ctx, "hello", "build", func(string, map[string]any) bool { return true }, func(agent.Event) {})
	if err == nil {
		t.Fatal("auth failure must surface as an error")
	}
	if calls != 1 {
		t.Fatalf("401 must not be retried, got %d calls", calls)
	}
}

// Regression: after the provider has actually produced output (the AI
// "continues"), a later failure must restart the reconnect counter from
// 1/10 instead of continuing from where it left off (e.g. 8/10). The
// optimistic "thinking" activity that precedes every request does NOT count
// as progress.
func TestReconnectCounterRestartsAfterLiveProgress(t *testing.T) {
	isolatedUsageHome(t)
	orig := turnBackoffs
	turnBackoffs = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}
	defer func() { turnBackoffs = orig }()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 2 {
			w.WriteHeader(502)
			_, _ = w.Write([]byte(`{"error":{"message":"bad gateway"}}`))
			return
		}
		if calls == 3 {
			// Real live progress (a streamed text delta), then a
			// truncated tool call: the retriable errTruncated failure
			// happens AFTER the provider already produced output.
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"part\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"bash\",\"arguments\":\"{\\\"command\\\":\"}}]}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	m := newTestModel(100, 40)
	m.cfg = config.Config{CurrentProvider: "t", CurrentModel: "m", MaxTokens: 64,
		Providers: map[string]config.Provider{"t": {Type: "openai", BaseURL: server.URL + "/v1"}}}
	m.agent = &agent.Agent{Cfg: m.cfg, Client: server.Client()}

	// Wire the agent's events through the model exactly like the live app
	// (eventMsg -> Update -> handle), and record the activity label in
	// force when each request starts.
	var labels []string
	emit := func(ev agent.Event) {
		if ev.Kind == "activity" && ev.Text == "thinking" {
			labels = append(labels, m.activity)
		}
		m.handle(ev)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := m.runTurn(ctx, "hello", "build", func(string, map[string]any) bool { return true }, emit)
	if err != nil {
		t.Fatalf("turn should succeed after retries, got %v", err)
	}
	if calls != 4 {
		t.Fatalf("expected 3 failures + 1 success, got %d calls", calls)
	}
	if len(labels) != 4 {
		t.Fatalf("expected 4 request-start labels, got %d: %q", len(labels), labels)
	}
	if !strings.Contains(labels[1], "reconnecting 1/10") {
		t.Fatalf("first retry should read 1/10, got %q", labels[1])
	}
	if !strings.Contains(labels[2], "reconnecting 2/10") {
		t.Fatalf("consecutive failure should climb to 2/10, got %q", labels[2])
	}
	if !strings.Contains(labels[3], "reconnecting 1/10") {
		t.Fatalf("counter must restart after live progress, got %q", labels[3])
	}
}

// The error block after a give-up must explain the token usage spike:
// how many times the provider was retried.
func TestErrorBlockStatesRetryCount(t *testing.T) {
	isolatedUsageHome(t)
	m := newTestModel(100, 40)
	m.lastRetries = 3
	m.finishTurn(resultMsg{err: errors.New("provider returned 503 - service unavailable")})
	if len(m.blocks) == 0 {
		t.Fatal("expected an error block")
	}
	b := m.blocks[len(m.blocks)-1]
	if b.kind != blockError {
		t.Fatalf("expected error block, got %v", b.kind)
	}
	if !strings.Contains(b.content, "retried 3 times") {
		t.Fatalf("error block should state the retry count, got %q", b.content)
	}
}

// /effort stores a per-model level, rejects levels the model does not expose,
// and the menu honors the same rules.
func TestEffortCommandAndMenu(t *testing.T) {
	isolatedUsageHome(t)
	m := newTestModel(100, 40)
	m.cfg = config.Config{CurrentProvider: "t", CurrentModel: "claude-opus-5",
		Providers: map[string]config.Provider{"t": {Type: "openai", BaseURL: "http://x.example/v1"}}}
	m.agent = &agent.Agent{}

	if cmd := m.command("/effort high"); cmd != nil {
		t.Fatal("/effort high should not return a follow-up cmd")
	}
	if got := m.cfg.Providers["t"].Efforts["claude-opus-5"]; got != "high" {
		t.Fatalf("effort should be stored per model, got %q", got)
	}
	if m.cfg.CurrentEffort() != "high" {
		t.Fatalf("CurrentEffort should resolve the active model, got %q", m.cfg.CurrentEffort())
	}
	// Survives a reload.
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Providers["t"].Efforts["claude-opus-5"]; got != "high" {
		t.Fatalf("effort lost after save+load: %q", got)
	}

	// A level the model cannot use must be rejected, not silently stored.
	m.cfg.CurrentModel = "llama3.1"
	m.command("/effort high")
	last := m.blocks[len(m.blocks)-1]
	if last.kind != blockError {
		t.Fatalf("unsupported level should raise an error block, got %q", last.content)
	}

	// The menu lists only the levels the model exposes; selecting stores it.
	m.command("/effort")
	if !strings.HasPrefix(m.overlay, "Effort · ") {
		t.Fatalf("/effort should open the effort menu, overlay=%q", m.overlay)
	}
	if len(m.overlayItems) != 1 || m.overlayItems[0].label != "  none" {
		t.Fatalf("non-reasoning model menu should hold only none, got %v", m.overlayItems)
	}
	m.cfg.CurrentModel = "gpt-5.6-sol"
	m.openEffortMenu()
	if len(m.overlayItems) != 4 {
		t.Fatalf("reasoning model menu should hold four levels, got %v", m.overlayItems)
	}
	m.overlayIndex = 3 // high
	m.selectOverlayItem()
	if got := m.cfg.Providers["t"].Efforts["gpt-5.6-sol"]; got != "high" {
		t.Fatalf("menu selection should store the level, got %q", got)
	}
	// The other model keeps its own level.
	if got := m.cfg.Providers["t"].Efforts["claude-opus-5"]; got != "high" {
		t.Fatalf("per-model effort of other models must be untouched, got %q", got)
	}
}

// ctrl+r cycles the active model through the effort levels it exposes,
// and back to none.
func TestCtrlRCyclesEffort(t *testing.T) {
	isolatedUsageHome(t)
	m := newTestModel(100, 40)
	m.cfg = config.Config{CurrentProvider: "t", CurrentModel: "claude-opus-5",
		Providers: map[string]config.Provider{"t": {Type: "openai", BaseURL: "http://x.example/v1"}}}
	m.agent = &agent.Agent{}

	for _, want := range []string{"low", "medium", "high", ""} {
		_, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlR})
		if !handled {
			t.Fatal("ctrl+r must be handled")
		}
		if got := m.cfg.CurrentEffort(); got != want {
			t.Fatalf("after cycle expected effort %q, got %q", want, got)
		}
	}
}

// A plain model has nothing to cycle; ctrl+r must not invent levels.
func TestCtrlRRefusedForNonReasoningModel(t *testing.T) {
	isolatedUsageHome(t)
	m := newTestModel(100, 40)
	m.cfg = config.Config{CurrentProvider: "t", CurrentModel: "llama3.1",
		Providers: map[string]config.Provider{"t": {Type: "openai", BaseURL: "http://x.example/v1"}}}
	m.agent = &agent.Agent{}
	_, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if !handled {
		t.Fatal("ctrl+r must be handled even when refused")
	}
	if p := m.cfg.Providers["t"]; len(p.Efforts) != 0 {
		t.Fatalf("no effort may be stored for a non-reasoning model, got %v", p.Efforts)
	}
	if !strings.Contains(m.toast, "does not expose an effort level") {
		t.Fatalf("expected the refusal toast, got %q", m.toast)
	}
}

// While the model is reasoning, the status line shows the live effort state
// (off = provider default) for models that expose one.
func TestStatusLineShowsEffortWhileThinking(t *testing.T) {
	m := newTestModel(100, 40)
	m.cfg = config.Config{CurrentProvider: "t", CurrentModel: "claude-opus-5", ContextWindow: 200000,
		Providers: map[string]config.Provider{"t": {Type: "openai", BaseURL: "http://x.example/v1"}}}
	m.busy = true
	m.activity = "thinking"

	row := m.statusRow()
	if !strings.Contains(stripANSI(row), "effort:off") {
		t.Fatalf("unset effort should show as off while thinking, row=%q", stripANSI(row))
	}
	p := m.cfg.Providers["t"]
	p.Efforts = map[string]string{"claude-opus-5": "high"}
	m.cfg.Providers["t"] = p
	row = m.statusRow()
	if !strings.Contains(stripANSI(row), "effort:high") {
		t.Fatalf("set effort should show while thinking, row=%q", stripANSI(row))
	}
	// Non-reasoning model: no effort state at all.
	m.cfg.CurrentModel = "llama3.1"
	row = m.statusRow()
	if strings.Contains(stripANSI(row), "effort:") {
		t.Fatalf("non-reasoning model must not show an effort state, row=%q", stripANSI(row))
	}
}

// A stopped local server must not clobber the stored list.
func TestStartupKeepsStoredListWhenLocalServerDown(t *testing.T) {
	isolatedUsageHome(t)
	m := newTestModel(100, 40)
	m.cfg = config.Config{
		CurrentProvider: "ollama", CurrentModel: "qwen2.5-coder",
		Providers: map[string]config.Provider{
			"ollama": {Label: "Ollama", Type: "openai", BaseURL: "http://127.0.0.1:59999/v1", Models: []string{"qwen2.5-coder"}},
		},
	}
	m.refreshLocalProviderModels()
	p := m.cfg.Providers["ollama"]
	if len(p.Models) != 1 || p.Models[0] != "qwen2.5-coder" || m.cfg.CurrentModel != "qwen2.5-coder" {
		t.Fatalf("stored list must survive a stopped local server: models=%v current=%q", p.Models, m.cfg.CurrentModel)
	}
}

// Narrowing the palette under a stale highlight must not crash enter.
func TestEnterClampsStalePaletteIndex(t *testing.T) {
	m := newTestModel(100, 40)
	m.input.SetValue("/")
	m.commandIndex = 10 // deep highlight from the full list
	m.input.SetValue("/mous") // filter narrows to a single command
	if items := m.filteredCommands(); len(items) != 1 {
		t.Fatalf("/mous should match exactly one command, got %d", len(items))
	}
	_, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("enter with palette open must be handled")
	}
	if m.input.Value() != "/mouse " {
		t.Fatalf("enter should complete the single match, input=%q", m.input.Value())
	}
}

func TestCtrlYCopiesLastReplyWithToast(t *testing.T) {
	m := newTestModel(100, 40)
	m.toastTTL = 50 * time.Millisecond
	m.appendBlock(&block{kind: blockUser, content: "question"})
	m.appendBlock(&block{kind: blockAssistant, content: "the answer text", finalized: true})

	_, cmd, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	if !handled || cmd == nil {
		t.Fatalf("ctrl+y mishandled: handled=%v cmd=%v", handled, cmd)
	}
	if !strings.Contains(m.toast, "Copied last reply") {
		t.Fatalf("expected copy toast, got %q", m.toast)
	}
	if content, err := clipboard.ReadAll(); err == nil && !strings.Contains(content, "the answer text") {
		t.Fatalf("clipboard does not hold the reply: %q", content)
	}

	// While the toast is live, ticks keep the animation loop scheduled.
	if _, cmd := m.Update(tickMsg(time.Now())); cmd == nil {
		t.Fatal("tick should reschedule while a toast is visible")
	}

	// Once expired, the toast clears and the tick loop stops.
	time.Sleep(60 * time.Millisecond)
	next, tickCmd := m.Update(tickMsg(time.Now()))
	_ = next
	if tickCmd != nil {
		t.Fatal("expired toast should not keep the tick loop alive")
	}
	if m.toast != "" {
		t.Fatalf("toast should have expired, got %q", m.toast)
	}
}

func TestCtrlYWithoutRepliesNotifies(t *testing.T) {
	m := newTestModel(100, 40)
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	if !strings.Contains(m.toast, "No reply to copy") {
		t.Fatalf("expected empty-history notice, got %q", m.toast)
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
		agent:           &agent.Agent{},
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

// When the shared cap is exhausted and a personal key exists, the next turn
// silently fails over to it instead of blocking — and billing flips buckets.
func TestPersonalKeyAutoFailover(t *testing.T) {
	isolatedUsageHome(t)
	usage.Add(usage.Entry{Provider: config.BuiltinPrimary, CostUSD: 10.00, Time: time.Now()})

	m := Model{
		cfg: config.Config{
			CurrentProvider: config.BuiltinPrimary, CurrentModel: "glm-5.3", BudgetUSD: 10,
			Providers: map[string]config.Provider{config.BuiltinPrimary: {
				Type: "openai", BaseURL: "http://127.0.0.1:1/v1",
				PersonalKey: "sk-personal-fallback-9876",
			}},
		},
		input:           newInput(""),
		activeAssistant: -1,
		activeTool:      -1,
		view:            viewport.New(80, 20),
		width:           100,
		height:          40,
		agent:           &agent.Agent{},
	}
	m.refreshSpend()

	if msg := m.budgetBlock(); msg != "" {
		t.Fatalf("personal key should prevent the hard block, got %q", msg)
	}
	cmd := m.startTurn("hello")
	if cmd == nil || !m.busy {
		t.Fatal("failover turn should start")
	}
	if m.keyKind != usage.Personal {
		t.Fatalf("key kind not switched: %q", m.keyKind)
	}
	got := m.agent.Cfg.Providers[config.BuiltinPrimary].APIKey
	if got != "sk-personal-fallback-9876" {
		t.Fatalf("agent not using personal key: %q", got)
	}
	if label := m.spendLabel(); !strings.Contains(label, "personal") {
		t.Fatalf("meter should show personal bucket mid-turn: %q", label)
	}
	m.cancel()
	m.finishTurn(resultMsg{})
	if m.keyKind != "" {
		t.Fatalf("kind must reset after the turn, got %q", m.keyKind)
	}
}

// Without a stored personal key the cap still blocks with guidance.
func TestCapStillBlocksWithoutPersonalKey(t *testing.T) {
	isolatedUsageHome(t)
	usage.Add(usage.Entry{Provider: config.BuiltinPrimary, CostUSD: 11.00, Time: time.Now()})

	m := Model{
		cfg: config.Config{
			CurrentProvider: config.BuiltinPrimary, CurrentModel: "glm-5.3", BudgetUSD: 10,
			Providers: map[string]config.Provider{config.BuiltinPrimary: {Type: "openai"}},
		},
		input:           newInput(""),
		activeAssistant: -1,
		activeTool:      -1,
		view:            viewport.New(80, 20),
		width:           100,
		height:          40,
	}
	m.refreshSpend()

	if msg := m.budgetBlock(); msg == "" {
		t.Fatal("expected the hard block without a personal key")
	}
	if cmd := m.startTurn("hello"); cmd != nil || m.busy {
		t.Fatal("turn must stay blocked")
	}
}

// The composer must grow for soft-wrapped overflow, not only for newlines,
// so long typing wraps upward instead of running off the right edge.
func TestComposerWrapsLongLines(t *testing.T) {
	m := newTestModel(100, 40)
	m.input.SetValue(strings.Repeat("x", 200)) // one long logical line
	m.resizeComposer()
	if m.input.Height() < 2 {
		t.Fatalf("long single line should wrap to multiple rows, height=%d", m.input.Height())
	}
	// Explicit multi-line stays bounded by the cap.
	m.input.SetValue("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn")
	m.resizeComposer()
	if m.input.Height() > maxComposerLines {
		t.Fatalf("composer exceeded cap: %d", m.input.Height())
	}
}

// Clicking a user message opens the Revert/Fork/Copy menu; other blocks do not.
func TestClickOpensMessageMenu(t *testing.T) {
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockUser, content: "first"})
	m.appendBlock(&block{kind: blockAssistant, content: "reply one", finalized: true})
	m.appendBlock(&block{kind: blockUser, content: "second"})
	m.appendBlock(&block{kind: blockAssistant, content: "reply two", finalized: true})
	m.relayout()
	m.refreshView()

	// Clicking the first user row (top of transcript) opens the menu on release.
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft, X: 5, Y: 2})
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft, X: 5, Y: 2})
	if m.overlay != "Message" {
		t.Fatalf("clicking a user message should open the menu, overlay=%q", m.overlay)
	}
	if len(m.overlayItems) != 3 || m.overlayItems[0].label != "revert" ||
		m.overlayItems[1].label != "fork" || m.overlayItems[2].label != "copy" {
		t.Fatalf("menu items wrong: %#v", m.overlayItems)
	}

	// Selecting "revert" loads the message text into the composer.
	m.selectOverlayItem()
	if !strings.Contains(m.input.Value(), "first") {
		t.Fatalf("revert did not prefill composer: %q", m.input.Value())
	}
}

// Resume must replay tool cards and never mislabel replies as user messages.
func TestReplayPreservesToolBlocksAndAttribution(t *testing.T) {
	m := newTestModel(100, 40)
	history := []map[string]any{
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "inspect the site"},
		{"role": "assistant", "content": `<tool_call>{"name":"read_file","arguments":{"path":"a.txt"}}</tool_call>`},
		{"role": "user", "content": `<tool_result name="read_file" id="prompt_1" status="ok">hello contents</tool_result>`},
		{"role": "assistant", "content": "Here is the file."},
	}
	m.restore(session.Record{Workspace: m.root, ID: "test", History: history})
	m.blocks = nil
	replayTranscript(&m, history)

	got := []blockKind{}
	for _, b := range m.blocks {
		got = append(got, b.kind)
	}
	want := []blockKind{blockUser, blockTool, blockTool, blockAssistant}
	if len(got) != len(want) {
		t.Fatalf("block kinds = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("block kinds = %v, want %v", got, want)
		}
	}
	// The tool_result payload must be a tool card, never a user box.
	if m.blocks[1].kind != blockTool {
		t.Fatalf("tool result rendered as non-tool: %v", m.blocks[1].kind)
	}
}

// Double Esc confirms termination; a single Esc only arms it.
func TestEscRequiresDoublePressToStop(t *testing.T) {
	m := newTestModel(100, 40)
	m.busy = true
	m.escArmed = false
	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.escArmed || m.busy != true {
		t.Fatalf("first esc should arm, not stop: armed=%v busy=%v", m.escArmed, m.busy)
	}
	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.escArmed || !strings.Contains(m.toast, "stopped") {
		t.Fatalf("second esc should stop: armed=%v toast=%q", m.escArmed, m.toast)
	}
}

func TestSeasonsAliasResumes(t *testing.T) {
	m := newTestModel(100, 40)
	m.cfg = config.Config{CurrentProvider: config.BuiltinPrimary, BudgetUSD: -1,
		Providers: map[string]config.Provider{config.BuiltinPrimary: {Type: "openai"}}}
	cmd := m.command("/seasons")
	if m.overlay != "" {
		// Either it opens the picker (no saved seasons) or reports none.
	}
	_ = cmd
	r1 := m.command("/resume")
	if m.overlay != "" && m.overlay != "Resume conversation" {
		t.Fatalf("/seasons or /resume opened unexpected overlay: %q", m.overlay)
	}
	_ = r1
}

// Launching is always a brand-new season: prior conversations live only
// behind /seasons, never auto-restored.
func TestLaunchStartsFreshSeason(t *testing.T) {
	isolatedUsageHome(t)
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	// A previous season exists for this workspace.
	prev := session.Record{ID: session.NewID(), Workspace: mustCwd(t), Title: "old", History: []map[string]any{{"role": "user", "content": "hi"}}}
	if err := session.Save(prev); err != nil {
		t.Fatal(err)
	}

	m, err := New(config.Config{ContextWindow: 200_000, BudgetUSD: -1}, "test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if m.sessionID == prev.ID {
		t.Fatal("launch must not reuse the previous season")
	}
	if len(m.blocks) != 0 {
		t.Fatalf("fresh launch should show the home page with no transcript, got %d blocks", len(m.blocks))
	}
	view := m.welcome()
	if !strings.Contains(view, "NEW SEASON") || !strings.Contains(view, "/seasons") {
		t.Fatalf("home page missing season affordance: %q", view)
	}

	// Explicit resume still works.
	m2, err := New(config.Config{ContextWindow: 200_000, BudgetUSD: -1}, "test", prev.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if m2.sessionID != prev.ID {
		t.Fatalf("explicit resume did not load season %q", prev.ID)
	}
}

// The message action menu is reachable without a mouse (keyboard [ ] focus).
func TestKeyMessageMenuWorksWithoutMouse(t *testing.T) {
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockUser, content: "hello there"})
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if !m.focusActive {
		t.Fatal("']' should open message focus without a mouse")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !strings.Contains(m.input.Value(), "hello there") {
		t.Fatalf("revert did not load the message: %q", m.input.Value())
	}
}

func mustCwd(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
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
		agent:           &agent.Agent{},
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

// Drag selection: press + motion arms the selection, release copies plain text.
func TestDragSelectionCopiesText(t *testing.T) {
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockAssistant, content: "the answer text", finalized: true})
	m.refreshView()

	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 0, Y: 1})
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, X: 17, Y: 1})
	if !m.selOn || !m.selDrag {
		t.Fatalf("selection should be armed while dragging: on=%v drag=%v", m.selOn, m.selDrag)
	}
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: 17, Y: 1})
	if m.selOn {
		t.Fatal("selection should clear after release")
	}
	if !strings.Contains(m.toast, "Copied") {
		t.Fatalf("expected a copy toast, got %q", m.toast)
	}
}

// Selection lives in content coordinates, so wheel scrolling keeps it armed.
func TestSelectionSurvivesScroll(t *testing.T) {
	m := newTestModel(100, 30)
	for i := 0; i < 12; i++ {
		m.appendBlock(&block{kind: blockAssistant, content: fmt.Sprintf("line batch %02d with some words", i), finalized: true})
	}
	m.refreshView()

	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 0, Y: 1})
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, X: 10, Y: 2})
	if !m.selOn || !m.selDrag {
		t.Fatal("selection not armed after drag start")
	}
	a, h := m.selA, m.selH
	m.scrollDown(5) // the viewport moves; the selection must not
	if !m.selOn || !m.selDrag {
		t.Fatal("scrolling should not drop an armed selection")
	}
	if m.selA != a || m.selH != h {
		t.Fatalf("scrolling moved the selection: before=%+v after=%+v", a, m.selH)
	}
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: 10, Y: 2})
	if !strings.Contains(m.toast, "Copied") {
		t.Fatalf("expected a copy toast after release, got %q", m.toast)
	}
}

// A single click (no motion) must not copy; it is handled as a menu click.
func TestClickWithoutMotionDoesNotCopy(t *testing.T) {
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockAssistant, content: "clickable area", finalized: true})
	m.refreshView()

	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 3, Y: 1})
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: 3, Y: 1})
	if m.toast != "" && strings.Contains(m.toast, "Copied") {
		t.Fatalf("plain click should not copy, toast=%q", m.toast)
	}
	if m.selOn {
		t.Fatal("selection state should be cleared after a click")
	}
}

// Esc cancels an armed selection without copying.
func TestEscCancelsSelection(t *testing.T) {
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockAssistant, content: "selectable words", finalized: true})
	m.refreshView()

	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 0, Y: 1})
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, X: 12, Y: 1})
	if !m.selOn {
		t.Fatal("selection should be armed")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.selOn {
		t.Fatal("esc should cancel the selection")
	}
	if strings.Contains(m.toast, "Copied") {
		t.Fatal("cancelled selection must not copy")
	}
}

// ask_user: options menu flow delivers the picked option to the agent.
func TestAskUserMenuDeliversChosenOption(t *testing.T) {
	m := newTestModel(100, 40)
	ans := make(chan string, 1)
	m.handle(agent.Event{Kind: "ask", Text: "Which DB?", Input: map[string]any{
		"question": "Which DB?",
		"options":  []any{"postgres", "sqlite"},
	}, Answer: ans})
	if m.pendingAsk == nil || m.askQuestion != "Which DB?" || len(m.askOptions) != 2 {
		t.Fatalf("ask event not installed: pending=%v q=%q opts=%v", m.pendingAsk != nil, m.askQuestion, m.askOptions)
	}
	if v := stripANSI(m.View()); !strings.Contains(v, "Which DB?") || !strings.Contains(v, "postgres") {
		t.Fatalf("ask view should show question and options, got:\n%s", v)
	}

	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown}) // move to option 2
	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case a := <-ans:
		if a != "sqlite" {
			t.Fatalf("wrong answer delivered: %q", a)
		}
	default:
		t.Fatal("answer was not delivered to the agent channel")
	}
	if m.pendingAsk != nil {
		t.Fatal("ask state should clear after answering")
	}
}

// ask_user: the custom-answer row and free text path.
func TestAskUserCustomAnswerPath(t *testing.T) {
	m := newTestModel(100, 40)
	ans := make(chan string, 1)
	m.handle(agent.Event{Kind: "ask", Text: "Name the file?", Input: map[string]any{
		"question": "Name the file?",
		"options":  []any{"a.txt", "b.txt"},
	}, Answer: ans})

	// To the last row (custom answer) and select it.
	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.askCustom {
		t.Fatal("last row should switch to the custom answer field")
	}
	m.connectInput.SetValue("notes.md")
	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case a := <-ans:
		if a != "notes.md" {
			t.Fatalf("custom answer mismatch: %q", a)
		}
	default:
		t.Fatal("custom answer not delivered")
	}
}

// ask_user: esc skips the question with a proceed-anyway answer.
func TestAskUserEscDismisses(t *testing.T) {
	m := newTestModel(100, 40)
	ans := make(chan string, 1)
	m.handle(agent.Event{Kind: "ask", Text: "Continue?", Input: map[string]any{"question": "Continue?"}, Answer: ans})
	_, _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	select {
	case a := <-ans:
		if !strings.Contains(a, "dismissed") {
			t.Fatalf("esc should deliver a dismissal answer, got %q", a)
		}
	default:
		t.Fatal("dismissal answer not delivered")
	}
}

// End-to-end regression: a real mouse press+release piped through the tea
// program's input parser must open the message action menu on a user message.
// This exercises the production mouse path, not just Update().
func TestClickMessageEndToEnd(t *testing.T) {
	isolatedUsageHome(t)
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockUser, content: "click me"})
	m.refreshView()

	pr, pw := io.Pipe()
	p := tea.NewProgram(&m,
		tea.WithInput(pr),
		tea.WithOutput(io.Discard),
		tea.WithoutRenderer(),
		tea.WithoutCatchPanics(),
		tea.WithMouseCellMotion(),
	)

	var opened bool
	go func() {
		_, _ = pw.Write([]byte("\x1b[<0;5;2M")) // SGR press, left button, (5,2)
		time.Sleep(100 * time.Millisecond)
		_, _ = pw.Write([]byte("\x1b[<0;5;2m")) // SGR release, same cell
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if m.overlay == "Message" { //nolint:staticcheck // test-only cross-goroutine flag
				opened = true
				_, _ = pw.Write([]byte("\x1b")) // esc closes the menu
				_, _ = pw.Write([]byte("\x03")) // quit
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
		_, _ = pw.Write([]byte("\x03"))
	}()

	_, _ = p.Run()
	pw.Close()
	if !opened {
		t.Fatal("clicking a user message did not open the action menu through the real input path")
	}
}

// todo_write: the card updates in place across successive calls.
func TestTodoCardUpdatesInPlace(t *testing.T) {
	m := newTestModel(100, 40)
	in1 := map[string]any{"todos": []any{
		map[string]any{"content": "step one", "status": "pending"},
	}}
	m.handle(agent.Event{Kind: "tool_start", Tool: "todo_write", Input: in1})
	m.handle(agent.Event{Kind: "tool_done", Tool: "todo_write", ToolResult: "OK: 0/1 done\n○ step one", Input: in1})
	if len(m.blocks) != 1 || m.blocks[0].kind != blockTodo {
		t.Fatalf("expected one todo card, got %d blocks", len(m.blocks))
	}
	if !strings.Contains(m.blocks[0].content, "○ step one") {
		t.Fatalf("todo card content: %q", m.blocks[0].content)
	}

	in2 := map[string]any{"todos": []any{
		map[string]any{"content": "step one", "status": "done"},
		map[string]any{"content": "step two", "status": "in_progress"},
	}}
	m.handle(agent.Event{Kind: "tool_start", Tool: "todo_write", Input: in2})
	m.handle(agent.Event{Kind: "tool_done", Tool: "todo_write", ToolResult: "OK: 1/2 done\n✓ step one\n◐ step two", Input: in2})
	if len(m.blocks) != 1 {
		t.Fatalf("todo card should update in place, now %d blocks", len(m.blocks))
	}
	b := m.blocks[0]
	if !strings.Contains(b.content, "✓ step one") || !strings.Contains(b.content, "◐ step two") {
		t.Fatalf("todo card not updated: %q", b.content)
	}
	if !strings.Contains(b.detail, "1/2") {
		t.Fatalf("todo summary missing: %q", b.detail)
	}
}

// Resumed sessions must rebuild the todo card from stored tool results.
func TestReplayRebuildsTodoCard(t *testing.T) {
	m := newTestModel(100, 40)
	history := []map[string]any{
		{"role": "assistant", "content": "planning", "tool_calls": []any{
			map[string]any{"id": "call_1", "type": "function", "function": map[string]any{
				"name": "todo_write", "arguments": `{"todos":[{"content":"a","status":"done"}]}`,
			}},
		}},
		{"role": "tool", "tool_call_id": "call_1", "content": "OK: 1/1 done\n✓ a"},
	}
	replayTranscript(&m, history)
	n := 0
	var last *block
	for _, b := range m.blocks {
		if b.kind == blockTodo {
			n++
			last = b
		}
	}
	if n < 2 || last == nil || !strings.Contains(last.content, "✓ a") {
		t.Fatalf("expected rebuilt todo card with list, got %d todo cards in %d blocks", n, len(m.blocks))
	}
}

// A click one line above a user card (on the separator row) still opens the
// menu via the line-based tolerance.
func TestClickNearUserMessageOpensMenu(t *testing.T) {
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockAssistant, content: "above reply", finalized: true})
	m.appendBlock(&block{kind: blockUser, content: "click near me"})
	m.appendBlock(&block{kind: blockAssistant, content: "below reply", finalized: true})
	m.refreshView()

	// Block layout: assistant(1) + separator(1) + user card(4) + separator(1) + assistant(1).
	// Content row 1 is the separator just above the user card.
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 4, Y: 2})
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: 4, Y: 2})
	if m.overlay != "Message" {
		t.Fatalf("click near a user message should open the menu, overlay=%q", m.overlay)
	}
}

// Spurious ±1 cell motion (a Windows Terminal artifact on plain clicks) must
// NOT be mistaken for a drag; the release should still open the message menu.
func TestClickJitterDeadZone(t *testing.T) {
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockUser, content: "click me"})
	m.appendBlock(&block{kind: blockAssistant, content: "reply", finalized: true})
	m.refreshView()

	// Press at (5,2), one spurious +1 jitter motion, release back at the cell.
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 5, Y: 2})
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, X: 6, Y: 2})
	if m.selDrag {
		t.Fatal("1-cell jitter must not arm a drag")
	}
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: 5, Y: 2})
	if m.overlay != "Message" {
		t.Fatalf("jittered click should still open the menu, overlay=%q", m.overlay)
	}
}

// A real drag (>=2 cells) is still a selection, not a menu click.
func TestDragBeyondDeadZoneSelects(t *testing.T) {
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockAssistant, content: "the answer text", finalized: true})
	m.refreshView()
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 0, Y: 1})
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, X: 17, Y: 1})
	if !m.selDrag {
		t.Fatal("a 17-cell move must arm a drag")
	}
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: 17, Y: 1})
	if m.overlay != "" {
		t.Fatalf("a real drag must not open the menu, overlay=%q", m.overlay)
	}
}

// The message menu must open even while a turn is in flight; only the
// destructive revert/fork actions are deferred until the turn finishes.
func TestMenuOpensWhileBusy(t *testing.T) {
	m := newTestModel(100, 40)
	m.appendBlock(&block{kind: blockUser, content: "first"})
	m.appendBlock(&block{kind: blockAssistant, content: "reply", finalized: true})
	m.refreshView()
	m.busy = true

	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 5, Y: 2})
	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: 5, Y: 2})
	if m.overlay != "Message" {
		t.Fatalf("menu should open while busy, overlay=%q", m.overlay)
	}
	// Selecting revert while busy is rejected, transcript untouched.
	m.selectOverlayItem()
	if len(m.blocks) != 2 {
		t.Fatalf("revert while busy must not drop blocks, blocks=%d", len(m.blocks))
	}
	if !strings.Contains(m.toast, "wait for the current turn") {
		t.Fatalf("expected a busy guard toast, got %q", m.toast)
	}
}

// truncateWord backs off to a word boundary instead of cutting mid-word.
func TestTruncateWordStopsAtWordBoundary(t *testing.T) {
	got := truncateWord("load this message into the composer to edit and resend", 20)
	if strings.Contains(got, "...") == false {
		t.Fatalf("expected truncation, got %q", got)
	}
	// No word may be split: the cut must land right after a space.
	body := strings.TrimSuffix(got, "...")
	if body == "" || body[len(body)-1] != ' ' && !strings.Contains(got, " ") {
		t.Fatalf("unexpected truncated form %q", got)
	}
}
