package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Ask-user flow: the agent pauses mid-turn and the model's question is shown
// as a menu. The user picks an option (or types a custom answer); the choice
// is delivered back over m.pendingAsk and becomes the tool result, so the
// model continues the same turn.

// optionsAsStrings normalizes the options argument (JSON []any or []string).
func optionsAsStrings(v any) []string {
	var out []string
	switch opts := v.(type) {
	case []string:
		out = opts
	case []any:
		for _, item := range opts {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// handleAsk installs a pending question from the agent.
func (m *Model) handleAsk(question string, input map[string]any, answer chan string) {
	m.pendingAsk = answer
	m.askQuestion = question
	m.askOptions = optionsAsStrings(input["options"])
	if len(m.askOptions) > 6 {
		m.askOptions = m.askOptions[:6]
	}
	m.askIndex = 0
	m.askCustom = false
	m.status = "question for you"
}

// answerAsk delivers the user's answer and clears the question state.
func (m *Model) answerAsk(answer string) {
	if m.pendingAsk != nil {
		m.pendingAsk <- answer
	}
	m.clearAsk()
	if m.busy {
		m.status = "working"
	} else {
		m.status = "ready"
	}
	m.refreshView()
}

func (m *Model) clearAsk() {
	m.pendingAsk = nil
	m.askQuestion = ""
	m.askOptions = nil
	m.askIndex = 0
	m.askCustom = false
	m.connectInput.Blur()
	m.connectInput.Reset()
}

// updateAskKey handles keystrokes while a question is on screen.
func (m *Model) updateAskKey(key tea.KeyMsg) (tea.Cmd, bool) {
	keyStr := key.String()
	if m.askCustom {
		switch keyStr {
		case "esc":
			m.askCustom = false
			return nil, true
		case "enter":
			if value := strings.TrimSpace(m.connectInput.Value()); value != "" {
				m.answerAsk(value)
				return nil, true
			}
		}
		var cmd tea.Cmd
		m.connectInput, cmd = m.connectInput.Update(key)
		return cmd, true
	}
	total := len(m.askOptions) + 1 // +1 for the "type a custom answer" row
	switch keyStr {
	case "esc":
		m.answerAsk("The user dismissed this question. Proceed with your best judgment or ask differently.")
		return nil, true
	case "up", "shift+tab", "k":
		m.askIndex = mod(m.askIndex-1, total)
	case "down", "tab", "j":
		m.askIndex = mod(m.askIndex+1, total)
	case "enter":
		m.pickAskOption()
		return nil, true
	default:
		if keyStr >= "1" && keyStr <= "9" {
			n := int(keyStr[0] - '0')
			if n <= len(m.askOptions) {
				m.askIndex = n - 1
				m.pickAskOption()
				return nil, true
			}
		}
	}
	return nil, true
}

func (m *Model) pickAskOption() {
	if m.askIndex < len(m.askOptions) {
		m.answerAsk(m.askOptions[m.askIndex])
		return
	}
	// Last row: type a custom answer.
	m.askCustom = true
	m.connectInput.Reset()
	m.connectInput.SetValue("")
	m.connectInput.Placeholder = "type your answer…"
	m.connectInput.SetWidth(minInt(58, maxInt(20, m.width-24)))
	m.connectInput.Focus()
}

// askView renders the pending question as a centered menu.
func (m *Model) askView() string {
	boxWidth := minInt(76, maxInt(52, m.width-8))
	header := lipgloss.NewStyle().Bold(true).Foreground(colCyan).Render("? mihani has a question")
	question := lipgloss.NewStyle().Foreground(colBright).Render(wrap(m.askQuestion, boxWidth-8))

	rows := make([]string, 0, len(m.askOptions)+1)
	for i, opt := range m.askOptions {
		rows = append(rows, m.askRow(opt, i == m.askIndex))
	}
	rows = append(rows, m.askRow("type a custom answer…", m.askIndex == len(m.askOptions)))

	var body strings.Builder
	body.WriteString(header + "\n\n" + question + "\n\n")
	for _, row := range rows {
		body.WriteString(row + "\n")
	}
	body.WriteString("\n")
	if m.askCustom {
		field := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colCyan).
			Padding(0, 1).
			Width(minInt(60, maxInt(24, boxWidth-8))).
			Render(m.connectInput.View())
		body.WriteString(field + "\n\n")
		body.WriteString(lipgloss.NewStyle().Foreground(colFaint).Render("enter send · esc back to options"))
	} else {
		body.WriteString(lipgloss.NewStyle().Foreground(colFaint).Render("↑↓/1-9 choose · enter select · esc skip question"))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colCyan).
		Padding(1, 2).
		Width(boxWidth).
		Render(body.String())
	return lipgloss.Place(m.width, maxInt(1, m.height), lipgloss.Center, lipgloss.Center, box)
}

func (m *Model) askRow(label string, active bool) string {
	style := lipgloss.NewStyle().Foreground(colDim)
	prefix := "  "
	if active {
		style = lipgloss.NewStyle().Foreground(colCyan).Bold(true)
		prefix = "› "
	}
	return style.Render(prefix + label)
}
