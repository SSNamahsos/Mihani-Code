// Package rtl makes Persian (and Arabic/Hebrew) text readable in terminals
// that lack bidirectional rendering: it shapes Arabic-script letters into
// their joined presentation forms and reorders bidi runs into visual order
// for display. The logical text is never modified — callers transform only
// what they render.
package rtl

import (
	"strings"

	"golang.org/x/text/unicode/bidi"
)

// zwnj (zero-width non-joiner) breaks letter joining inside a word. It is
// essential for correct Persian orthography (e.g. می‌روم).
const zwnj = '\u200C'

// forms holds the four Arabic Presentation Forms-B shapes of one base letter.
// Zero means the shape does not exist for that letter.
type forms struct{ isol, final, initial, medial rune }

// arabicForms maps base letters (U+0600 block + Persian additions) to their
// presentation forms. Letters missing from the table pass through unjoined.
var arabicForms = map[rune]forms{
	'\u0621': {isol: '\uFE80'},                                  // ء hamza
	'\u0622': {isol: '\uFE81', final: '\uFE82'},                 // آ alef madda
	'\u0623': {isol: '\uFE83', final: '\uFE84'},                 // أ alef hamza above
	'\u0624': {isol: '\uFE85', final: '\uFE86'},                 // ؤ waw hamza
	'\u0625': {isol: '\uFE87', final: '\uFE88'},                 // إ alef hamza below
	'\u0626': {isol: '\uFE89', final: '\uFE8A', initial: '\uFE8B', medial: '\uFE8C'}, // ئ yeh hamza
	'\u0627': {isol: '\uFE8D', final: '\uFE8E'},                 // ا alef
	'\u0628': {isol: '\uFE8F', final: '\uFE90', initial: '\uFE91', medial: '\uFE92'}, // ب beh
	'\u0629': {isol: '\uFE93', final: '\uFE94'},                 // ة teh marbuta
	'\u062A': {isol: '\uFE95', final: '\uFE96', initial: '\uFE97', medial: '\uFE98'}, // ت teh
	'\u062B': {isol: '\uFE99', final: '\uFE9A', initial: '\uFE9B', medial: '\uFE9C'}, // ث theh
	'\u062C': {isol: '\uFE9D', final: '\uFE9E', initial: '\uFE9F', medial: '\uFEA0'}, // ج jeem
	'\u062D': {isol: '\uFEA1', final: '\uFEA2', initial: '\uFEA3', medial: '\uFEA4'}, // ح hah
	'\u062E': {isol: '\uFEA5', final: '\uFEA6', initial: '\uFEA7', medial: '\uFEA8'}, // خ khah
	'\u062F': {isol: '\uFEA9', final: '\uFEAA'},                 // د dal
	'\u0630': {isol: '\uFEAB', final: '\uFEAC'},                 // ذ thal
	'\u0631': {isol: '\uFEAD', final: '\uFEAE'},                 // ر reh
	'\u0632': {isol: '\uFEAF', final: '\uFEB0'},                 // ز zain
	'\u0633': {isol: '\uFEB1', final: '\uFEB2', initial: '\uFEB3', medial: '\uFEB4'}, // س seen
	'\u0634': {isol: '\uFEB5', final: '\uFEB6', initial: '\uFEB7', medial: '\uFEB8'}, // ش sheen
	'\u0635': {isol: '\uFEB9', final: '\uFEBA', initial: '\uFEBB', medial: '\uFEBC'}, // ص sad
	'\u0636': {isol: '\uFEBD', final: '\uFEBE', initial: '\uFEBF', medial: '\uFEC0'}, // ض dad
	'\u0637': {isol: '\uFEC1', final: '\uFEC2', initial: '\uFEC3', medial: '\uFEC4'}, // ط tah
	'\u0638': {isol: '\uFEC5', final: '\uFEC6', initial: '\uFEC7', medial: '\uFEC8'}, // ظ zah
	'\u0639': {isol: '\uFEC9', final: '\uFECA', initial: '\uFECB', medial: '\uFECC'}, // ع ain
	'\u063A': {isol: '\uFECD', final: '\uFECE', initial: '\uFECF', medial: '\uFED0'}, // غ ghain
	'\u0641': {isol: '\uFED1', final: '\uFED2', initial: '\uFED3', medial: '\uFED4'}, // ف feh
	'\u0642': {isol: '\uFED5', final: '\uFED6', initial: '\uFED7', medial: '\uFED8'}, // ق qaf
	'\u0643': {isol: '\uFED9', final: '\uFEDA', initial: '\uFEDB', medial: '\uFEDC'}, // ك kaf
	'\u0644': {isol: '\uFEDD', final: '\uFEDE', initial: '\uFEDF', medial: '\uFEE0'}, // ل lam
	'\u0645': {isol: '\uFEE1', final: '\uFEE2', initial: '\uFEE3', medial: '\uFEE4'}, // م meem
	'\u0646': {isol: '\uFEE5', final: '\uFEE6', initial: '\uFEE7', medial: '\uFEE8'}, // ن noon
	'\u0647': {isol: '\uFEE9', final: '\uFEEA', initial: '\uFEEB', medial: '\uFEEC'}, // ه heh
	'\u0648': {isol: '\uFEED', final: '\uFEEE'},                 // و waw
	'\u0649': {isol: '\uFEEF', final: '\uFEF0'},                 // ى alef maksura
	'\u064A': {isol: '\uFEF1', final: '\uFEF2', initial: '\uFEF3', medial: '\uFEF4'}, // ي arabic yeh
	'\u067E': {isol: '\uFB56', final: '\uFB57', initial: '\uFB58', medial: '\uFB59'}, // پ peh
	'\u0686': {isol: '\uFB7C', final: '\uFB7D', initial: '\uFB7E', medial: '\uFB7F'}, // چ tcheh
	'\u0698': {isol: '\uFB8A', final: '\uFB8B'},                 // ژ jeh
	'\u06A9': {isol: '\uFB8E', final: '\uFB8F', initial: '\uFB90', medial: '\uFB91'}, // ک keheh
	'\u06AF': {isol: '\uFB92', final: '\uFB93', initial: '\uFB94', medial: '\uFB95'}, // گ gaf
	'\u06CC': {isol: '\uFBFC', final: '\uFBFD', initial: '\uFBFE', medial: '\uFBFF'}, // ی farsi yeh
	'\u06D2': {isol: '\uFBAE', final: '\uFBAF'},                 // ے yeh barree
}

// rightJoin lists letters that connect to the PREVIOUS letter only (they never
// join forward). Everything else in the table is dual-joining.
var rightJoin = map[rune]bool{
	'\u0621': true, '\u0622': true, '\u0623': true, '\u0624': true,
	'\u0625': true, '\u0627': true, '\u0629': true, '\u062F': true,
	'\u0630': true, '\u0631': true, '\u0632': true, '\u0648': true,
	'\u0649': true, '\u0698': true, '\u06CC': false, '\u06D2': true,
}

// combining marks are transparent for joining: they sit between letters
// without breaking the connection.
func transparent(r rune) bool {
	return (r >= 0x0610 && r <= 0x061A) || (r >= 0x064B && r <= 0x065F) || r == 0x0670
}

// lamAlef maps the alef variants that form a required lam-alef ligature.
var lamAlef = map[rune]bool{
	'\u0622': true, '\u0623': true, '\u0625': true, '\u0627': true,
}

const (
	lamAlefIsol = '\uFEFB'
	lamAlefFinal = '\uFEFC'
)

// shape converts Arabic-script letters to their contextual presentation
// forms, preserving any ANSI escapes carried by the cells. Non-Arabic
// characters (Latin, digits, punctuation, ZWNJ) pass through untouched;
// ZWNJ additionally breaks the joining on both sides.
func shape(s string) string {
	return joinCells(shapeCells(splitCells(s)))
}

func shapeCells(cells []cell) []cell {
	out := make([]cell, 0, len(cells))
	prevConnects := false // the previous letter connects forward into this one
	for i := 0; i < len(cells); i++ {
		r := cells[i].r
		if r == zwnj {
			prevConnects = false
			out = append(out, cells[i])
			continue
		}
		f, ok := arabicForms[r]
		if !ok {
			// Transparent marks do not disturb the current join state.
			if !transparent(r) {
				prevConnects = false
			}
			out = append(out, cells[i])
			continue
		}
		// Required lam-alef ligature: lam immediately followed by an alef
		// variant merges into one glyph.
		if r == '\u0644' && i+1 < len(cells) && lamAlef[cells[i+1].r] {
			lig := lamAlefIsol
			if prevConnects {
				lig = lamAlefFinal
			}
			merged := cell{pre: cells[i].pre + cells[i+1].pre, r: lig}
			out = append(out, merged)
			prevConnects = false // the ligature ends in alef: right-joining
			i++                  // consume the alef
			continue
		}
		// Look past transparent marks for the letter that would receive the
		// forward join.
		j := i + 1
		for j < len(cells) && transparent(cells[j].r) {
			j++
		}
		joinToNext := !rightJoin[r] && j < len(cells) && cells[j].r != zwnj && arabicForms[cells[j].r].isol != 0
		var chosen rune
		switch {
		case prevConnects && joinToNext && f.medial != 0:
			chosen = f.medial
		case prevConnects && f.final != 0:
			chosen = f.final
		case joinToNext && f.initial != 0:
			chosen = f.initial
		default:
			chosen = f.isol
		}
		out = append(out, cell{pre: cells[i].pre, r: chosen})
		prevConnects = joinToNext
	}
	return out
}

// HasRTL reports whether s contains strong right-to-left characters
// (Arabic or Hebrew script).
func HasRTL(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x0590 && r <= 0x05FF, // Hebrew
			r >= 0x0600 && r <= 0x06FF, // Arabic
			r >= 0x0750 && r <= 0x077F, // Arabic supplement
			r >= 0xFB50 && r <= 0xFDFF, // Arabic presentation A
			r >= 0xFE70 && r <= 0xFEFF: // Arabic presentation B
			return true
		}
	}
	return false
}

// baseDirection picks the paragraph direction with the "first strong
// character" rule: the first strong LTR or RTL character decides.
func baseDirection(s string) bidi.Direction {
	for _, r := range s {
		switch {
		case r >= 0x0590 && r <= 0x05FF, r >= 0x0600 && r <= 0x06FF,
			r >= 0x0750 && r <= 0x077F, r >= 0xFB50 && r <= 0xFDFF,
			r >= 0xFE70 && r <= 0xFEFF:
			return bidi.RightToLeft
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
			return bidi.LeftToRight
		}
	}
	return bidi.LeftToRight
}

func reverseRunes(s string) string {
	rs := []rune(s)
	for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
		rs[i], rs[j] = rs[j], rs[i]
	}
	return string(rs)
}

// cell is one display cell: the ANSI escape sequences that style it plus the
// rune it shows. Keeping escapes attached to cells lets the bidi transform
// reverse styled text without corrupting the sequences.
type cell struct {
	pre string // leading escape sequences
	r   rune
}

// Mode selects how right-to-left text is prepared for display. Terminals
// differ wildly here, so Mihani supports three:
//
//	ModeOff    — pass logical text through untouched. For terminals that
//	             implement bidi reordering AND Arabic shaping natively.
//	ModeBidi   — reorder the bidi runs into visual order but keep the base
//	             U+0600 codepoints, so the terminal's own shaper still joins
//	             the letters (Windows Terminal shapes via DirectWrite but
//	             does not reorder). Default.
//	ModeShaped — shape into Arabic Presentation Forms and reorder. For dumb
//	             terminals that do neither (classic console, many Linux
//	             terminal emulators).
type Mode string

const (
	ModeOff    Mode = "off"
	ModeBidi   Mode = "bidi"
	ModeShaped Mode = "shaped"
)

// NormalizeMode maps any stored string to a valid Mode, defaulting to bidi.
func NormalizeMode(s string) Mode {
	switch Mode(strings.ToLower(strings.TrimSpace(s))) {
	case ModeOff:
		return ModeOff
	case ModeShaped:
		return ModeShaped
	default:
		return ModeBidi
	}
}

// RTLBase reports whether the line's first strong character is right-to-left.
// ANSI escape sequences are ignored. Use it to decide right-alignment.
func RTLBase(s string) bool {
	return baseDirection(stripEscapes(s)) == bidi.RightToLeft
}

// Display transforms one logical line for display under the given mode.
// NEWLINES MUST NOT APPEAR — transform each line separately.
func Display(s string, mode Mode) string {
	switch mode {
	case ModeOff:
		return s
	case ModeBidi:
		if !HasRTL(s) {
			return s
		}
		return reorder(splitCells(s), baseDirection(s))
	default: // ModeShaped
		if !HasRTL(s) {
			return s
		}
		return reorder(shapeCells(splitCells(s)), baseDirection(s))
	}
}

// DisplayANSI is Display for a line that already contains ANSI styling: the
// escape sequences survive intact and travel with the runes they style.
func DisplayANSI(s string, mode Mode) string {
	switch mode {
	case ModeOff:
		return s
	case ModeBidi:
		if !HasRTL(stripEscapes(s)) {
			return s
		}
		return reorder(splitCells(s), baseDirection(stripEscapes(s)))
	default: // ModeShaped
		if !HasRTL(stripEscapes(s)) {
			return s
		}
		return reorder(shapeCells(splitCells(s)), baseDirection(stripEscapes(s)))
	}
}

func splitCells(s string) []cell {
	var cells []cell
	var pre strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\x1b' {
			// Consume the whole CSI sequence as part of the next cell.
			j := i + 1
			if j < len(rs) && rs[j] == '[' {
				j++
				for j < len(rs) && (rs[j] == ';' || (rs[j] >= '0' && rs[j] <= '9')) {
					j++
				}
				if j < len(rs) {
					j++ // final byte
				}
			} else {
				j = i + 2
			}
			pre.WriteString(string(rs[i:min(j, len(rs))]))
			i = j - 1
			continue
		}
		cells = append(cells, cell{pre: pre.String(), r: rs[i]})
		pre.Reset()
	}
	if pre.Len() > 0 && len(cells) > 0 {
		cells[len(cells)-1].pre += pre.String()
	}
	return cells
}

func stripEscapes(s string) string {
	var b strings.Builder
	for _, c := range splitCells(s) {
		b.WriteRune(c.r)
	}
	return b.String()
}

func joinCells(cells []cell) string {
	var b strings.Builder
	for _, c := range cells {
		b.WriteString(c.pre)
		b.WriteRune(c.r)
	}
	return b.String()
}

// reorder applies the simplified Unicode bidi algorithm over display cells:
// x/text resolves the directional runs in LOGICAL order; each RTL run has its
// cells reversed, and a right-to-left base paragraph reverses the run order.
func reorder(cells []cell, base bidi.Direction) string {
	logical := stripEscapes(joinCells(cells))
	p := &bidi.Paragraph{}
	if _, err := p.SetString(logical, bidi.DefaultDirection(base)); err != nil {
		return joinCells(cells)
	}
	ord, err := p.Order()
	if err != nil {
		return joinCells(cells)
	}
	var runs [][]cell
	pos := 0 // rune index into `logical`, aligned with cells
	for i := 0; i < ord.NumRuns(); i++ {
		run := ord.Run(i)
		n := len([]rune(run.String()))
		seg := cells[pos : pos+n]
		if run.Direction() == bidi.RightToLeft {
			for i2, j := 0, len(seg)-1; i2 < j; i2, j = i2+1, j-1 {
				seg[i2], seg[j] = seg[j], seg[i2]
			}
		}
		runs = append(runs, seg)
		pos += n
	}
	if pos < len(cells) {
		runs = append(runs, cells[pos:])
	}
	if base == bidi.RightToLeft {
		for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
			runs[i], runs[j] = runs[j], runs[i]
		}
	}
	var b strings.Builder
	for _, seg := range runs {
		b.WriteString(joinCells(seg))
	}
	return b.String()
}
