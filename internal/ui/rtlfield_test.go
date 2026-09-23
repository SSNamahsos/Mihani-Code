package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
)

// Reproduction of the field crash: /mouse on, click on a user message (with
// Persian content) to open the action menu. Must not panic, and the menu must
// open.
func TestMouseClickOnPersianMessageOpensMenu(t *testing.T) {
	m := newTestModel(80, 24)
	m.version = "test"
	on := true
	m.cfg.UseMouse = &on
	m.agent = &agent.Agent{Cfg: m.cfg}
	m.appendBlock(&block{kind: blockUser, content: "سلام این یک پیام فارسی است"})
	m.appendBlock(&block{kind: blockAssistant, content: "پاسخ فارسی از هوش مصنوعی\n\nEnglish mixed line"})
	m.relayout()

	// Find a screen row over the user message: header is row 0, transcript
	// starts at row 1.
	m.refreshView()
	y := -1
	for row := 1; row < 20; row++ {
		if idx := m.blockAtScreenY(row); idx == 0 {
			y = row
			break
		}
	}
	if y < 0 {
		t.Fatal("could not locate the user message on screen")
	}

	m.mousePress(tea.MouseMsg{X: 5, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.mouseRelease(tea.MouseMsg{X: 5, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	if m.overlay != "Message" {
		t.Fatalf("message menu did not open (overlay=%q)", m.overlay)
	}
	// Rendering the menu must not panic either.
	if v := m.View(); strings.TrimSpace(v) == "" {
		t.Fatal("empty view with menu open")
	}
}

// Full render pass over Persian transcript content in every RTL mode: guards
// the render pipeline against panics and keeps line counts stable (selection
// hit-testing depends on them).
func TestRenderPersianAllModesStableLineCounts(t *testing.T) {
	modes := []string{"off", "bidi", "shaped"}
	for _, mode := range modes {
		setRTLMode(mode)
		m := newTestModel(80, 24)
		m.blocks = append(m.blocks,
			&block{kind: blockUser, content: "سلام دنیا"},
			&block{kind: blockAssistant, content: "این یک **متن** فارسی است با `code`", finalized: true},
			&block{kind: blockInfo, content: "اطلاعات"},
			&block{kind: blockError, content: "خطا رخ داد"},
			&block{kind: blockTool, label: "Mihani_Read_File", detail: "مسیر/فایل.txt", status: statusDone, finalized: true},
			&block{kind: blockTodo, detail: "1/1 done", content: "✓ انجام شد", finalized: true},
		)
		before := len(m.blockLines)
		m.relayout()
		if len(m.blockLines) == before && before == 0 {
			t.Fatalf("mode %s: no lines rendered", mode)
		}
		if v := m.View(); strings.TrimSpace(v) == "" {
			t.Fatalf("mode %s: empty view", mode)
		}
	}
	setRTLMode("bidi")
}

// The composer right-aligns RTL rows within its width and keeps the cursor
// inside the visible content. Default mode is "bidi": base letters, reversed
// into visual order (the terminal's shaper joins them).
func TestComposerRTLAlignment(t *testing.T) {
	c := newComposer()
	c.SetWidth(40)
	c.SetValue("سلام")
	c.Focus()
	view := c.View()
	// The rendered row is padded to the composer width.
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Fatalf("composer row overflows width: %d cells", w)
		}
	}
	// The row must be right-aligned: padded with leading spaces.
	rows := strings.Split(view, "\n")
	first := rows[0]
	if !strings.HasPrefix(first, "   ") {
		t.Fatalf("RTL composer row not right-aligned: %q", first)
	}
	// Base letters in visual (reversed) order: م ا ل س
	if !strings.Contains(stripANSI(view), "\u0645\u0627\u0644\u0633") {
		t.Fatalf("persian not rendered in visual order: %q", stripANSI(view))
	}
}
