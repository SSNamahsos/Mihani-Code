package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"

	"github.com/SSNamahsos/Mihani-Code/internal/rtl"
)

// Typing Persian: Value() keeps the LOGICAL order for the model while the
// rendered view shows SHAPED glyphs in visual (right-to-left) order.
func TestComposerPersianTyping(t *testing.T) {
	setRTLMode("shaped")
	defer setRTLMode("bidi")
	c := newComposer()
	c.SetWidth(60)
	c.Focus()
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("سلام")})
	if got := c.Value(); got != "سلام" {
		t.Fatalf("logical value wrong: %q", got)
	}
	view := stripANSI(c.View())
	// shaped and reversed: م FEE1, lam-alef FEFC, س FEB3
	if !strings.Contains(view, "\uFEE1\uFEFC\uFEB3") {
		t.Fatalf("view does not show shaped visual persian: %U", []rune(view))
	}
	// The cursor rides on the leftmost glyph while appending (the row itself
	// is right-aligned, so it sits after the leading padding).
	if !strings.Contains(view, composerCursorStyle.Render("\uFEE1")) {
		t.Fatal("cursor not rendered at the append position")
	}
}

// The cursor sits ON the leftmost glyph while typing pure Persian (append
// flow: new glyphs appear to its left), and trails at the right edge when
// the cursor is at the logical start.
func TestComposerCursorVisualPosition(t *testing.T) {
	setRTLMode("shaped")
	defer setRTLMode("bidi")
	c := newComposer()
	c.SetWidth(60)
	c.SetValue("سلام") // cursor at logical end
	c.Focus()
	view := stripANSI(c.View())
	if !strings.Contains(view, composerCursorStyle.Render("\uFEE1")) {
		t.Fatalf("append cursor should sit on the leftmost glyph: %q", view)
	}
	c.col = 0 // logical start
	view = stripANSI(c.View())
	if !strings.HasSuffix(view, composerCursorStyle.Render(" ")) {
		t.Fatalf("start cursor should trail at the right edge: %q", view)
	}
}

// Backspace removes the logical last rune and the view stays consistent.
func TestComposerBackspacePersian(t *testing.T) {
	c := newComposer()
	c.SetWidth(60)
	c.SetValue("سلام")
	c.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := c.Value(); got != "سلا" {
		t.Fatalf("backspace wrong: %q", got)
	}
}

// Multi-line editing: insert with newlines splits lines like textarea.
func TestComposerMultilineInsert(t *testing.T) {
	c := newComposer()
	c.SetValue("one two")
	c.col = 3 // after "one"
	c.InsertString("\nsecond\n")
	want := "one\nsecond\n two"
	if got := c.Value(); got != want {
		t.Fatalf("insert = %q, want %q", got, want)
	}
	if c.row != 2 || c.col != 0 {
		t.Fatalf("cursor at %d:%d, want 2:0", c.row, c.col)
	}
}

func TestComposerWordKill(t *testing.T) {
	c := newComposer()
	c.SetValue("alpha beta ")
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	if got := c.Value(); got != "alpha " {
		t.Fatalf("ctrl+w = %q, want %q", got, "alpha ")
	}
}

func TestComposerPlaceholder(t *testing.T) {
	c := newComposer()
	c.Placeholder = "Ask away..."
	c.SetWidth(60)
	if v := c.View(); !strings.Contains(stripANSI(v), "Ask away...") {
		t.Fatalf("placeholder not shown: %q", v)
	}
	c.SetValue("x")
	if v := c.View(); strings.Contains(stripANSI(v), "Ask away...") {
		t.Fatalf("placeholder must hide once text exists: %q", v)
	}
}

// The composer never leaks shaped glyphs into Value(): round-trip safety.
func TestComposerValueStaysLogical(t *testing.T) {
	c := newComposer()
	c.SetWidth(60)
	persian := "این یک جمله فارسی است"
	c.SetValue(persian)
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if got := c.Value(); got != persian+"!" {
		t.Fatalf("value mutated: %q", got)
	}
	if rtl.HasRTL(stripANSI(c.View())) && !rtl.HasRTL(persian) {
		t.Fatal("unexpected")
	}
}
