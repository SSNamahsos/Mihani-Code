package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
	"github.com/SSNamahsos/Mihani-Code/internal/session"
	"github.com/SSNamahsos/Mihani-Code/internal/usage"
)

const maxComposerLines = 8

// relayout recomputes component sizes from the terminal dimensions.
func (m *Model) relayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.view.Width = maxInt(20, m.width-2)
	m.view.Height = maxInt(3, m.height-1-m.composerHeight()-1-m.paletteHeight())
	if m.updateOpen {
		w := m.updateBoxWidth() - 6
		if w < 20 {
			w = 20
		}
		m.updateVp.Width = w
		if h := m.height - 13; h > 3 {
			m.updateVp.Height = h
		}
	}
	for _, b := range m.blocks {
		b.invalidate()
	}
	m.refreshView()
}

func (m *Model) composerHeight() int {
	return m.input.Height() + 2 // rounded border adds two rows
}

// resizeComposer grows the editor with its content up to a cap. Both explicit
// newlines and soft-wrapped overflow count, so long typing wraps upward (to
// the next row) instead of running off the right edge.
func (m *Model) resizeComposer() {
	lines := wrappedLineCount(m.input.Value(), maxInt(12, m.width-8))
	if lines > maxComposerLines {
		lines = maxComposerLines
	}
	if lines < 1 {
		lines = 1
	}
	if m.input.Height() != lines {
		m.input.SetHeight(lines)
		m.relayout()
	}
}

// wrappedLineCount estimates how many terminal rows value occupies, accounting
// for soft wrapping at the given width per logical line.
func wrappedLineCount(value string, width int) int {
	if width < 8 {
		width = 8
	}
	total := 0
	for _, line := range strings.Split(value, "\n") {
		display := lipgloss.Width(strings.TrimRight(line, " "))
		rows := 1 + display/width
		if display%width == 0 && display > 0 {
			rows = display / width
		}
		total += rows
	}
	return total
}

func (m *Model) paletteHeight() int {
	if !m.showCommands() {
		return 0
	}
	n := len(m.filteredCommands())
	if n == 0 {
		return 2 // "no matches" box
	}
	if n > 6 {
		n = 6
	}
	return n + 2
}

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return lipgloss.NewStyle().Foreground(colDim).Render("starting mihani…")
	}
	switch {
	case m.connectOpen:
		return m.connectView()
	case m.keyEditOpen:
		return m.keyEditorView()
	case m.updateOpen:
		return m.updateView()
	case m.overlay != "":
		return m.overlayView()
	case m.pendingApproval != nil:
		return m.approvalOverlay()
	case m.pendingAsk != nil:
		return m.askView()
	}
	var b strings.Builder
	b.WriteString(m.headerRow())
	b.WriteString("\n")
	if len(m.blocks) == 0 && !m.busy && m.queued == "" {
		b.WriteString(m.welcome())
	} else {
		b.WriteString(m.view.View())
	}
	b.WriteString("\n")
	if m.showCommands() {
		b.WriteString(m.commandPalette())
		b.WriteString("\n")
	}
	b.WriteString(m.composerBox())
	b.WriteString("\n")
	b.WriteString(m.statusRow())
	return b.String()
}

func (m *Model) headerRow() string {
	logo := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("◆ mihani")
	version := lipgloss.NewStyle().Foreground(colFaint).Render(" " + m.version)
	if m.updateReady() {
		version += lipgloss.NewStyle().Foreground(colAmber).Render(" · ⟳ " + m.updateLatest.Tag)
	}
	modelLabel := fmt.Sprintf("%s · %s", m.cfg.ProviderLabel(), m.cfg.CurrentModel)
	if effort := m.cfg.CurrentEffort(); effort != "" {
		modelLabel += " · effort:" + effort
	}
	model := lipgloss.NewStyle().Foreground(colDim).Render(modelLabel)
	mode := currentMode(m.modeIndex)
	pill := lipgloss.NewStyle().Bold(true).
		Background(mode.color).
		Foreground(lipgloss.Color("#101216")).
		Padding(0, 1).
		Render(strings.ToUpper(mode.name))
	right := pill + " " + model
	left := logo + version
	gap := maxInt(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) composerBox() string {
	mode := currentMode(m.modeIndex)
	return lipgloss.NewStyle().
		Border(boxBorder()).
		BorderForeground(mode.color).
		Padding(0, 1).
		Width(maxInt(10, m.width-2)).
		Render(m.input.View())
}

func (m *Model) statusRow() string {
	var left string
	switch {
	case !m.toastUntil.IsZero() && time.Now().Before(m.toastUntil):
		left = lipgloss.NewStyle().Foreground(colGreen).Bold(true).
			Render("✓ " + m.toast)
	case m.pendingApproval != nil:
		left = lipgloss.NewStyle().Foreground(colAmber).Render(
			fmt.Sprintf("%s approve %s? [y]es [n]o [a]lways", m.spinnerGlyph(), m.approvalTool))
	case m.busy:
		label := m.activity
		if label == "" {
			label = "thinking"
		}
		// While the model is reasoning, surface the effort state (off =
		// provider default) for models that actually expose one.
		if label == "thinking" && len(agent.EffortLevels(m.cfg.CurrentModel)) > 1 {
			if effort := m.cfg.CurrentEffort(); effort == "" {
				label += " · effort:off"
			} else {
				label += " · effort:" + effort
			}
		}
		left = lipgloss.NewStyle().Foreground(colAccent).
			Render(fmt.Sprintf("%s %s", m.spinnerGlyph(), label))
	case m.queued != "":
		left = lipgloss.NewStyle().Foreground(colCyan).Render("… message queued")
	default:
		left = lipgloss.NewStyle().Foreground(colFaint).
			Render("/ seasons · / commands · tab mode · ↑↓/pgup scroll · esc stop")
	}
	window := m.cfg.ContextWindow
	if window <= 0 {
		window = 200_000
	}
	pct := float64(m.tokens) / float64(window) * 100
	right := fmt.Sprintf("%s · %sk tokens (%.0f%%) · %s",
		shortPath(m.root), formatK(m.tokens), pct, m.status)
	spendPart := m.spendLabel()
	if spendPart != "" {
		right = spendPart + " · " + right
	}
	return lipgloss.JoinHorizontal(lipgloss.Left,
		left,
		strings.Repeat(" ", maxInt(1, m.width-lipgloss.Width(stripANSI(left))-lipgloss.Width(stripANSI(right)))),
		lipgloss.NewStyle().Foreground(colFaint).Render(right),
	)
}

// spendLabel renders the live daily budget meter for the active provider.
// It appears only while using built-in Mihani endpoints — custom providers
// are not subject to the shared credit cap. Personal-key turns meter in cyan
// without a cap since it is the user's own quota.
func (m *Model) spendLabel() string {
	budget := m.cfg.BudgetEnforced(m.cfg.CurrentProvider)
	if budget <= 0 {
		return ""
	}
	if m.keyKind == usage.Personal {
		return lipgloss.NewStyle().Foreground(colCyan).
			Render(fmt.Sprintf("$%.2f personal", m.personalSpend))
	}
	style := lipgloss.NewStyle()
	switch {
	case m.spend >= budget:
		style = style.Foreground(colRed).Bold(true)
	case m.spend >= budget*0.8:
		style = style.Foreground(colAmber)
	default:
		style = style.Foreground(colGreen)
	}
	return style.Render(fmt.Sprintf("$%.2f/$%.2f", m.spend, budget))
}

func stripANSI(s string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range s {
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		if r == '\x1b' {
			inEscape = true
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func formatK(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1f", float64(n)/1000)
	}
	return fmt.Sprint(n)
}

func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		path = "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func (m *Model) commandPalette() string {
	items := m.filteredCommands()
	rows := make([]string, 0, len(items))
	start := 0
	end := len(items)
	const visible = 6
	if len(items) > visible {
		start = maxInt(0, minInt(len(items)-visible, m.commandIndex-visible/2))
		end = start + visible
	}
	if len(items) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(colDim).Render("  no matching commands"))
	}
	for i := start; i < end; i++ {
		item := items[i]
		style := lipgloss.NewStyle().Foreground(colDim)
		prefix := "  "
		if i == m.commandIndex {
			style = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
			prefix = "› "
		}
		rows = append(rows, style.Render(prefix+fmt.Sprintf("%-11s %s", item.name, item.description)))
	}
	return lipgloss.NewStyle().
		Border(boxBorder()).
		BorderForeground(colBorder).
		Padding(0, 1).
		Width(maxInt(30, m.width-6)).
		Render(strings.Join(rows, "\n"))
}

func (m *Model) welcome() string {
	mode := currentMode(m.modeIndex)
	logo := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("◆ Mihani Code") +
		lipgloss.NewStyle().Foreground(colFaint).Render("  "+m.version)

	// Home page: this launch is a brand-new conversation (season).
	seasonHeader := lipgloss.NewStyle().Bold(true).Foreground(colBright).Render("NEW SEASON") +
		lipgloss.NewStyle().Foreground(colFaint).Render("  ·  "+shortID(m.sessionID))
	infoRows := []string{
		seasonHeader,
		lipgloss.NewStyle().Foreground(colFaint).Render("workspace ") + shortPath(m.root),
		lipgloss.NewStyle().Foreground(colFaint).Render("provider  ") + m.cfg.ProviderLabel() + " · " + m.cfg.CurrentModel,
		lipgloss.NewStyle().Foreground(mode.color).Render("mode      ") + mode.name + " — " + mode.description,
	}

	// Count past seasons in this folder so /seasons feels discoverable.
	seasonHint := "no previous seasons here yet"
	if records, err := session.List(); err == nil && len(records) > 0 {
		n := 0
		for _, r := range records {
			if r.Workspace == m.root {
				n++
			}
		}
		if n > 0 {
			seasonHint = fmt.Sprintf("%d previous season(s) in this folder — type /seasons to switch", n)
		} else {
			seasonHint = "you have seasons in other folders — type /seasons"
		}
	}

	keys := []string{
		lipgloss.NewStyle().Foreground(colText).Render("/ seasons") + lipgloss.NewStyle().Foreground(colFaint).Render("   open a past conversation from this folder"),
		lipgloss.NewStyle().Foreground(colText).Render("/ commands") + lipgloss.NewStyle().Foreground(colFaint).Render("   browse everything mihani can do"),
		lipgloss.NewStyle().Foreground(colText).Render("tab modes") + lipgloss.NewStyle().Foreground(colFaint).Render("     build, plan, research, or ask"),
		lipgloss.NewStyle().Foreground(colText).Render("click a message") + lipgloss.NewStyle().Foreground(colFaint).Render("  open revert / fork / copy actions"),
		lipgloss.NewStyle().Foreground(colText).Render("select text") + lipgloss.NewStyle().Foreground(colFaint).Render("    drag to select · release copies it"),
	}

	examples := []string{
		"❯ explain what this project does",
		"❯ find and fix the failing test",
		"❯ refactor config loading into its own package",
		"❯ add tests to the biggest package — watch the todo list update",
	}

	section := func(title string, rows []string) string {
		out := lipgloss.NewStyle().Bold(true).Foreground(colBright).Render(title)
		for _, row := range rows {
			out += "\n  " + row
		}
		return out
	}

	rows := []string{
		logo,
		"",
		lipgloss.NewStyle().Foreground(colDim).Render("Terminal workspace for building software with AI agents."),
	}
	if m.updateReady() {
		rows = append(rows,
			"",
			lipgloss.NewStyle().Bold(true).Foreground(colAmber).Render(
				"✦ "+m.updateLatest.Tag+" is available — type /update to see what's new and install it"),
		)
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		append(rows,
			"",
			section("SEASON", infoRows),
			lipgloss.NewStyle().Foreground(colFaint).Render("  "+seasonHint),
			"",
			section("QUICK KEYS", keys),
			"",
			section("TRY", examples),
		)...,
	)
	return lipgloss.Place(maxInt(40, m.view.Width), maxInt(8, m.view.Height),
		lipgloss.Center, lipgloss.Center, body)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
