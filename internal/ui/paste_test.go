package ui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Bracketed paste delivers the whole clipboard as one KeyMsg. It must land
// in the composer as ONE multiline message — never split, never auto-sent.
func TestBracketedPasteKeepsWholeBlock(t *testing.T) {
	m := newTestModel(80, 24)
	pasted := "line one\nline two\nline three with \"quotes\" and {json}"
	msg := tea.KeyMsg{Type: tea.KeyRunes, Paste: true, Runes: []rune(pasted)}
	_, _ = m.Update(msg)
	if got := m.input.Value(); got != pasted {
		t.Fatalf("paste lost content:\n got %q\nwant %q", got, pasted)
	}
	if len(m.blocks) != 0 {
		t.Fatalf("paste must not submit; blocks=%d", len(m.blocks))
	}
	if m.busy {
		t.Fatal("paste must not start a turn")
	}
}

// Raw-paste fallback: a terminal without bracketed paste streams the
// clipboard as individual keys, and each newline arrives as Enter. On a
// multiline composer a fast rune burst right before Enter is a paste, not a
// deliberate submit.
func TestRawPasteBurstEnterKeepsNewline(t *testing.T) {
	m := newTestModel(80, 24)
	m.input.SetValue("first line\nsecond")
	m.burstRunes = 40
	m.lastKeyAt = time.Now()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.input.Value(); got != "first line\nsecond\n" {
		t.Fatalf("burst enter should insert a newline, got %q", got)
	}
	if len(m.blocks) != 0 || m.busy {
		t.Fatalf("burst enter must not submit")
	}
}

// A deliberate Enter (no recent burst) on a multiline composer still sends.
func TestDeliberateEnterStillSendsMultiline(t *testing.T) {
	m := newTestModel(80, 24)
	m.input.SetValue("first line\nsecond")
	m.burstRunes = 40
	m.lastKeyAt = time.Now().Add(-time.Second) // burst too old
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("deliberate enter should submit (start a turn)")
	}
}

// Regression (field log 2026-08-29): once the message menu opened, every
// mouse event was dropped, so the user could no longer drag-select text
// until pressing esc. A press outside the box must close the menu AND arm a
// selection; a press on an item row selects that item.
func TestOverlayClickOutsideClosesAndArmsSelection(t *testing.T) {
	m := newTestModel(80, 50)
	for i := 0; i < 20; i++ {
		m.appendBlock(&block{kind: blockUser, content: fmt.Sprintf("prompt %d", i)})
		m.appendBlock(&block{kind: blockAssistant, content: fmt.Sprintf("reply %d", i), finalized: true})
	}
	m.relayout()
	m.refreshView()
	// Pin the transcript to the top of the screen so content-row math is
	// deterministic (production keeps whatever scroll offset the user had).
	m.view.SetYOffset(0)
	m.openOverlay("Modes", []overlayItem{{label: "a"}, {label: "b"}})
	boxTop, _, _ := m.overlayGeometry()
	boxBottom := boxTop + 9 // 2 items: border/pad/title/blank/items/blank/hint/pad/border
	if len(m.renderedLines) <= boxBottom+1 {
		t.Fatalf("test setup: transcript (%d rows) too short for box bottom %d", len(m.renderedLines), boxBottom)
	}
	// Press just below the box, still inside the transcript.
	m.mouseOverlayClick(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: boxBottom + 1})
	if m.overlay != "" {
		t.Fatal("press outside the box should close the overlay")
	}
	if !m.selOn {
		t.Fatal("press outside should arm a drag selection at the press point")
	}
}

func TestOverlayClickOnItemRowSelects(t *testing.T) {
	m := newTestModel(80, 50)
	m.modeIndex = 0
	items := []overlayItem{{label: "build"}, {label: "plan"}, {label: "research"}, {label: "ask"}}
	m.openOverlay("Modes", items)
	_, itemTop, itemRows := m.overlayGeometry()
	if itemTop < 0 || itemRows != 4 {
		t.Fatalf("bad geometry: top=%d rows=%d", itemTop, itemRows)
	}
	// Click the second item row ("plan").
	m.mouseOverlayClick(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 20, Y: itemTop + 1})
	if m.overlay != "" {
		t.Fatal("item click should close the overlay")
	}
	if currentMode(m.modeIndex).name != "plan" {
		t.Fatalf("item click should select 'plan', mode is %q", currentMode(m.modeIndex).name)
	}
}
