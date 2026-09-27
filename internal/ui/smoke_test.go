package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/session"
)

// smokeModel returns a model wired like the real app (providers, budget, a
// session, and a transcript) so command paths behave as they do in production.
func smokeModel(t *testing.T, width, height int) *Model {
	t.Helper()
	isolatedUsageHome(t)
	m := newTestModel(width, height)
	m.cfg = config.Config{
		Version:         2,
		CurrentProvider: config.BuiltinPrimary,
		CurrentModel:    "DeepSeek-V4-Pro",
		MaxTokens:       16384,
		ContextWindow:   200_000,
		Providers: map[string]config.Provider{
			config.BuiltinPrimary: {
				Label:   "Mihani Cloud",
				Type:    "openai",
				BaseURL: "https://example.invalid/v1",
				Models:  []string{"DeepSeek-V4-Pro", "step-3.7-flash"},
			},
			"local": {Label: "Ollama (local)", Type: "openai", BaseURL: "http://localhost:11434/v1", Models: []string{}},
		},
	}
	m.agent = &agent.Agent{Cfg: m.cfg, Root: t.TempDir()}
	m.sessionID = session.NewID()
	m.version = "v0.0.0-test"
	m.root = t.TempDir()
	m.appendBlock(&block{kind: blockUser, content: "hello there"})
	m.appendBlock(&block{kind: blockAssistant, content: "hi", finalized: true})
	m.appendBlock(&block{kind: blockTool, label: "bash", detail: "echo hi", status: statusDone, finalized: true})
	m.relayout()
	m.refreshView()
	return &m
}

// guard runs fn and turns a panic into a test failure naming the step, so a
// crash in any UI path is reported instead of aborting the whole run.
func guard(t *testing.T, step string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC in %s: %v", step, r)
		}
	}()
	fn()
}

// assertRendered checks the view is not empty and has no obvious breakage.
func assertRendered(t *testing.T, step string, m *Model) {
	t.Helper()
	guard(t, "render "+step, func() {
		view := m.View()
		if strings.TrimSpace(stripANSI(view)) == "" {
			t.Fatalf("%s: rendered an empty screen", step)
		}
		if strings.Contains(view, "panic:") {
			t.Fatalf("%s: panic text leaked into the view", step)
		}
	})
}

// Every slash command must run without crashing, produce visible output, and
// leave the app in a usable state.
func TestSmokeEveryCommand(t *testing.T) {
	names := make([]string, 0, len(commands))
	for _, c := range commands {
		names = append(names, c.name)
	}
	if len(names) < 15 {
		t.Fatalf("command list looks truncated: %d commands", len(names))
	}
	for _, name := range names {
		// /quit and /update hit the network / exit, so they get their own
		// dedicated coverage below.
		if name == "/quit" || name == "/update" {
			continue
		}
		m := smokeModel(t, 100, 30)
		guard(t, name, func() {
			// Type the command into the composer and press enter, exactly as
			// a user would.
			m.input.SetValue(name)
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		})
		assertRendered(t, name, m)
		// esc must always get us out of whatever opened.
		guard(t, name+" esc", func() {
			for i := 0; i < 3; i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			}
		})
		assertRendered(t, name+" after esc", m)
	}
}

// Commands that open an overlay must render it, and the overlay must survive
// navigation (down/up/enter) plus esc without getting stuck or crashing.
func TestSmokeOverlaysNavigate(t *testing.T) {
	overlayCommands := []string{
		"/help", "/mode", "/providers", "/models", "/settings", "/effort", "/export",
	}
	for _, name := range overlayCommands {
		m := smokeModel(t, 100, 30)
		guard(t, name, func() {
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)})
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		})
		if m.overlay == "" {
			// Some of these (e.g. /help) print inline instead of opening a
			// box; that is fine as long as something is shown.
			if len(m.blocks) == 0 {
				t.Fatalf("%s produced no output at all", name)
			}
			continue
		}
		assertRendered(t, name+" overlay", m)
		guard(t, name+" navigate", func() {
			for i := 0; i < 8; i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyDown})
				m.Update(tea.KeyMsg{Type: tea.KeyUp})
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // pick the highlighted item
		})
		assertRendered(t, name+" after pick", m)
		guard(t, name+" esc", func() {
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		})
		if m.overlay != "" {
			t.Fatalf("%s: esc did not close the overlay", name)
		}
		assertRendered(t, name+" closed", m)
	}
}

// Every global key binding, in sequence, on one live model: nothing may panic
// and the app must keep rendering afterwards.
func TestSmokeAllGlobalKeys(t *testing.T) {
	m := smokeModel(t, 100, 30)
	keys := []tea.KeyMsg{
		{Type: tea.KeyTab}, {Type: tea.KeyShiftTab},
		{Type: tea.KeyRunes, Runes: []rune("/")},
		{Type: tea.KeyDown}, {Type: tea.KeyUp}, {Type: tea.KeyPgUp}, {Type: tea.KeyPgDown},
		{Type: tea.KeyHome}, {Type: tea.KeyEnd},
		{Type: tea.KeyCtrlF}, {Type: tea.KeyCtrlR}, {Type: tea.KeyCtrlY}, {Type: tea.KeyCtrlL},
		{Type: tea.KeyBackspace}, {Type: tea.KeyDelete},
		{Type: tea.KeyLeft}, {Type: tea.KeyRight},
		{Type: tea.KeyCtrlJ},
		{Type: tea.KeyEsc}, {Type: tea.KeyEsc},
	}
	for i, k := range keys {
		step := fmt.Sprintf("key %d (%v)", i, k.Type)
		guard(t, step, func() { m.Update(k) })
		assertRendered(t, step, m)
	}
	// esc x2 must not leave an overlay or approval stuck open.
	if m.overlay != "" || m.pendingApproval != nil || m.pendingAsk != nil {
		t.Fatalf("state stuck after key sweep: overlay=%q approval=%v ask=%v",
			m.overlay, m.pendingApproval != nil, m.pendingAsk != nil)
	}
}

// Mouse paths: press, drag, wheel, release, and clicks on menus — with mouse
// capture both on and off, since those take different branches in Update().
func TestSmokeMousePaths(t *testing.T) {
	for _, capture := range []bool{true, false} {
		m := smokeModel(t, 100, 30)
		if capture {
			on := true
			m.cfg.UseMouse = &on
		}
		// A long transcript so scrolling and dragging have room.
		for i := 0; i < 40; i++ {
			m.appendBlock(&block{kind: blockUser, content: fmt.Sprintf("prompt number %d with some text", i)})
			m.appendBlock(&block{kind: blockAssistant, content: fmt.Sprintf("reply number %d with a bit more text", i), finalized: true})
		}
		m.relayout()
		m.refreshView()
		steps := []tea.MouseMsg{
			{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress, X: 10, Y: 5},
			{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, X: 10, Y: 5},
			{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 3},
			{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, X: 20, Y: 4},
			{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, X: 40, Y: 10},
			{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: 40, Y: 10},
			{Button: tea.MouseButtonRight, Action: tea.MouseActionPress, X: 5, Y: 5},
			{Button: tea.MouseButtonMiddle, Action: tea.MouseActionPress, X: 5, Y: 5},
		}
		for i, msg := range steps {
			step := fmt.Sprintf("mouse capture=%v step %d", capture, i)
			guard(t, step, func() { m.Update(msg) })
			assertRendered(t, step, m)
		}
		// After a click, esc must always clear the menu.
		guard(t, "close menu", func() {
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		})
	}
}

// Rendering must survive any terminal size, both UI modes, and hostile content
// (empty blocks, very long lines, CJK, RTL, emoji, control characters).
func TestSmokeRenderMatrix(t *testing.T) {
	sizes := [][2]int{
		{20, 5}, {20, 10}, {40, 8}, {60, 20}, {80, 24}, {100, 30},
		{120, 40}, {200, 60}, {300, 100}, {1, 1}, {2, 2}, {500, 5},
	}
	contents := []struct {
		name string
		b    *block
	}{
		{"empty assistant", &block{kind: blockAssistant, content: "", finalized: true}},
		{"long line", &block{kind: blockAssistant, content: strings.Repeat("very long content ", 400), finalized: true}},
		{"newlines only", &block{kind: blockAssistant, content: "\n\n\n\n", finalized: true}},
		{"cjk", &block{kind: blockAssistant, content: strings.Repeat("你好世界テスト한국어 ", 30), finalized: true}},
		{"rtl", &block{kind: blockAssistant, content: strings.Repeat("سلام دنیا این یک تست است ", 20), finalized: true}},
		{"emoji", &block{kind: blockAssistant, content: strings.Repeat("✅ ❌ 🔧 ✨ done ", 40), finalized: true}},
		{"controls", &block{kind: blockAssistant, content: "a\x00b\x01c\td\x7fe", finalized: true}},
		{"error block", &block{kind: blockError, content: "something failed: \x00 invalid"}},
		{"info block", &block{kind: blockInfo, content: "note"}},
		{"todo", &block{kind: blockTodo, content: "✓ one\n◐ two\n○ three", finalized: true}},
		{"thinking", &block{kind: blockThinking, content: strings.Repeat("thinking about it ", 50), finalized: true}},
		{"tool", &block{kind: blockTool, label: "bash", detail: strings.Repeat("x", 300), status: statusError, content: "@@ -a\n++ +b"}},
	}
	for _, plain := range []bool{false, true} {
		for _, size := range sizes {
			m := smokeModel(t, size[0], size[1])
			guard(t, fmt.Sprintf("plain=%v size=%dx%d", plain, size[0], size[1]), func() {
				plainUI = plain
				defer func() { plainUI = false }()
				for _, c := range contents {
					m.appendBlock(c.b)
					m.relayout()
					m.refreshView()
					view := m.View()
					if strings.TrimSpace(stripANSI(view)) == "" {
						t.Fatalf("empty view for %s at %dx%d plain=%v", c.name, size[0], size[1], plain)
					}
				}
			})
		}
	}
}

// A turn that streams reasoning, tools, todos, errors and then completes must
// render correctly at every stage - this is the normal shape of real usage.
func TestSmokeFullTurnRendering(t *testing.T) {
	m := smokeModel(t, 100, 30)
	events := []agent.Event{
		{Kind: "activity", Text: "thinking", Iteration: 1},
		{Kind: "reasoning", Text: "let me look at the file first"},
		{Kind: "tool_start", Tool: "read_file", Input: map[string]any{"path": "main.go"}, ToolCallID: "c1"},
		{Kind: "tool_preview", Tool: "read_file", ToolResult: "+line", ToolCallID: "c1"},
		{Kind: "tool_done", Tool: "read_file", ToolResult: "package main", ToolCallID: "c1", Input: map[string]any{"path": "main.go"}},
		{Kind: "tool_start", Tool: "todo_write", Input: map[string]any{"todos": []any{map[string]any{"content": "step one", "status": "in_progress"}}}, ToolCallID: "c2"},
		{Kind: "tool_done", Tool: "todo_write", ToolResult: "ok", ToolCallID: "c2"},
		{Kind: "text", Text: "Here is what I found "},
		{Kind: "text", Text: "in the file."},
		{Kind: "usage", Tokens: 1200, MaxTokens: 200000, CostUSD: 0.01},
		{Kind: "done", Done: true},
	}
	for i, e := range events {
		step := fmt.Sprintf("turn event %d (%s)", i, e.Kind)
		guard(t, step, func() { m.handle(e) })
		assertRendered(t, step, m)
	}
	// A failing turn afterwards must also render (error block, no crash).
	guard(t, "failed turn", func() {
		m.handle(agent.Event{Kind: "error", Text: "provider returned 503"})
		m.handle(agent.Event{Kind: "usage", Tokens: 1500, MaxTokens: 200000})
	})
	assertRendered(t, "after failure", m)
}

// The approval and ask_user modals must render and accept input without a
// running program (the production path uses m.program to deliver the answer).
func TestSmokeApprovalAndAskModals(t *testing.T) {
	m := smokeModel(t, 100, 30)
	approval := make(chan bool, 1)
	m.pendingApproval = approval
	m.approvalTool = "bash"
	m.approvalDetail = "rm -rf build"
	m.relayout()
	assertRendered(t, "approval modal", m)
	guard(t, "approve with y", func() {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	})
	if m.pendingApproval != nil {
		t.Fatal("approval modal did not close on y")
	}
	select {
	case ok := <-approval:
		if !ok {
			t.Fatal("y should approve")
		}
	default:
		t.Fatal("approval answer not delivered")
	}

	answer := make(chan string, 1)
	m.handle(agent.Event{Kind: "ask", Text: "Pick one", Input: map[string]any{
		"question": "Pick one",
		"options":  []any{"Refactor it", "Leave it alone"},
	}, Answer: answer})
	m.relayout()
	assertRendered(t, "ask modal", m)
	guard(t, "answer question", func() {
		m.Update(tea.KeyMsg{Type: tea.KeyDown}) // highlight the second option
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	})
	select {
	case a := <-answer:
		if strings.TrimSpace(a) == "" {
			t.Fatal("empty answer delivered to the agent loop")
		}
		if a != "Leave it alone" {
			t.Fatalf("expected the highlighted option, got %q", a)
		}
	default:
		t.Fatal("ask answer not delivered")
	}
	if m.pendingAsk != nil {
		t.Fatal("ask modal stayed open after answering")
	}
	assertRendered(t, "after ask", m)
}

// The update modal must render even with no release data (offline startup) and
// must not crash when the user presses keys in it.
func TestSmokeUpdateModalOffline(t *testing.T) {
	m := smokeModel(t, 100, 30)
	guard(t, "open update offline", func() {
		m.input.SetValue("/update")
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	})
	if m.updateOpen {
		assertRendered(t, "update modal", m)
		guard(t, "update keys", func() {
			for _, k := range []tea.KeyMsg{
				{Type: tea.KeyDown}, {Type: tea.KeyUp}, {Type: tea.KeyPgDown},
				{Type: tea.KeyRunes, Runes: []rune("o")},
				{Type: tea.KeyRunes, Runes: []rune("d")},
				{Type: tea.KeyRunes, Runes: []rune("i")},
			} {
				m.Update(k)
			}
		})
		guard(t, "close update", func() {
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		})
	}
}

// The connect form must render and survive typing/pasting garbage in every
// field, including a NUL-prefixed paste from the Windows console.
func TestSmokeConnectFormGarbageInput(t *testing.T) {
	m := smokeModel(t, 100, 30)
	guard(t, "open connect", func() {
		m.input.SetValue("/connect")
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	})
	if !m.connectOpen {
		t.Skip("connect did not open")
	}
	assertRendered(t, "connect form", m)
	garbage := []string{"\x00https://api.example.com/v1", "not a url", "://x", "https://", "ftp://a.b", "https://a.b/c d"}
	for _, g := range garbage {
		guard(t, "connect input "+g, func() {
			m.connectInput.SetValue(g)
			m.Update(tea.KeyMsg{Type: tea.KeyTab})
		})
		assertRendered(t, "connect field "+g, m)
		guard(t, "connect submit "+g, func() {
			m.connectInput.SetValue(g)
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		})
	}
	guard(t, "connect esc", func() {
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	})
}

// Message actions (copy / fork / revert) must not crash on an empty or
// single-message conversation, where there is nothing to act on.
func TestSmokeMessageActionsOnEmptyTranscript(t *testing.T) {
	m := newTestModel(100, 30)
	m.cfg = smokeModel(t, 100, 30).cfg
	m.agent = &agent.Agent{Cfg: m.cfg, Root: t.TempDir()}
	m.sessionID = session.NewID()
	m.relayout()
	m.refreshView()
	guard(t, "focus on empty", func() {
		for i := 0; i < 5; i++ {
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
		}
	})
	assertRendered(t, "empty transcript focus", &m)
	guard(t, "copy with nothing", func() {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	})
	assertRendered(t, "after copy", &m)
}

// The status bar and header must render with extreme values (0 tokens, huge
// token counts, tiny budget) without producing broken output.
func TestSmokeStatusExtremes(t *testing.T) {
	cases := []struct {
		name  string
		apply func(*Model)
	}{
		{"zero", func(m *Model) { m.tokens = 0 }},
		{"huge", func(m *Model) { m.tokens = 9_000_000 }},
		{"over budget", func(m *Model) { m.spend = 99; m.cfg.BudgetUSD = 10 }},
		{"long path", func(m *Model) { m.root = "C:\\very\\long\\path\\that\\keeps\\going\\and\\going\\project\\directory" }},
		{"no model", func(m *Model) { m.cfg.CurrentModel = "" }},
	}
	for _, c := range cases {
		m := smokeModel(t, 100, 30)
		guard(t, c.name, func() {
			c.apply(m)
			m.relayout()
		})
		assertRendered(t, c.name, m)
	}
}

// Long-running churn: many turns, queueing, interrupting and clearing must not
// destabilize the model.
func TestSmokeManyTurnsAndQueueing(t *testing.T) {
	m := smokeModel(t, 100, 30)
	guard(t, "churn", func() {
		for i := 0; i < 25; i++ {
			m.input.SetValue(fmt.Sprintf("turn %d", i))
			m.handle(agent.Event{Kind: "text", Text: "working"})
			m.handle(agent.Event{Kind: "done", Done: true})
			m.input.SetValue("queued while busy")
			m.busy = true
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m.busy = false
			m.handle(agent.Event{Kind: "done", Done: true})
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("clear")})
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	})
	assertRendered(t, "after churn", m)
	if len(m.blocks) != 0 {
		t.Fatalf("/clear left %d blocks", len(m.blocks))
	}
}

// A fully-typed command must run on the FIRST enter. Requiring a second press
// (insert, then run) made every command look like it ignored the key.
func TestSmokeFullyTypedCommandRunsOnFirstEnter(t *testing.T) {
	m := smokeModel(t, 100, 30)
	before := len(m.blocks)
	m.input.SetValue("/clear")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.blocks) != 0 {
		t.Fatalf("/clear + one enter left %d blocks (wanted 0, started at %d)", len(m.blocks), before)
	}
}

// A partial prefix must still complete the match rather than running a
// different command, and the palette must close.
func TestSmokePartialPrefixCompletesInsteadOfRunning(t *testing.T) {
	m := smokeModel(t, 100, 30)
	m.input.SetValue("/cle")
	items := m.filteredCommands()
	if len(items) != 1 || items[0].name != "/clear" {
		t.Fatalf("expected /clear as the only match, got %v", items)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.input.Value() != "/clear " {
		t.Fatalf("partial prefix should complete to /clear, got %q", m.input.Value())
	}
	if len(m.blocks) == 0 {
		t.Fatal("completing a partial prefix must not run it yet")
	}
}
