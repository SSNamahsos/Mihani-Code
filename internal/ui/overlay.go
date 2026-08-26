package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/gitx"
	"github.com/SSNamahsos/Mihani-Code/internal/mcp"
	"github.com/SSNamahsos/Mihani-Code/internal/secrets"
	"github.com/SSNamahsos/Mihani-Code/internal/session"
	"github.com/SSNamahsos/Mihani-Code/internal/tools"
	"github.com/SSNamahsos/Mihani-Code/internal/usage"
)

type overlayItem struct {
	label  string
	detail string
}

func (m *Model) openOverlay(title string, items []overlayItem) {
	m.overlay = title
	m.overlayItems = items
	m.overlayIndex = 0
}

func (m *Model) closeOverlay() {
	m.overlay = ""
	m.overlayItems = nil
	m.overlayIndex = 0
	m.resumeRecords = nil
}

func (m *Model) updateOverlayKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc", "ctrl+c":
		m.closeOverlay()
	case "up", "shift+tab", "k":
		if len(m.overlayItems) > 0 {
			m.overlayIndex = mod(m.overlayIndex-1, len(m.overlayItems))
		}
	case "down", "tab", "j":
		if len(m.overlayItems) > 0 {
			m.overlayIndex = mod(m.overlayIndex+1, len(m.overlayItems))
		}
	case "enter":
		m.selectOverlayItem()
	}
	return nil
}

// selectOverlayItem applies the action bound to the active overlay.
func (m *Model) selectOverlayItem() {
	switch {
	case m.overlay == "Modes":
		if m.overlayIndex < len(modes) {
			m.modeIndex = m.overlayIndex
		}
		m.closeOverlay()

	case strings.HasPrefix(m.overlay, "Models ·"):
		provider := m.cfg.Providers[m.cfg.CurrentProvider]
		if m.overlayIndex < len(provider.Models) {
			m.cfg.CurrentModel = provider.Models[m.overlayIndex]
			_ = m.cfg.Save()
		}
		m.closeOverlay()

	case m.overlay == "Providers":
		names := sortedProviderNames(m.cfg)
		if m.overlayIndex < len(names) {
			m.applyProviderSwitch(names[m.overlayIndex])
		}
		m.closeOverlay()

	case m.overlay == "Resume conversation":
		if m.overlayIndex < len(m.resumeRecords) {
			record := m.resumeRecords[m.overlayIndex]
			m.closeOverlay()
			m.blocks = nil
			m.activeAssistant, m.activeTool = -1, -1
			m.tokens = 0
			if ok := m.restore(record); ok {
				m.appendBlock(&block{kind: blockInfo, content: "resumed session " + shortID(record.ID)})
				if record.Mode != "" {
					for i, mode := range modes {
						if mode.name == record.Mode {
							m.modeIndex = i
							break
						}
					}
				}
			} else {
				m.appendBlock(&block{kind: blockError, content: "could not restore session " + record.ID})
			}
			m.relayout()
		} else {
			m.closeOverlay()
		}

	case m.overlay == "Settings":
		if m.overlayIndex < len(m.overlayItems) {
			label := m.overlayItems[m.overlayIndex].label
			switch {
			case label == "Auto confirm":
				m.cfg.AutoConfirm = !m.cfg.AutoConfirm
				_ = m.cfg.Save()
				m.openOverlay("Settings", m.settingsItems())
				return
			case label == "Reset usage window":
				usage.Reset()
				m.refreshSpend()
				m.openOverlay("Settings", m.settingsItems())
				return
			case strings.HasPrefix(label, "Personal API key · "):
				short := strings.TrimPrefix(label, "Personal API key · ")
				for _, id := range []string{config.BuiltinPrimary, config.BuiltinSecondary} {
					if m.cfg.Providers[id].Label == short {
						m.openKeyEditor(id)
						return
					}
				}
			}
		}
		m.closeOverlay()

	default:
		m.closeOverlay()
	}
}

func (m *Model) overlayView() string {
	boxWidth := minInt(80, maxInt(50, m.width-8))
	rows := make([]string, 0, len(m.overlayItems))
	if len(rows) == 0 && len(m.overlayItems) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(colDim).Render("nothing to show"))
	}
	start := 0
	end := len(m.overlayItems)
	const visible = 12
	if len(m.overlayItems) > visible {
		start = maxInt(0, minInt(len(m.overlayItems)-visible, m.overlayIndex-visible/2))
		end = start + visible
	}
	for i := start; i < end; i++ {
		item := m.overlayItems[i]
		style := lipgloss.NewStyle().Foreground(colDim)
		prefix := "  "
		if i == m.overlayIndex {
			style = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
			prefix = "› "
		}
		line := prefix + item.label
		if item.detail != "" {
			pad := maxInt(2, 34-lipgloss.Width(item.label))
			detailStyle := lipgloss.NewStyle().Foreground(colFaint)
			if i == m.overlayIndex {
				detailStyle = lipgloss.NewStyle().Foreground(colDim)
			}
			line += strings.Repeat(" ", pad) + detailStyle.Render(truncateText(item.detail, boxWidth-40))
		}
		rows = append(rows, style.Render(line))
	}
	hint := lipgloss.NewStyle().Foreground(colFaint).Render("↑↓ navigate · enter select · esc close")
	body := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render(m.overlay),
		"",
		strings.Join(rows, "\n"),
		"",
		hint,
	)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colBorder).
		Padding(1, 2).
		Width(boxWidth).
		Render(body)
	return lipgloss.Place(m.width, maxInt(1, m.height), lipgloss.Center, lipgloss.Center, box)
}

// command executes a slash command and returns any follow-up command.
func (m *Model) command(s string) tea.Cmd {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "/help":
		var rows []string
		for _, item := range commands {
			rows = append(rows, fmt.Sprintf("%-11s %s", item.name, item.description))
		}
		keys := "enter send · ctrl+j newline · tab/shift+tab cycle modes · esc interrupt or clear\n" +
			"↑↓/pgup/pgdn scroll transcript · ctrl+y or /copy copy last reply\n" +
			"[ ] inspect your messages → y copy · f fork from here · r revert to composer\n" +
			"drag-select any text to copy natively (ctrl+shift+c / right-click)\n" +
			"ctrl+c cancel request or quit"
		m.appendBlock(&block{kind: blockInfo, content: "Commands\n" + strings.Join(rows, "\n") + "\n\nKeys\n" + keys})

	case "/clear":
		m.blocks = nil
		m.activeAssistant, m.activeTool = -1, -1
		m.tokens = 0
		m.agent.Reset()
		m.view.SetContent("")

	case "/new":
		m.agent.Close()
		m.agent.Reset()
		m.blocks = nil
		m.activeAssistant, m.activeTool = -1, -1
		m.tokens = 0
		m.sessionID = session.NewID()
		m.appendBlock(&block{kind: blockInfo, content: "started new session " + shortID(m.sessionID)})
		m.relayout()

	case "/resume":
		records, err := session.List()
		if err != nil || len(records) == 0 {
			m.appendBlock(&block{kind: blockInfo, content: "no saved sessions found"})
			return nil
		}
		// Conversations from this folder first; other workspaces stay out of
		// the way but are counted so you know they exist.
		var here, elsewhere []session.Record
		for _, r := range records {
			if r.Workspace == m.root {
				here = append(here, r)
			} else {
				elsewhere = append(elsewhere, r)
			}
		}
		if len(here) == 0 {
			m.appendBlock(&block{kind: blockInfo,
				content: fmt.Sprintf("no previous conversations in %s\n(%d session(s) exist in other folders — open those folders to reach them)",
					shortPath(m.root), len(elsewhere))})
			return nil
		}
		items := make([]overlayItem, 0, len(here))
		for _, r := range here {
			title := firstNonEmptyStr(r.Title, "(untitled)")
			when := r.UpdatedAt.Local().Format("Jan 02 15:04")
			items = append(items, overlayItem{
				label:  truncateText(title, 34),
				detail: fmt.Sprintf("%s · %s/%s", when, m.providerDisplay(r.Provider), r.Model),
			})
		}
		m.resumeRecords = here
		m.openOverlay("Resume conversation", items)

	case "/mode":
		if len(fields) > 1 {
			query := strings.ToLower(fields[1])
			for i, candidate := range modes {
				if strings.HasPrefix(candidate.name, query) {
					m.modeIndex = i
					m.appendBlock(&block{kind: blockInfo, content: "switched to " + candidate.name + " mode"})
					return nil
				}
			}
			m.appendBlock(&block{kind: blockError, content: "unknown mode: " + fields[1] + " (build, plan, research, ask)"})
			return nil
		}
		items := make([]overlayItem, 0, len(modes))
		for i, candidate := range modes {
			mark := " "
			if i == m.modeIndex {
				mark = "●"
			}
			items = append(items, overlayItem{
				label:  mark + " " + candidate.name,
				detail: candidate.description,
			})
		}
		m.openOverlay("Modes", items)

	case "/providers":
		items := make([]overlayItem, 0, len(m.cfg.Providers))
		for _, name := range sortedProviderNames(m.cfg) {
			provider := m.cfg.Providers[name]
			mark := " "
			if name == m.cfg.CurrentProvider {
				mark = "●"
			}
			label := fmt.Sprintf("%d models", len(provider.Models))
			items = append(items, overlayItem{label: mark + " " + provider.Label, detail: label})
		}
		m.openOverlay("Providers", items)

	case "/models":
		provider, ok := m.cfg.Providers[m.cfg.CurrentProvider]
		if !ok || len(provider.Models) == 0 {
			m.appendBlock(&block{kind: blockInfo, content: "no models listed for " + m.cfg.CurrentProvider})
			return nil
		}
		items := make([]overlayItem, 0, len(provider.Models))
		for _, model := range provider.Models {
			mark := " "
			if model == m.cfg.CurrentModel {
				mark = "●"
			}
			items = append(items, overlayItem{label: mark + " " + model, detail: ""})
		}
		m.openOverlay("Models · "+provider.Label, items)

	case "/git":
		what := "status"
		if len(fields) > 1 {
			what = fields[1]
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var out string
		var err error
		if what == "diff" {
			out, err = gitx.Diff(ctx, m.root)
		} else {
			out, err = gitx.Status(ctx, m.root)
		}
		if err != nil {
			m.appendBlock(&block{kind: blockError, content: "git: " + err.Error()})
			return nil
		}
		if out == "" {
			out = "(" + what + " is empty)"
		}
		m.appendBlock(&block{kind: blockInfo, content: truncateText(out, 4000)})

	case "/status":
		mode := currentMode(m.modeIndex)
		m.appendBlock(&block{kind: blockInfo, content: fmt.Sprintf(
			"workspace  %s\nprovider   %s · %s\nmode       %s\nsession    %s",
			shortPath(m.root), m.cfg.ProviderLabel(), m.cfg.CurrentModel, mode.name, m.sessionID)})

	case "/session":
		m.saveSession()
		m.appendBlock(&block{kind: blockInfo, content: "session saved as " + m.sessionID})

	case "/mcp":
		servers := mcp.Discover(m.root)
		if len(servers) == 0 {
			m.appendBlock(&block{kind: blockInfo, content: "no MCP servers configured — add .mihani/mcp.json"})
			return nil
		}
		var b strings.Builder
		b.WriteString("MCP servers:\n")
		for _, server := range servers {
			endpoint := server.Command
			if endpoint == "" {
				endpoint = server.URL
			}
			fmt.Fprintf(&b, "  %-14s %s\n", server.Name, endpoint)
		}
		m.appendBlock(&block{kind: blockInfo, content: strings.TrimRight(b.String(), "\n")})

	case "/undo":
		result, err := tools.Undo(m.root)
		if err != nil {
			m.appendBlock(&block{kind: blockError, content: "undo: " + err.Error()})
			return nil
		}
		m.appendBlock(&block{kind: blockInfo, content: result})

	case "/copy":
		if cmd := m.copyLastReply(); cmd != nil {
			return cmd
		}

	case "/settings":
		m.openOverlay("Settings", m.settingsItems())

	case "/connect":
		m.openConnect()

	case "/quit", "/exit":
		m.quitting = true
		return tea.Quit

	default:
		m.appendBlock(&block{kind: blockError, content: "unknown command: " + fields[0] + " — try /help"})
	}
	return nil
}

// settingsItems builds the /settings overlay. Provider credentials are
// deliberately absent: keys are never displayed anywhere in the UI.
func (m *Model) settingsItems() []overlayItem {
	auto := "off — dangerous tools ask first"
	if m.cfg.AutoConfirm {
		auto = "on — tools run without asking"
	}
	reset := usage.NextReset(m.cfg.CurrentProvider)
	resetLabel := "—"
	if !reset.IsZero() {
		resetLabel = time.Until(reset).Round(time.Minute).String()
	}
	builtin := m.cfg.IsBuiltinProvider(m.cfg.CurrentProvider)
	used := "$0.00"
	if builtin {
		used = fmt.Sprintf("$%.2f · oldest entry clears in %s", m.spend, resetLabel)
	} else {
		used = fmt.Sprintf("$%.2f tracked — not capped (your own credentials)", m.spend)
	}
	items := []overlayItem{
		{label: "Auto confirm", detail: auto + "  (enter toggles)"},
		{label: "Reset usage window", detail: "clear today's spend record  (enter resets)"},
		{label: "Daily budget", detail: fmt.Sprintf("$%.2f on Mihani built-in endpoints only", m.cfg.Budget())},
		{label: "Used (24h)", detail: used},
	}
	// Personal keys: the user's own credentials for a built-in endpoint, used
	// automatically when the shared daily credit runs out.
	for _, id := range []string{config.BuiltinPrimary, config.BuiltinSecondary} {
		p := m.cfg.Providers[id]
		detail := config.MaskedKey(p.PersonalKey) + "  (enter to set/clear)"
		if p.PersonalKey != "" {
			detail += " · auto-fallback on"
		}
		items = append(items, overlayItem{label: "Personal API key · " + p.Label, detail: detail})
	}
	items = append(items,
		overlayItem{label: "Max iterations", detail: fmt.Sprintf("%d tool loops per turn", effectiveIterations(m.cfg))},
		overlayItem{label: "Context window", detail: fmt.Sprintf("%d tokens", m.cfg.ContextWindow)},
		overlayItem{label: "Max output tokens", detail: fmt.Sprint(m.cfg.MaxTokens)},
	)
	return items
}

// openKeyEditor starts the personal-key prompt for a provider.
func (m *Model) openKeyEditor(providerID string) {
	m.closeOverlay()
	m.keyEditOpen = true
	m.keyEditTarget = providerID
	m.connectInput.Reset()
	m.connectInput.SetValue(m.cfg.Providers[providerID].PersonalKey)
	m.connectInput.Placeholder = "paste your key — submit empty to remove"
	m.connectInput.SetWidth(minInt(58, maxInt(20, m.width-20)))
	m.connectInput.Focus()
}

func (m *Model) closeKeyEditor() {
	m.keyEditOpen = false
	m.keyEditTarget = ""
	m.connectInput.Blur()
	m.connectInput.Reset()
}

// updateKeyEditor handles keystrokes inside the personal-key prompt.
func (m *Model) updateKeyEditor(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.closeKeyEditor()
		return nil
	case "enter":
		value := strings.TrimSpace(m.connectInput.Value())
		label := m.keyEditTarget
		p := m.cfg.Providers[label]
		p.PersonalKey = value
		m.cfg.Providers[label] = p
		if err := m.cfg.Save(); err != nil {
			m.notify("Could not save: " + err.Error())
		} else if value == "" {
			m.notify("Personal key removed")
		} else {
			secrets.Register(value)
			m.notify("Personal key saved for " + p.Label + " — auto-fallback armed")
		}
		m.agent.Cfg = m.cfg
		m.closeKeyEditor()
		return nil
	}
	var cmd tea.Cmd
	m.connectInput, cmd = m.connectInput.Update(key)
	return cmd
}

// keyEditorView renders the personal-key entry modal.
func (m *Model) keyEditorView() string {
	boxWidth := minInt(70, maxInt(50, m.width-8))
	p := m.cfg.Providers[m.keyEditTarget]
	body := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("Personal API key · "+p.Label),
		lipgloss.NewStyle().Foreground(colFaint).Render(
			"Your own key for this endpoint. When the shared daily credit is\nexhausted, Mihani switches to it automatically. Stored only in\nyour local config file and never displayed again."),
		"",
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent).
			Padding(0, 1).
			Width(boxWidth-8).
			Render(m.connectInput.View()),
		"",
		lipgloss.NewStyle().Foreground(colFaint).Render("enter save · esc cancel"),
	)
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colAccent).
		Padding(1, 3).
		Width(boxWidth).
		Render(body)
	return lipgloss.Place(m.width, maxInt(1, m.height), lipgloss.Center, lipgloss.Center, box)
}

// providerDisplay maps a stored provider id to its public-facing name,
// resolving renamed built-ins so endpoint identifiers never surface.
func (m *Model) providerDisplay(id string) string {
	if replacement, ok := config.LegacyProviderID(id); ok {
		id = replacement
	}
	if p, ok := m.cfg.Providers[id]; ok && p.Label != "" {
		return p.Label
	}
	return "Mihani Code"
}

// switchProvider applies a provider selection from the overlay.
func (m *Model) applyProviderSwitch(name string) {
	m.cfg.CurrentProvider = name
	if models := m.cfg.Providers[name].Models; len(models) > 0 {
		m.cfg.CurrentModel = models[0]
	}
	_ = m.cfg.Save()
	m.agent.Cfg = m.cfg
	m.refreshSpend()
}

func effectiveIterations(cfg config.Config) int {
	if cfg.MaxIterations > 0 {
		return cfg.MaxIterations
	}
	return 40
}

func sortedProviderNames(cfg config.Config) []string {
	names := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func truncateText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func mod(a, n int) int { return (a%n + n) % n }
