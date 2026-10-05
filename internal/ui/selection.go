package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

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

// Mouse reporting is a pair of terminal control sequences, and the halves only
// work together:
//
//   - 1002 (button-event tracking) makes the terminal report presses, releases,
//     the wheel, and motion while a button is held — what drag selection needs.
//   - 1006 (SGR encoding) makes those reports plain ASCII: "ESC [ < b ; x ; y M".
//     Without it the terminal falls back to the legacy X10 encoding, "ESC [ M"
//     followed by three raw bytes that are each a coordinate offset by 32.
//     Those bytes are printable, and "ESC [ M" on its own is a complete (if
//     meaningless) CSI sequence, so a report that arrives split across two reads
//     — routine on a console — is mis-parsed: the header is swallowed as a
//     stray sequence and the three payload bytes reach the composer as typed
//     characters. That is the reported "####@@!" typed into the chat box.
//
// tea.Program.EnableMouseCellMotion writes only 1002; the startup path
// (tea.WithMouseCellMotion) writes both. Toggling capture on at runtime through
// that helper is what used to leave the terminal reporting in X10 mode, so
// this app writes both halves itself.
const (
	mouseModeOn  = "\x1b[?1002h\x1b[?1006h"
	mouseModeOff = "\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1015l"
)

// setMouseMode switches the terminal's mouse reporting on or off, handing the
// terminal's own text selection back when it goes off.
//
// The sequence goes out in one write. A terminal parses DECSET in stream order,
// but splitting the write would let a renderer frame land in the middle of it.
// tea.Printf is not an option: bubbletea discards it while the alternate screen
// is active, and this app always runs in one.
func (m *Model) setMouseMode(on bool) {
	seq := mouseModeOff
	if on {
		seq = mouseModeOn
	}
	if m.out == nil {
		return
	}
	_, _ = io.WriteString(m.out, seq)
}

// mouseDebugLog appends raw mouse traffic to %TEMP%\mihani-mouse.log while
// MIHANI_DEBUG is set — isolates terminal input issues in the field.
func mouseDebugLog(x tea.MouseMsg, m *Model) {
	if os.Getenv("MIHANI_DEBUG") == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "mihani-mouse.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s button=%v action=%v x=%d y=%d yoffset=%d rows=%d selOn=%v overlay=%q busy=%v\n",
		time.Now().Format("15:04:05.000"), x.Button, x.Action, x.X, x.Y, m.view.YOffset, len(m.renderedLines), m.selOn, m.overlay, m.busy)
}

// mouseDebugProbe writes a startup line to the same log so a missing log
// file means the env var never reached the process, while an empty log means
// mouse events never reached Update.
func mouseDebugProbe(version string, mouseEnabled bool, useMouseSet bool) {
	if os.Getenv("MIHANI_DEBUG") == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "mihani-mouse.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s PROBE version=%s mouse_enabled=%v use_mouse_set=%v wt_session=%q term=%q term_program=%q\n",
		time.Now().Format("15:04:05.000"), version, mouseEnabled, useMouseSet, os.Getenv("WT_SESSION"), os.Getenv("TERM"), os.Getenv("TERM_PROGRAM"))
}

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
	p := selPos{row: r, col: x.X}
	prev := m.selH
	m.selH = p
	// Jitter dead zone: terminals (notably Windows Terminal) deliver spurious
	// ±1 cell motion events on plain clicks, which used to turn a click into
	// a drag and silently swallow the menu. Only genuine movement counts.
	if !m.selDrag {
		dx := p.col - m.selA.col
		if dx < 0 {
			dx = -dx
		}
		dy := p.row - m.selA.row
		if dy < 0 {
			dy = -dy
		}
		if dx >= 2 || dy >= 2 {
			m.selDrag = true
		}
	}
	// Drag-to-edge auto-scroll: holding the pointer near the top or bottom
	// edge of the transcript scrolls the viewport and extends the selection
	// into the newly revealed rows, so a selection can span more than one
	// screen. The native terminal selection can never do this (it is
	// screen-anchored and the app runs in the alternate buffer).
	const edge = 2
	top := 1 // transcript starts under the header row
	bottom := top + m.view.Height - 1
	offset := m.view.YOffset
	switch {
	case x.Y <= top+edge:
		m.scrollUp(2)
		m.moveSelectionHead(-2)
	case x.Y >= bottom-edge:
		m.scrollDown(2)
		m.moveSelectionHead(2)
	}
	// Repaint only when something the user can see actually changed.
	// Button-event tracking (1002) streams a motion message for every cell the
	// pointer crosses, and refreshView re-renders every block in the transcript,
	// so repainting unconditionally queued hundreds of full re-renders for a
	// single drag and locked the UI up. Pointer jitter before the drag threshold
	// is crossed changes nothing on screen either, so it skips the repaint too.
	if m.view.YOffset != offset || (m.selDrag && p != prev) {
		m.refreshView()
	}
}

// extendSelection moves the drag head when the transcript scrolls during an
// active selection, then repaints. The anchor is content-anchored already; the
// head follows the content so a wheel scroll mid-drag grows the selection into
// whatever the scroll revealed. No-op when nothing is being selected.
func (m *Model) extendSelection(delta int) {
	if !m.selOn {
		return
	}
	m.moveSelectionHead(delta)
	m.refreshView()
}

// moveSelectionHead shifts the drag head to follow the content the viewport
// just scrolled to, keeping it inside the transcript. No-op when nothing is
// being selected.
func (m *Model) moveSelectionHead(delta int) {
	if !m.selOn {
		return
	}
	m.selH.row += delta
	if m.selH.row < 0 {
		m.selH.row = 0
	}
	if m.selH.row >= len(m.renderedLines) {
		m.selH.row = len(m.renderedLines) - 1
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
		// No movement: treat as a click. The menu opens even mid-turn; the
		// destructive actions (revert/fork) guard themselves on m.busy.
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
// interior line was selected. Columns are display columns (rune widths), not
// bytes — indexing bytes used to garble every cut on non-ASCII output
// (box-drawing borders, CJK text).
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
		start, end := 0, lipgloss.Width(line)
		if i == r1 {
			_, tail := plainCutDisplay(line, c1)
			start = len(line) - len(tail)
		}
		if i == r2 {
			_, tail := plainCutDisplay(line, c2)
			end = len(line) - len(tail)
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

// plainCutDisplay splits ANSI-free text at a display column, respecting rune
// widths (a wide CJK glyph occupies two columns; a combining mark zero).
func plainCutDisplay(s string, col int) (string, string) {
	if col <= 0 {
		return "", s
	}
	w := 0
	for i, r := range s {
		if w >= col {
			return s[:i], s[i:]
		}
		w += lipgloss.Width(string(r))
	}
	return s, ""
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

// splitDisplay cuts a styled string at a display column, skipping ANSI escape
// sequences (zero display width) and respecting rune widths. Returns the part
// before and the part from that column.
func splitDisplay(s string, col int) (string, string) {
	if col <= 0 {
		return "", s
	}
	c := 0
	for i := 0; i < len(s); {
		r := s[i]
		if r == 0x1b {
			// Escape sequence: skip the introducer ([ ] P X ^ _), then all
			// parameter/intermediate bytes (0x20-0x3F), up to and including
			// the final byte (0x40-0x7E). Stopping at the introducer itself
			// (it is inside 0x40-0x7E) used to count "[31m" as display text.
			j := i + 1
			if j < len(s) && (s[j] == '[' || s[j] == ']' || s[j] == 'P' || s[j] == 'X' || s[j] == '^' || s[j] == '_') {
				j++
			}
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				i = j + 1 // skip the whole escape sequence
				continue
			}
			// Unterminated escape: treat as a normal byte.
		}
		size := 1
		if r >= 0x80 {
			if _, n := utf8.DecodeRuneInString(s[i:]); n > 1 {
				size = n
			}
		}
		c += lipgloss.Width(s[i : i+size])
		i += size
		if c >= col {
			return s[:i], s[i:]
		}
	}
	return s, ""
}
