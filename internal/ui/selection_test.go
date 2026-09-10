package ui

import (
	"fmt"
	"strings"
	"testing"
)

// Regression: selection columns are display columns, not byte offsets.
// Byte indexing used to garble every cut once the line contained multi-byte
// runes (box-drawing card borders, CJK text).
func TestPlainCutDisplayRespectsRuneWidths(t *testing.T) {
	s := "│ héllo wörld │"
	// cut at display column 2 (after "│ ")
	head, tail := plainCutDisplay(s, 2)
	if head != "│ " {
		t.Fatalf("head = %q, want %q", head, "│ ")
	}
	if tail != "héllo wörld │" {
		t.Fatalf("tail = %q", tail)
	}
	// wide rune: "你好" occupies 4 display columns; column 2 falls right
	// after 你, column 4 right after 好
	w := "你好世界"
	_, tail2 := plainCutDisplay(w, 2)
	if tail2 != "好世界" {
		t.Fatalf("cut after first wide rune: %q", tail2)
	}
	_, tail3 := plainCutDisplay(w, 4)
	if tail3 != "世界" {
		t.Fatalf("cut after second wide rune: %q", tail3)
	}
}

func TestSelectedTextUnicode(t *testing.T) {
	m := &Model{}
	m.renderedLines = []string{"│ 你好世界 │"}
	got := m.selectedText(selPos{row: 0, col: 0}, selPos{row: 0, col: 8})
	if strings.Contains(got, "\uFFFD") {
		t.Fatalf("selection produced replacement runes: %q", got)
	}
	if !strings.Contains(got, "你好") {
		t.Fatalf("selection lost wide-rune content: %q", got)
	}
}

func TestSplitDisplayStyled(t *testing.T) {
	styled := "\x1b[31m你好世界\x1b[0m plain"
	// display column 4 is after the two wide runes, inside the styled run
	pre, post := splitDisplay(styled, 4)
	if !strings.Contains(pre, "你好") || strings.Contains(pre, "世界") {
		t.Fatalf("pre = %q", pre)
	}
	if !strings.HasPrefix(post, "世界") {
		t.Fatalf("post = %q", post)
	}
}

// Wheel-scrolling mid-drag extends the selection into the newly revealed
// content rows instead of leaving the head where the pointer stopped.
func TestWheelScrollExtendsActiveSelection(t *testing.T) {
	m := newTestModel(80, 24)
	for i := 0; i < 10; i++ {
		m.appendBlock(&block{kind: blockUser, content: fmt.Sprintf("prompt %d", i)})
	}
	m.relayout()
	m.refreshView()
	total := len(m.renderedLines)
	if total < 5 {
		t.Fatalf("test setup: transcript too short (%d rows)", total)
	}
	m.selOn = true
	m.selDrag = true
	m.selA = selPos{row: total - 1, col: 0}
	m.selH = m.selA
	m.extendSelection(50) // far past the end: clamps to the last row
	if m.selH.row != total-1 {
		t.Fatalf("head row = %d, want clamp at %d", m.selH.row, total-1)
	}
	m.extendSelection(-total * 2)
	if m.selH.row != 0 {
		t.Fatalf("head row = %d, want clamp at 0", m.selH.row)
	}
	m.clearSelection()
	m.extendSelection(5)
	if m.selH != (selPos{}) {
		t.Fatal("extendSelection must be a no-op without an active selection")
	}
}
