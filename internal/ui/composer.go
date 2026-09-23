package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SSNamahsos/Mihani-Code/internal/rtl"
)

// Composer is the prompt editor. It keeps the text in logical order (what
// Value() returns and what gets sent to the model) but renders it visually —
// Arabic/Persian rows are shaped into joined glyphs and reordered
// right-to-left, with the cursor mapped into visual space. This is what lets
// Persian read and type correctly in terminals that have no bidi support.
//
// It implements the subset of bubbles/textarea's API the app relies on, so it
// drops in where textarea.Model used to be.
type Composer struct {
	lines       [][]rune
	row, col    int // cursor in logical coordinates (line index, rune offset)
	width       int
	height      int
	focused     bool
	Placeholder string // mirrors textarea.Model's exported field
}

func newComposer() *Composer {
	return &Composer{
		lines:  [][]rune{{}},
		width:  60,
		height: 1,
	}
}

func (c *Composer) Value() string {
	out := make([]string, len(c.lines))
	for i, l := range c.lines {
		out[i] = string(l)
	}
	return strings.Join(out, "\n")
}

// nlNormalize folds CRLF/CR into LF so pasted and set text is uniform.
func nlNormalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func (c *Composer) SetValue(s string) {
	c.lines = nil
	for _, line := range strings.Split(nlNormalize(s), "\n") {
		c.lines = append(c.lines, []rune(line))
	}
	if len(c.lines) == 0 {
		c.lines = [][]rune{{}}
	}
	// textarea semantics: the cursor lands at the end of the new value.
	c.row = len(c.lines) - 1
	c.col = len(c.lines[c.row])
}

func (c *Composer) Reset() {
	c.lines = [][]rune{{}}
	c.row, c.col = 0, 0
}

// InsertString inserts text at the cursor, honouring embedded newlines.
func (c *Composer) InsertString(s string) {
	parts := strings.Split(nlNormalize(s), "\n")
	line := c.lines[c.row]
	head := append([]rune{}, line[:c.col]...)
	tail := append([]rune{}, line[c.col:]...)
	first := []rune(parts[0])
	if len(parts) == 1 {
		c.lines[c.row] = append(append(head, first...), tail...)
		c.col += len(first)
		c.clampCursor()
		return
	}
	c.lines[c.row] = append(head, first...)
	inserted := make([][]rune, 0, len(parts)-1)
	for _, p := range parts[1:] {
		inserted = append(inserted, []rune(p))
	}
	// The last inserted line carries the old tail of the split line; the
	// cursor stops at the end of the inserted part, before that tail.
	lastPart := inserted[len(inserted)-1]
	inserted[len(inserted)-1] = append(lastPart, tail...)
	c.lines = append(c.lines[:c.row+1], append(inserted, c.lines[c.row+1:]...)...)
	c.row += len(inserted)
	c.col = len(lastPart)
	c.clampCursor()
}

func (c *Composer) Focus()      { c.focused = true }
func (c *Composer) Blur()       { c.focused = false }
func (c *Composer) Height() int { return c.height }

func (c *Composer) SetHeight(h int) {
	if h < 1 {
		h = 1
	}
	c.height = h
}

func (c *Composer) SetWidth(w int) {
	if w < 8 {
		w = 8
	}
	c.width = w
}

func (c *Composer) Width() int { return c.width }

func (c *Composer) clampCursor() {
	if c.row >= len(c.lines) {
		c.row = len(c.lines) - 1
	}
	if c.row < 0 {
		c.row = 0
	}
	if c.col > len(c.lines[c.row]) {
		c.col = len(c.lines[c.row])
	}
	if c.col < 0 {
		c.col = 0
	}
}

// Update handles the editing keys; unknown messages are ignored. Enter never
// reaches the composer — the model layer intercepts it for submitting.
func (c *Composer) Update(msg tea.Msg) (*Composer, tea.Cmd) {
	x, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	switch x.Type {
	case tea.KeyRunes, tea.KeySpace:
		if len(x.Runes) > 0 {
			c.InsertString(string(x.Runes))
		}
	case tea.KeyBackspace:
		c.deleteBackward()
	case tea.KeyDelete:
		c.deleteForward()
	case tea.KeyLeft:
		if c.col > 0 {
			c.col--
		} else if c.row > 0 {
			c.row--
			c.col = len(c.lines[c.row])
		}
	case tea.KeyRight:
		if c.col < len(c.lines[c.row]) {
			c.col++
		} else if c.row < len(c.lines)-1 {
			c.row++
			c.col = 0
		}
	case tea.KeyUp:
		if c.row > 0 {
			c.row--
			c.clampCursor()
		}
	case tea.KeyDown:
		if c.row < len(c.lines)-1 {
			c.row++
			c.clampCursor()
		}
	case tea.KeyHome:
		c.col = 0
	case tea.KeyEnd:
		c.col = len(c.lines[c.row])
	default:
		switch x.String() {
		case "ctrl+a":
			c.col = 0
		case "ctrl+e":
			c.col = len(c.lines[c.row])
		case "ctrl+k":
			c.lines[c.row] = c.lines[c.row][:c.col]
		case "ctrl+u":
			c.lines[c.row] = c.lines[c.row][c.col:]
			c.col = 0
		case "ctrl+w", "alt+backspace":
			c.deleteWordBackward()
		}
	}
	return c, nil
}

func (c *Composer) deleteBackward() {
	if c.col > 0 {
		line := c.lines[c.row]
		c.lines[c.row] = append(line[:c.col-1], line[c.col:]...)
		c.col--
		return
	}
	if c.row > 0 {
		prev := c.lines[c.row-1]
		c.col = len(prev)
		c.lines[c.row-1] = append(prev, c.lines[c.row]...)
		c.lines = append(c.lines[:c.row], c.lines[c.row+1:]...)
		c.row--
	}
}

func (c *Composer) deleteForward() {
	line := c.lines[c.row]
	if c.col < len(line) {
		c.lines[c.row] = append(line[:c.col], line[c.col+1:]...)
		return
	}
	if c.row < len(c.lines)-1 {
		c.lines[c.row] = append(line, c.lines[c.row+1]...)
		c.lines = append(c.lines[:c.row+1], c.lines[c.row+2:]...)
	}
}

func (c *Composer) deleteWordBackward() {
	line := c.lines[c.row]
	i := c.col
	for i > 0 && line[i-1] == ' ' {
		i--
	}
	for i > 0 && line[i-1] != ' ' {
		i--
	}
	c.lines[c.row] = append(line[:i], line[c.col:]...)
	c.col = i
}

// composerCursorStyle draws the cursor cell in inverse video.
var composerCursorStyle = lipgloss.NewStyle().Reverse(true)

// View renders the composer: logical lines wrapped to the width, every visual
// row with RTL characters shaped and reordered, cursor mapped accordingly.
func (c *Composer) View() string {
	if c.Value() == "" && c.Placeholder != "" {
		body := lipgloss.NewStyle().Foreground(colDim).Render(truncateWord(c.Placeholder, maxInt(8, c.width)))
		if c.focused {
			body = composerCursorStyle.Render(" ") + body
		}
		return body
	}
	w := maxInt(8, c.width)
	var rows []string
	cursorRow := 0
	for li, line := range c.lines {
		// Wrap the logical line into rows of at most w display cells.
		type chunk struct {
			text  string
			start int // rune offset in the logical line
			runes []rune
		}
		var chunks []chunk
		width := 0
		for i, r := range line {
			rw := runewidth.RuneWidth(r)
			if rw < 1 {
				rw = 1
			}
			if len(chunks) == 0 || width+rw > w {
				chunks = append(chunks, chunk{start: i})
				width = 0
			}
			ci := len(chunks) - 1
			chunks[ci].runes = append(chunks[ci].runes, r)
			width += rw
		}
		if len(chunks) == 0 {
			chunks = append(chunks, chunk{})
		}

		for ci := range chunks {
			ch := &chunks[ci]
			rowLen := len(ch.runes)
			isLastChunk := ci == len(chunks)-1
			// The cursor belongs to this row when it falls inside it, or at
			// the very end of the logical line (last row owns it).
			onCursorRow := c.row == li &&
				(c.col >= ch.start && c.col < ch.start+rowLen ||
					(c.col == ch.start+rowLen && isLastChunk))
			text := string(ch.runes)
			if rtlMode != rtl.ModeOff && rtl.HasRTL(text) {
				shaped := rtl.DisplayANSI(text, rtlMode)
				if onCursorRow {
					p := c.col - ch.start
					if c.col == len(line) && isLastChunk {
						p = rowLen
					}
					cellIdx := len([]rune(stripANSI(shaped))) - (p - lamAlefBefore(ch.runes, p))
					if cellIdx < 0 {
						cellIdx = 0
					}
					rows = append(rows, alignRTL(drawCursor(shaped, cellIdx), w))
				} else {
					rows = append(rows, alignRTL(shaped, w))
				}
			} else {
				if onCursorRow {
					rows = append(rows, drawCursor(text, c.col-ch.start))
				} else {
					rows = append(rows, text)
				}
			}
			if onCursorRow {
				cursorRow = len(rows) - 1
			}
		}
	}
	// Keep the cursor row inside the visible window when content overflows
	// the composer height.
	start := 0
	if len(rows) > c.height {
		start = cursorRow - (c.height - 1)
		if start < 0 {
			start = 0
		}
		if start > len(rows)-c.height {
			start = len(rows) - c.height
		}
	}
	shown := rows[start:]
	if len(shown) > c.height {
		shown = shown[:c.height]
	}
	return strings.Join(shown, "\n")
}

// drawCursor overlays the cursor on the visual cell at idx (or appends one at
// the end when idx == len).
func drawCursor(cells string, idx int) string {
	rs := []rune(cells)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rs) {
		return cells + composerCursorStyle.Render(" ")
	}
	return string(rs[:idx]) + composerCursorStyle.Render(string(rs[idx])) + string(rs[idx+1:])
}

// lamAlefBefore counts lam-alef pairs in runes whose second rune lies fully
// before logical position p (each pair occupies one visual cell instead of two).
func lamAlefBefore(runes []rune, p int) int {
	n := 0
	for i := 0; i+1 < len(runes) && i+1 < p; i++ {
		if runes[i] == '\u0644' && (runes[i+1] == '\u0622' || runes[i+1] == '\u0623' || runes[i+1] == '\u0625' || runes[i+1] == '\u0627') {
			n++
			i++
		}
	}
	return n
}
