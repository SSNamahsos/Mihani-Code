package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SSNamahsos/Mihani-Code/internal/update"
)

// updateCheckMsg carries the result of a background GitHub release check.
type updateCheckMsg struct {
	release *update.Release
	err     error
}

// updateChangelogMsg carries the fetched CHANGELOG section for the new release.
type updateChangelogMsg struct {
	text string
}

// updateApplyMsg carries the result of a download/self-update attempt.
type updateApplyMsg struct {
	note string
	err  error
}

// checkUpdateCmd fetches the newest GitHub release in the background and
// reports it over updateCheckMsg. It is a plain HTTPS call to api.github.com —
// no model call, so it costs zero tokens and never touches a provider.
func (m *Model) checkUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		release, err := update.Latest(context.Background())
		return updateCheckMsg{release: release, err: err}
	}
}

// updateReady reports whether a newer release was detected and not dismissed.
func (m *Model) updateReady() bool {
	return m.updateLatest != nil && update.Newer(m.updateLatest.Tag, m.version) && !m.updateDismissed
}

// updateBoxWidth is the update modal's outer width, clamped to the terminal.
func (m *Model) updateBoxWidth() int {
	return minInt(84, maxInt(60, m.width-4))
}

// openUpdateModal shows the update screen: current vs latest, the release
// changelog ("what's new"), where it comes from (GitHub), and install action.
// It returns a cmd that pulls the detailed changelog for a pending update.
func (m *Model) openUpdateModal() tea.Cmd {
	boxWidth := m.updateBoxWidth()
	m.updateVp = viewport.New(maxInt(20, boxWidth-6), maxInt(3, m.height-13))
	m.updateChangelog = ""
	m.updateOpen = true
	m.updateNote = ""
	m.updateBusy = false
	if m.updateReady() {
		m.updateVp.SetContent("Loading what's new…")
		return m.fetchChangelogCmd()
	}
	m.updateVp.SetContent(m.updateChangelogText())
	return nil
}

// fetchChangelogCmd pulls the detailed CHANGELOG section for the new release
// (a raw file read from GitHub — no model, no tokens) and reports it.
func (m *Model) fetchChangelogCmd() tea.Cmd {
	return func() tea.Msg {
		text, _ := update.Changelog(context.Background(), m.updateLatest.Tag)
		return updateChangelogMsg{text: text}
	}
}

func (m *Model) closeUpdate() {
	m.updateOpen = false
	if m.updateCancel != nil {
		m.updateCancel()
		m.updateCancel = nil
	}
}

// updateChangelogText is the scrollable body of the update modal: the fetched
// CHANGELOG section (preferred, detailed) or the release body as fallback,
// plus the source (GitHub repo) and the specific release page.
func (m *Model) updateChangelogText() string {
	if m.updateLatest == nil {
		return "No release information loaded yet."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", m.updateLatest.Tag)
	if name := m.updateLatest.Name; name != "" && name != m.updateLatest.Tag {
		fmt.Fprintf(&b, "%s\n", name)
	}
	b.WriteString("\n")
	text := strings.TrimSpace(m.updateChangelog)
	if text == "" {
		text = strings.TrimSpace(m.updateLatest.Body)
	}
	if text == "" {
		text = "(no release notes were published for this version)"
	}
	b.WriteString(text)
	b.WriteString("\n\nSource: " + update.HomePage)
	b.WriteString("\nRelease: " + m.updateLatest.URL)
	return b.String()
}

func (m *Model) updateScroll(n int) tea.Cmd {
	switch {
	case n < 0:
		m.updateVp.LineUp(-n)
	case n > 0:
		m.updateVp.LineDown(n)
	}
	return nil
}

// applyUpdateCmd runs the download + self-update in the background and reports
// over updateApplyMsg, so the UI stays responsive and esc can cancel.
func (m *Model) applyUpdateCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		note, err := update.Apply(ctx, m.updateLatest)
		return updateApplyMsg{note: note, err: err}
	}
}

// updateKey handles keys while the update modal is open.
func (m *Model) updateKey(x tea.KeyMsg) tea.Cmd {
	key := x.String()
	switch key {
	case "esc":
		if m.updateBusy {
			if m.updateCancel != nil {
				m.updateCancel()
				m.updateBusy = false
				m.updateNote = "Download cancelled."
				m.refreshView()
			}
			return nil
		}
		m.closeUpdate()
		return nil
	case "i", "I", "enter":
		if m.updateBusy {
			return nil
		}
		if !m.updateReady() {
			m.closeUpdate()
			return nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		m.updateCancel = cancel
		m.updateBusy = true
		m.updateNote = "Downloading " + m.updateLatest.Tag + " — keep this window open…"
		m.refreshView()
		return m.applyUpdateCmd(ctx)
	case "o", "O":
		if m.updateLatest != nil {
			if err := update.OpenURL(m.updateLatest.URL); err != nil {
				m.notify("Could not open the browser: " + err.Error())
			}
		}
		return nil
	case "d", "D", "n", "N":
		m.updateDismissed = true
		m.closeUpdate()
		m.notify("Update hidden for this session")
		return nil
	case "up":
		return m.updateScroll(-3)
	case "down":
		return m.updateScroll(3)
	case "pgup":
		return m.updateScroll(-m.updateVp.Height / 2)
	case "pgdown":
		return m.updateScroll(m.updateVp.Height / 2)
	}
	var cmd tea.Cmd
	m.updateVp, cmd = m.updateVp.Update(x)
	return cmd
}

// updateView renders the update modal. It always shows where the update comes
// from (GitHub) and, when a newer release exists, its changelog and how to
// install it.
func (m *Model) updateView() string {
	boxWidth := m.updateBoxWidth()
	title := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("◆ Mihani Code — update")

	var status string
	switch {
	case m.updateLatest == nil:
		if m.updateCheckErr != "" {
			status = lipgloss.NewStyle().Foreground(colRed).Render("check failed: " + m.updateCheckErr)
		} else {
			status = lipgloss.NewStyle().Foreground(colFaint).Render("checking GitHub for the latest release…")
		}
	case m.updateBusy:
		status = lipgloss.NewStyle().Foreground(colAmber).Render(m.updateNote)
	case m.updateNote != "":
		status = lipgloss.NewStyle().Foreground(colGreen).Render(m.updateNote)
	case !update.Newer(m.updateLatest.Tag, m.version):
		status = lipgloss.NewStyle().Foreground(colGreen).Render("You are on the latest version (" + m.version + ")")
	default:
		status = lipgloss.NewStyle().Foreground(colAmber).Bold(true).
			Render(fmt.Sprintf("%s  →  %s   (new version available)", m.version, m.updateLatest.Tag))
	}

	var hint string
	switch {
	case m.updateLatest == nil, m.updateBusy, m.updateNote != "",
		!update.Newer(m.updateLatest.Tag, m.version):
		hint = "esc close" + func() string {
			if m.updateBusy {
				return " · (esc cancels the download)"
			}
			return ""
		}()
	default:
		hint = "i install · o open on github · d not now · ↑↓ scroll · esc close"
	}

	var mid string
	if m.updateLatest != nil {
		mid = m.updateVp.View()
	} else {
		mid = lipgloss.NewStyle().Foreground(colFaint).Render(" ")
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		status,
		"",
		lipgloss.NewStyle().Foreground(colFaint).Render("What's new"),
		mid,
		"",
		lipgloss.NewStyle().Foreground(colFaint).Render(hint),
	)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colBorder).
		Padding(1, 2).
		Width(boxWidth).
		Render(body)
	return lipgloss.Place(m.width, maxInt(1, m.height), lipgloss.Center, lipgloss.Center, box)
}
