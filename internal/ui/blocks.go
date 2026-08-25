package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

type blockKind int

const (
	blockUser blockKind = iota
	blockAssistant
	blockTool
	blockInfo
	blockError
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
	width     int // zero means the cached render is stale
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
	case blockTool:
		b.rendered = b.renderTool(w, spinnerChar)
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
	body := wrap(b.content, w-2)
	lines := strings.Split(body, "\n")
	marker := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("❯")
	out := marker + " " + lipgloss.NewStyle().Foreground(colBright).Render(strings.TrimRight(lines[0], " "))
	for _, line := range lines[1:] {
		out += "\n  " + lipgloss.NewStyle().Foreground(colBright).Render(strings.TrimRight(line, " "))
	}
	return out
}

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
		out += "\n" + colorizeDiff(indentBlock(b.content, 3), w)
	}
	return out
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
