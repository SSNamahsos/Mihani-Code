package rtl

import (
	"strings"
	"testing"
)

func TestHasRTL(t *testing.T) {
	if HasRTL("hello world 123") {
		t.Fatal("latin text must not be detected as RTL")
	}
	if !HasRTL("سلام") {
		t.Fatal("persian text must be detected as RTL")
	}
	if !HasRTL("code := true // توضیح") {
		t.Fatal("mixed line with persian comment must be detected as RTL")
	}
}

// سلام = س(initial FEB3) + lam-alef ligature(final FEFC, joined from س) +
// م(isolated FEE1, because the ligature ends in a right-joining alef).
func TestShapePersianWord(t *testing.T) {
	got := shape("سلام")
	if got != "\uFEB3\uFEFC\uFEE1" {
		t.Fatalf("shape(سلام) = %U, want [\\uFEB3 \\uFEFC \\uFEE1]", []rune(got))
	}
}

// می: م joins forward (initial FEE3), ی only receives (final FBFD).
func TestShapeJoiningForms(t *testing.T) {
	got := shape("می")
	if got != "\uFEE3\uFBFD" {
		t.Fatalf("shape(می) = %U, want [\\uFEE3 \\uFBFD]", []rune(got))
	}
}

// ZWNJ breaks the join on both sides: می‌ر starts م-ی joined, then ر isolated.
func TestShapeZWNJBreaksJoining(t *testing.T) {
	got := shape("می‌ر")
	want := "\uFEE3\uFBFD\u200C\uFEAD"
	if got != want {
		t.Fatalf("shape(می‌ر) = %U, want %U", []rune(got), []rune(want))
	}
}

// Display turns logical Persian into visual order for terminals without bidi:
// the shaped glyphs come back right-to-left.
func TestDisplayReversesForVisual(t *testing.T) {
	got := Display("سلام", ModeShaped)
	if got != "\uFEE1\uFEFC\uFEB3" {
		t.Fatalf("Display(سلام) = %U, want [\\uFEE1 \\uFEFC \\uFEB3]", []rune(got))
	}
}

// A persian word followed by latin: base RTL puts the latin word to the left,
// the persian word to the right reading correctly.
func TestDisplayMixed(t *testing.T) {
	got := Display("سلام world", ModeShaped)
	if got != "world \uFEE1\uFEFC\uFEB3" {
		t.Fatalf("Display(سلام world) = %q (%U)", got, []rune(got))
	}
}

// Digits flow left-to-right even inside an RTL sentence.
func TestDisplayKeepsDigitOrder(t *testing.T) {
	got := Display("نسخه ۱۲۳۴ خوب است", ModeShaped)
	for _, want := range []string{"۱", "۲", "۳", "۴"} {
		if !strings.Contains(got, want) {
			t.Fatalf("digit %s lost in %q", want, got)
		}
	}
	i1, i2, i3, i4 := strings.Index(got, "۱"), strings.Index(got, "۲"), strings.Index(got, "۳"), strings.Index(got, "۴")
	if !(i1 < i2 && i2 < i3 && i3 < i4) {
		t.Fatalf("digits reordered: %q", got)
	}
}

// ANSI styling must survive the transform: no mangled escapes.
func TestDisplayANSIPreservesEscapes(t *testing.T) {
	line := "\x1b[31mسلام\x1b[0m"
	got := DisplayANSI(line, ModeShaped)
	if strings.Count(got, "\x1b[31m") != 1 || strings.Count(got, "\x1b[0m") != 1 {
		t.Fatalf("escapes corrupted: %q", got)
	}
	if strings.Contains(got, "\x1b[3") && strings.Contains(got, "31mسلام") {
		t.Fatalf("escape fused with text: %q", got)
	}
	// The glyphs must still be the shaped, reversed ones.
	if !strings.Contains(stripEscapes(got), "\uFEE1\uFEFC\uFEB3") {
		t.Fatalf("glyphs not visually reordered: %U", []rune(stripEscapes(got)))
	}
}

// Non-RTL input short-circuits unchanged.
func TestDisplayFastPath(t *testing.T) {
	const s = "plain english line"
	if got := Display(s, ModeShaped); got != s {
		t.Fatalf("fast path changed the text: %q", got)
	}
	if got := DisplayANSI("\x1b[1mbold\x1b[0m", ModeShaped); got != "\x1b[1mbold\x1b[0m" {
		t.Fatalf("ANSI fast path changed the text: %q", got)
	}
}

// Hebrew gets bidi reordering (no shaping) too.
func TestHebrewBidi(t *testing.T) {
	got := Display("שלום", ModeShaped)
	if got != reverseRunes("שלום") {
		t.Fatalf("hebrew not reversed: %q", got)
	}
}

// ModeOff passes RTL text through untouched (terminals with native bidi).
func TestModeOffPassthrough(t *testing.T) {
	const s = "سلام دنیا"
	if got := Display(s, ModeOff); got != s {
		t.Fatalf("ModeOff changed the text: %q", got)
	}
	if got := DisplayANSI("\x1b[31m"+s+"\x1b[0m", ModeOff); got != "\x1b[31m"+s+"\x1b[0m" {
		t.Fatalf("ModeOff changed styled text: %q", got)
	}
}

// ModeBidi reorders runs but keeps the BASE Arabic codepoints, so a terminal
// with a native shaper (Windows Terminal) still joins the letters itself.
func TestModeBidiKeepsBaseCodepoints(t *testing.T) {
	got := Display("سلام", ModeBidi)
	// Reversed LOGICAL order, no presentation forms: م ا ل س
	if got != "\u0645\u0627\u0644\u0633" {
		t.Fatalf("ModeBidi = %U, want base letters reversed", []rune(got))
	}
	if NormalizeMode("off") != ModeOff || NormalizeMode("shaped") != ModeShaped ||
		NormalizeMode("bidi") != ModeBidi || NormalizeMode("garbage") != ModeBidi {
		t.Fatal("NormalizeMode broken")
	}
	if !RTLBase("سلام") || RTLBase("hello") {
		t.Fatal("RTLBase wrong")
	}
}
