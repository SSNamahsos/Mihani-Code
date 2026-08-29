package ui

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)
// App-level drag selection. Selection positions live in CONTENT coordinates
// (rows of the full transcript, not the visible viewport), so scrolling with
// the wheel while a selection is armed keeps it anchored and lets the user
// extend it into newly visible rows. A plain click (no drag) opens the
// message action menu instead.

type selPos struct{ row, col int }

var selHighlight = lipgloss.NewStyle().Background(lipgloss.Color("#414a70"))

// contentRowOf maps a mouse screen row to a transcript content row.
// Screen layout: row 0 is the header, transcript content starts at row 1.
// Returns -1 when the position is outside the transcript.
func (m *Model) contentRowOf(y int) int {
	r := m.view.YOffset + y - 1
	if r < 0 || r >= len(m.renderedLines) {
		return -1
	}
	return r
}

func (m *Model) mousePress(x tea.MouseMsg) {
	m.clearSelection()
	r := m.contentRowOf(x.Y)
	if r < 0 {
		return
	}
	p := selPos{row: r, col: x.X}
	m.selOn = true
	m.selA = p
	m.selH = p
}

func (m *Model) mouseMove(x tea.MouseMsg) {
	if !m.selOn {
		return
	}
	r := m.view.YOffset + x.Y - 1
	if r < 0 {
		r = 0
	}
	if r >= len(m.renderedLines) {
		r = len(m.renderedLines) - 1
	}
	m.selH = selPos{row: r, col: x.X}
	if m.selH != m.selA {
		m.selDrag = true
	}
}

func (m *Model) mouseRelease(x tea.MouseMsg) {
	if !m.selOn {
		return
	}
	wasDrag := m.selDrag
	a, h := m.selA, m.selH
	m.clearSelection()
	if !wasDrag {
		// No movement: treat as a click.
		if m.busy {
			m.notify("wait for the current turn to finish")
			return
		}
		if b := m.nearUserMessage(x.Y); b >= 0 {
			m.openMessageMenu(b)
		}
		return
	}
	text := m.selectedText(a, h)
	if text == "" {
		return
	}
	if err := clipboard.WriteAll(text); err != nil {
		m.notify("Clipboard unavailable: " + err.Error())
		return
	}
	lines := strings.Count(text, "\n") + 1
	if lines == 1 {
		m.notify(fmt.Sprintf("Copied %d character(s) to clipboard", len([]rune(text))))
	} else {
		m.notify(fmt.Sprintf("Copied %d line(s) to clipboard", lines))
	}
}

func (m *Model) clearSelection() {
	if !m.selOn {
		return
	}
	m.selOn = false
	m.selDrag = false
	m.selA = selPos{}
	m.selH = selPos{}
	m.refreshView()
}

// selectedText extracts plain text for the (possibly un-ordered) selection
// range. Content lines are stripped of ANSI styles so the clipboard gets
// clean text; box-drawn borders around cards are trimmed when the whole
// interior line was selected.
func (m *Model) selectedText(a, h selPos) string {
	if len(m.renderedLines) == 0 {
		return ""
	}
	r1, c1, r2, c2 := a.row, a.col, h.row, h.col
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	if r1 < 0 {
		r1 = 0
	}
	if r2 >= len(m.renderedLines) {
		r2 = len(m.renderedLines) - 1
	}
	var out []string
	for i := r1; i <= r2; i++ {
		line := stripANSI(m.renderedLines[i])
		start, end := 0, len(line)
		if i == r1 {
			start = c1
		}
		if i == r2 {
			end = c2
		}
		if start < 0 {
			start = 0
		}
		if end > len(line) {
			end = len(line)
		}
		if start >= end {
			continue
		}
		chunk := line[start:end]
		// Drop the card border pair when the interior is selected.
		if strings.HasPrefix(chunk, "│ ") && strings.HasSuffix(chunk, " │") {
			chunk = strings.TrimRight(strings.TrimPrefix(chunk, "│ "), " │")
		}
		out = append(out, chunk)
	}
	text := strings.TrimRight(strings.Join(out, "\n"), " \t")
	return text
}

// highlightSelection paints the selected range in the styled transcript.
func highlightSelection(lines []string, a, h selPos) []string {
	r1, c1, r2, c2 := a.row, a.col, h.row, h.col
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	for i := r1; i <= r2 && i < len(lines); i++ {
		w := lipgloss.Width(lines[i])
		start, end := 0, w
		if i == r1 {
			start = c1
		}
		if i == r2 {
			end = c2
		}
		if start < 0 {
			start = 0
		}
		if end > w {
			end = w
		}
		if start >= end {
			continue
		}
		pre, rest := splitDisplay(lines[i], start)
		seg, post := splitDisplay(rest, end-start)
		if seg == "" {
			continue
		}
		lines[i] = pre + selHighlight.Render(seg) + post
	}
	return lines
}

// splitDisplay cuts s at display column col, skipping ANSI escape sequences
// (zero display width). Returns the part before and the part from that column.
func splitDisplay(s string, col int) (string, string) {
	if col <= 0 {
		return "", s
	}
	c := 0
	for i := 0; i < len(s); {
		r := s[i]
		if r == 0x1b {
			j := i + 1
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
				j++
			}
			if j < len(s) {
				i = j + 1 // skip the whole escape sequence
				continue
			}
			// Unterminated escape: treat as a normal byte.
		}
		c++
		i++
		if c >= col {
			return s[:i], s[i:]
		}
	}
	return s, ""
}
