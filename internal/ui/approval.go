package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// approvalOverlay renders the permission modal centered on screen.
func (m *Model) approvalOverlay() string {
	boxWidth := minInt(78, maxInt(50, m.width-8))
	preview := m.approvalPreview
	if strings.TrimSpace(preview) == "" {
		preview = "This action will modify your workspace."
	}
	if len(preview) > 2400 {
		preview = clipText(preview, 2400) + "\n… preview truncated"
	}

	detail := ""
	if m.approvalDetail != "" {
		detail = lipgloss.NewStyle().Foreground(colBright).Render(m.approvalDetail) + "\n\n"
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(colAmber).Render("⚠ Permission required"),
		"",
		lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Foreground(colFaint).Render("Mihani wants to run "),
			lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render(m.approvalTool),
		),
		"",
		detail+colorizeDiff(indentBlock(preview, 2), boxWidth-6),
		"",
		m.approvalHints(),
	)
	box := lipgloss.NewStyle().
		Border(boxBorderDouble()).
		BorderForeground(colAmber).
		Padding(1, 3).
		Width(boxWidth).
		Render(body)
	return lipgloss.Place(m.width, maxInt(1, m.height), lipgloss.Center, lipgloss.Center, box)
}

func (m *Model) approvalHints() string {
	hint := func(key, label string, color lipgloss.Color) string {
		return lipgloss.NewStyle().Bold(true).Foreground(color).Render("["+key+"] ") +
			lipgloss.NewStyle().Foreground(colDim).Render(label)
	}
	parts := []string{
		hint("y", "approve once", colGreen),
		hint("n", "deny", colRed),
		hint("a", "always allow", colAmber),
		hint("esc", "deny", colRed),
	}
	return strings.Join(parts, "   ")
}

// clipText truncates to n characters without splitting a rune.
func clipText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
