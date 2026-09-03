package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"

	"github.com/SSNamahsos/Mihani-Code/internal/tools"
)

type blockKind int

const (
	blockUser blockKind = iota
	blockAssistant
	blockTool
	blockTodo
	blockInfo
	blockError
	blockThinking
)

// Tool statuses.
const (
	statusRunning = "running"
	statusDone    = "done"
	statusError   = "error"
	statusDenied  = "denied"
)

type block struct {
	kind      blockKind
	label     string // tool name
	detail    string // argument summary
	content   string // body text (user prompt, assistant reply, diff preview)
	status    string
	finalized bool
	focused   bool // user message under the keyboard action cursor
	width     int  // zero means the cached render is stale
	rendered  string
}

func (b *block) invalidate() { b.width = 0 }

func wrap(s string, w int) string {
	if w < 12 {
		w = 12
	}
	return lipgloss.NewStyle().Width(w).Render(s)
}

func indentBlock(s string, pad int) string {
	prefix := strings.Repeat(" ", pad)
	lines := strings.Split(s, "\n")
	for i := range lines {
		if lines[i] != "" {
			lines[i] = prefix + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

// render returns the block's display output, re-rendering only when stale.
func (b *block) render(w int, spinnerChar string) string {
	if b.width == w && b.width != 0 {
		return b.rendered
	}
	switch b.kind {
	case blockUser:
		b.rendered = b.renderUser(w)
	case blockAssistant:
		b.rendered = b.renderAssistant(w, spinnerChar)
	case blockThinking:
		b.rendered = b.renderThinking(w, spinnerChar)
	case blockTool:
		b.rendered = b.renderTool(w, spinnerChar)
	case blockTodo:
		b.rendered = b.renderTodo(w, spinnerChar)
	case blockInfo:
		b.rendered = lipgloss.NewStyle().Foreground(colDim).
			Render(indentBlock(wrap(b.content, w-2), 2))
	case blockError:
		b.rendered = lipgloss.NewStyle().Foreground(colRed).Render(
			indentBlock(wrap("✗ "+b.content, w-2), 2))
	}
	b.width = w
	return b.rendered
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (b *block) renderUser(w int) string {
	body := wrap(b.content, w-6)
	lines := strings.Split(body, "\n")
	header := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("❯ you")
	var inner strings.Builder
	inner.WriteString(lipgloss.NewStyle().Foreground(colBright).Render(strings.TrimRight(lines[0], " ")))
	for _, line := range lines[1:] {
		inner.WriteString("\n" + lipgloss.NewStyle().Foreground(colBright).Render(strings.TrimRight(line, " ")))
	}
	if b.focused {
		hints := lipgloss.NewStyle().Foreground(colFaint).
			Render("[y] copy  [f] fork from here  [r] revert to composer  [esc] close")
		inner.WriteString("\n" + hints)
	}
	border := colFaint
	borderStyle := boxBorder()
	if b.focused {
		border = colAccent
	}
	return lipgloss.NewStyle().
		Border(borderStyle).
		BorderForeground(border).
		Padding(0, 1).
		Width(maxInt(12, w-2)).
		Render(header + "\n" + inner.String())
}

// focused marks the block under the keyboard action cursor.
func (b *block) focusedBox() bool { return b.focused }

func (b *block) renderAssistant(w int, spinnerChar string) string {
	var body string
	if b.finalized {
		body = renderMarkdown(b.content, w)
	} else {
		text := b.content
		if spinnerChar != "" {
			text += " " + lipgloss.NewStyle().Foreground(colFaint).Render(spinnerChar)
		}
		body = lipgloss.NewStyle().Foreground(colText).Render(wrap(text, w-2))
	}
	return indentBlock(body, 2)
}

// renderThinking shows live model reasoning in a dimmed, clearly secondary
// block so it never competes with the actual answer.
func (b *block) renderThinking(w int, spinnerChar string) string {
	header := lipgloss.NewStyle().Foreground(colFaint).Bold(true).
		Render("✻ thinking")
	if !b.finalized && spinnerChar != "" {
		header += " " + lipgloss.NewStyle().Foreground(colFaint).Render(spinnerChar)
	}
	body := lipgloss.NewStyle().Foreground(colFaint).Italic(true).
		Render(wrap(b.content, w-4))
	out := header + "\n" + indentBlock(body, 2)
	const maxThinkingLines = 12
	lines := strings.Split(out, "\n")
	if len(lines) > maxThinkingLines {
		// Keep the head visible while streaming; collapse the middle.
		shown := append(lines[:6], lines[len(lines)-5:]...)
		out = strings.Join(shown, "\n") + "\n" +
			lipgloss.NewStyle().Foreground(colFaint).Render("  … reasoning continues")
	}
	return out
}

// sanitizeStream converts raw streamed model output into display-safe text:
// complete <tool_call>/<tool_result>-style protocol blocks are removed (they
// are rendered as their own tool cards), and any trailing partial opener is
// withheld until more stream arrives.
func sanitizeStream(raw string) (clean string, cutAt int) {
	s := completeToolCallRe.ReplaceAllString(raw, "")
	s = completeFenceRe.ReplaceAllString(s, "")

	cut := len(s)
	if idx := partialOpener(s, "<tool_call>"); idx >= 0 {
		cut = idx
	}
	if idx := partialOpener(s, "```tool_call"); idx >= 0 && idx < cut {
		cut = idx
	}
	return s[:cut], cut
}

var (
	completeToolCallRe = regexp.MustCompile(`(?s)<tool_call>.*?</tool_call>`)
	completeFenceRe    = regexp.MustCompile("(?s)```tool_call.*?```")
)

// partialOpener finds where an unterminated opener begins so its tail stays
// hidden mid-stream; returns -1 when no partial opener is pending.
func partialOpener(s, opener string) int {
	start := strings.LastIndex(s, opener)
	if start >= 0 && !strings.Contains(s[start:], closerFor(opener)) {
		return start
	}
	// Also withhold a bare "<" tail that could be the birth of "<tool_call>".
	tail := s
	for width := 1; width < len(opener) && width <= len(tail); width++ {
		candidate := tail[len(tail)-width:]
		if strings.HasPrefix(opener, candidate) {
			return len(tail) - width
		}
	}
	return -1
}

func closerFor(opener string) string {
	if opener == "```tool_call" {
		return "```"
	}
	return "</tool_call>"
}

var toolStatusStyles = map[string]lipgloss.Style{
	statusDone:   lipgloss.NewStyle().Foreground(colGreen),
	statusError:  lipgloss.NewStyle().Foreground(colRed),
	statusDenied: lipgloss.NewStyle().Foreground(colAmber),
}

var toolStatusMarks = map[string]string{
	statusDone:   "✓",
	statusError:  "✗",
	statusDenied: "⊘",
}

func (b *block) renderTool(w int, spinnerChar string) string {
	var glyph string
	if style, ok := toolStatusStyles[b.status]; ok {
		glyph = style.Render(toolStatusMarks[b.status])
	} else {
		glyph = lipgloss.NewStyle().Foreground(colAmber).Render(spinnerChar)
	}
	name := lipgloss.NewStyle().Bold(true).Foreground(colText).Render(b.label)
	available := w - lipgloss.Width(glyph) - lipgloss.Width(name) - 4
	if available < 8 {
		available = 8
	}
	detail := ""
	if b.detail != "" {
		detail = lipgloss.NewStyle().Foreground(colDim).Render(truncate.String(b.detail, uint(available)))
	}
	out := glyph + " " + name
	if detail != "" {
		out += "  " + detail
	}
	if b.content != "" {
		out += "\n" + colorizeDiff(indentBlock(b.content, 1), w-6)
	}
	// Tool activity lives in its own bordered card so it reads as a distinct
	// unit, separate from prose.
	borderColor := colBorder
	switch b.status {
	case statusError:
		borderColor = colRed
	case statusDenied:
		borderColor = colAmber
	}
	return lipgloss.NewStyle().
		Border(boxBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(maxInt(14, w-2)).
		Render(out)
}

// renderTodo shows the live task list: one line per item, tinted by status,
// inside the same card chrome as tool activity.
func (b *block) renderTodo(w int, spinnerChar string) string {
	var header string
	if b.status == statusRunning && !b.finalized {
		spin := ""
		if spinnerChar != "" {
			spin = " " + lipgloss.NewStyle().Foreground(colFaint).Render(spinnerChar)
		}
		header = lipgloss.NewStyle().Bold(true).Foreground(colAmber).Render("☑ todos") + spin
	} else {
		summary := lipgloss.NewStyle().Foreground(colFaint).Render(" "+b.detail)
		header = lipgloss.NewStyle().Bold(true).Foreground(colGreen).Render("☑ todos") + summary
	}
	var body strings.Builder
	for i, line := range strings.Split(strings.TrimRight(b.content, "\n"), "\n") {
		if i > 0 {
			body.WriteString("\n")
		}
		switch {
		case strings.HasPrefix(line, "✓"):
			body.WriteString(lipgloss.NewStyle().Foreground(colGreen).Render(line))
		case strings.HasPrefix(line, "◐"):
			body.WriteString(lipgloss.NewStyle().Foreground(colAmber).Render(line))
		default:
			body.WriteString(lipgloss.NewStyle().Foreground(colDim).Render(line))
		}
	}
	if b.content == "" {
		body.WriteString(lipgloss.NewStyle().Foreground(colFaint).Render("(updating…)"))
	}
	borderColor := colBorder
	if b.status == statusError {
		borderColor = colRed
	}
	return lipgloss.NewStyle().
		Border(boxBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(maxInt(14, w-2)).
		Render(header + "\n" + body.String())
}

const maxPreviewLines = 14

// colorizeDiff tints unified-diff lines and caps their height.
func colorizeDiff(diff string, w int) string {
	added := lipgloss.NewStyle().Foreground(colGreen)
	removed := lipgloss.NewStyle().Foreground(colRed)
	header := lipgloss.NewStyle().Foreground(colFaint)
	lines := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	shown := 0
	var out []string
	for _, line := range lines {
		if shown >= maxPreviewLines {
			out = append(out, lipgloss.NewStyle().Foreground(colFaint).
				Render(fmt.Sprintf("   … %d more line(s)", len(lines)-shown)))
			break
		}
		line = truncate.String(line, uint(maxInt(20, w-4)))
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			out = append(out, header.Render(line))
			continue
		case strings.HasPrefix(line, "+"):
			out = append(out, added.Render(line))
		case strings.HasPrefix(line, "-"):
			out = append(out, removed.Render(line))
		default:
			out = append(out, lipgloss.NewStyle().Foreground(colDim).Render(line))
		}
		shown++
	}
	return strings.Join(out, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// todoSummaryFromInput renders "N/M done" from a todo_write call's input.
func todoSummaryFromInput(input map[string]any) string {
	list, err := tools.ParseTodoList(input["todos"])
	if err != nil {
		return ""
	}
	return tools.TodoSummary(list)
}

// todoContentFromInput formats the card body for a todo_write call.
func todoContentFromInput(input map[string]any) (string, bool) {
	list, err := tools.ParseTodoList(input["todos"])
	if err != nil {
		return "", false
	}
	return tools.FormatTodoList(list), true
}

// summarizeInput builds a short human-readable description of a tool call.
func summarizeInput(name string, input map[string]any) string {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := input[k]; ok {
				s := strings.TrimSpace(fmt.Sprint(v))
				if s != "" && s != "<nil>" {
					if len(s) > 120 {
						s = s[:117] + "..."
					}
					return strings.ReplaceAll(firstLine(s), "\t", " ")
				}
			}
		}
		return ""
	}
	switch name {
	case "bash":
		return get("command")
	case "ask_user":
		return get("question")
	case "todo_write":
		return todoSummaryFromInput(input)
	case "read_file", "write_file", "edit_file", "delete_file":
		return get("path")
	case "search_files":
		if p := get("pattern"); p != "" {
			if dir := get("path"); dir != "" && dir != "." {
				return fmt.Sprintf("%q in %s", p, dir)
			}
			return fmt.Sprintf("%q", p)
		}
	case "list_dir":
		return get("path")
	}
	if rest := get("name", "path", "command", "query"); rest != "" {
		return rest
	}
	return ""
}
