package ui

import (
	"testing"
)

// plain UI mode must swap the box-drawing/braille glyphs for plain ASCII so
// terminals whose font lacks those glyphs stop rendering "?".
func TestPlainUIGlyphs(t *testing.T) {
	defer func() { plainUI = false }()

	plainUI = false
	if boxBorder().TopLeft != "╭" {
		t.Fatalf("default border should be rounded, got %q", boxBorder().TopLeft)
	}
	if spinFrame(0) != "⠋" {
		t.Fatalf("default spinner should start on a braille frame, got %q", spinFrame(0))
	}

	plainUI = true
	b := boxBorder()
	if b.TopLeft != "+" || b.Top != "-" || b.Left != "|" {
		t.Fatalf("plain border should be ASCII + - |, got %+v", b)
	}
	if bd := boxBorderDouble(); bd.TopLeft != "#" {
		t.Fatalf("plain double border should use #, got %q", bd.TopLeft)
	}
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		g := spinFrame(i)
		if r := []rune(g); len(r) > 1 {
			t.Fatalf("plain spinner must be a single ASCII char, got %q", g)
		}
		seen[g] = true
	}
	if len(seen) < 2 {
		t.Fatalf("plain spinner should animate, got %v", seen)
	}
}
