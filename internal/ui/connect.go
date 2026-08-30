package ui

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/providers"
	"github.com/SSNamahsos/Mihani-Code/internal/secrets"
)

var connectPlaceholders = [3]string{
	"Provider ID, e.g. local-qwen",
	"Base URL, e.g. https://api.example.com/v1",
	"API key (leave blank for local providers)",
}

var connectLabels = [3]string{"Provider ID", "Base URL", "API key"}

func (m *Model) openConnect() {
	m.closeOverlay()
	m.connectOpen = true
	m.connecting = false
	m.connectField = 0
	m.connectFields = [3]string{"", "", ""}
	m.connectName, m.connectURL = "", ""
	m.connectError = ""
	m.connectInput.Reset()
	m.connectInput.Placeholder = connectPlaceholders[0]
	m.connectInput.SetWidth(minInt(58, maxInt(20, m.width-20)))
	m.connectInput.Focus()
}

func (m *Model) closeConnect() {
	m.connectOpen = false
	m.connecting = false
	m.connectField = 0
	m.connectFields = [3]string{"", "", ""}
	m.connectError = ""
	m.connectInput.Blur()
	m.connectInput.Reset()
}

func (m *Model) updateConnectKey(key tea.KeyMsg) tea.Cmd {
	if m.connecting {
		return nil
	}
	switch key.String() {
	case "esc":
		m.closeConnect()
		return nil
	case "tab", "down":
		m.saveConnectField()
		m.connectField = mod(m.connectField+1, len(connectLabels))
		m.loadConnectField()
		return nil
	case "shift+tab", "up":
		m.saveConnectField()
		m.connectField = mod(m.connectField-1, len(connectLabels))
		m.loadConnectField()
		return nil
	case "enter":
		m.saveConnectField()
		if m.connectField < len(connectLabels)-1 {
			m.connectField++
			m.loadConnectField()
			return nil
		}
		return m.startConnect()
	}
	var cmd tea.Cmd
	m.connectInput, cmd = m.connectInput.Update(key)
	return cmd
}

func (m *Model) saveConnectField() {
	m.connectFields[m.connectField] = strings.TrimSpace(m.connectInput.Value())
}

func (m *Model) loadConnectField() {
	m.connectInput.Reset()
	m.connectInput.SetValue(m.connectFields[m.connectField])
	m.connectInput.Placeholder = connectPlaceholders[m.connectField]
	m.connectInput.SetWidth(minInt(58, maxInt(20, m.width-20)))
	m.connectInput.Focus()
}

// startConnect validates the form and discovers models from the endpoint.
func (m *Model) startConnect() tea.Cmd {
	name := m.connectFields[0]
	base := m.connectFields[1]
	key := m.connectFields[2]
	switch {
	case name == "":
		m.connectError = "a provider id is required"
		return nil
	case base == "" || !strings.Contains(base, "://"):
		m.connectError = "enter a valid http(s) base URL"
		return nil
	}
	m.connectName = name
	m.connectURL = strings.TrimRight(base, "/")
	m.connectError = ""
	m.connecting = true
	client := &http.Client{Timeout: 15 * time.Second}
	return func() tea.Msg {
		models, err := providers.DiscoverModels(client, m.connectURL, key)
		return modelsMsg{models: models, err: err}
	}
}

// finishConnect persists a discovered provider or keeps the form open on error.
func (m *Model) finishConnect(x modelsMsg) tea.Cmd {
	m.connecting = false
	if x.err != nil {
		m.connectError = x.err.Error()
		m.connectOpen = true
		return nil
	}
	name := m.connectName
	provider := providers.NormalizeProvider(name, m.connectURL, m.connectFields[2], x.models)
	if key := strings.TrimSpace(m.connectFields[2]); key != "" {
		// The runtime APIKey field is never persisted; the only key field that
		// survives a save is PersonalKey, which Key() falls back to.
		provider.PersonalKey = key
		secrets.Register(key)
	}
	if m.cfg.Providers == nil {
		m.cfg.Providers = map[string]config.Provider{}
	}
	m.cfg.Providers[name] = provider
	m.cfg.CurrentProvider = name
	if len(x.models) > 0 {
		m.cfg.CurrentModel = x.models[0]
	}
	err := m.cfg.Save()
	m.closeConnect()
	m.agent.Cfg = m.cfg
	switch {
	case err != nil:
		m.appendBlock(&block{kind: blockError, content: fmt.Sprintf("connected %s but could not save config: %v", name, err)})
	case len(x.models) == 0:
		m.appendBlock(&block{kind: blockInfo, content: fmt.Sprintf("connected %s — no models were listed by the endpoint", name)})
	default:
		m.appendBlock(&block{kind: blockInfo, content: fmt.Sprintf("connected %s — %d models available", name, len(x.models))})
	}
	m.relayout()
	return nil
}

// refreshResult carries one provider's re-discovery outcome.
type refreshResult struct {
	models []string
	err    string
}

type refreshMsg struct {
	results map[string]refreshResult
}

// refreshProvidersCmd re-discovers models for user-added providers in the
// background and reports back over refreshMsg.
func (m *Model) refreshProvidersCmd(targets []string) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 15 * time.Second}
		results := make(map[string]refreshResult, len(targets))
		for _, name := range targets {
			p := m.cfg.Providers[name]
			models, err := providers.DiscoverModels(client, p.BaseURL, m.cfg.Key(name))
			if err != nil {
				results[name] = refreshResult{err: err.Error()}
				continue
			}
			results[name] = refreshResult{models: models}
		}
		return refreshMsg{results: results}
	}
}

// finishRefresh applies refreshed model lists, persists them, and reports.
func (m *Model) finishRefresh(x refreshMsg) tea.Cmd {
	names := make([]string, 0, len(x.results))
	for name := range x.results {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := []string{}
	for _, name := range names {
		res := x.results[name]
		if res.err != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", name, res.err))
			continue
		}
		p := m.cfg.Providers[name]
		p.Models = res.models
		m.cfg.Providers[name] = p
		switch {
		case len(res.models) == 0:
			lines = append(lines, name+": endpoint listed no models")
		default:
			lines = append(lines, fmt.Sprintf("%s: %d models", name, len(res.models)))
		}
		if name == m.cfg.CurrentProvider && len(res.models) > 0 && !modelInList(res.models, m.cfg.CurrentModel) {
			m.cfg.CurrentModel = res.models[0]
			lines = append(lines, "active model reset to "+res.models[0])
		}
	}
	if err := m.cfg.Save(); err != nil {
		m.appendBlock(&block{kind: blockError, content: "refreshed but could not save config: " + err.Error()})
		return nil
	}
	m.agent.Cfg = m.cfg
	m.appendBlock(&block{kind: blockInfo, content: "model refresh\n" + strings.Join(lines, "\n")})
	m.relayout()
	return nil
}

func modelInList(models []string, model string) bool {
	for _, m := range models {
		if m == model {
			return true
		}
	}
	return false
}

// refreshLocalProviderModels re-fetches the active provider's model list when
// its endpoint is local (ollama etc.), so the app shows the models actually
// installed instead of a stale stored list. Remote providers keep their list;
// a stopped local server keeps the stored list.
func (m *Model) refreshLocalProviderModels() {
	p, ok := m.cfg.Providers[m.cfg.CurrentProvider]
	if !ok || p.BaseURL == "" {
		return
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		return
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return
	}
	client := &http.Client{Timeout: 2 * time.Second}
	models, err := providers.DiscoverModels(client, p.BaseURL, m.cfg.Key(m.cfg.CurrentProvider))
	if err != nil || len(models) == 0 {
		return
	}
	p.Models = models
	m.cfg.Providers[m.cfg.CurrentProvider] = p
	if !modelInList(models, m.cfg.CurrentModel) {
		m.cfg.CurrentModel = models[0]
	}
	_ = m.cfg.Save()
}

func (m *Model) connectView() string {
	boxWidth := minInt(72, maxInt(50, m.width-8))
	rows := make([]string, 0, len(connectLabels))
	for i, label := range connectLabels {
		labelStyle := lipgloss.NewStyle().Foreground(colFaint)
		borderColor := colBorder
		fieldStyle := lipgloss.NewStyle().Foreground(colDim)
		var inner string
		if i == m.connectField {
			labelStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
			borderColor = colAccent
			fieldStyle = lipgloss.NewStyle().Foreground(colBright)
			inner = m.connectInput.View()
		} else {
			value := m.connectFields[i]
			if value == "" {
				value = connectPlaceholders[i]
				fieldStyle = lipgloss.NewStyle().Foreground(colFaint)
			}
			inner = fieldStyle.Render(value)
		}
		rows = append(rows,
			labelStyle.Render(label),
			lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				Padding(0, 1).
				Width(boxWidth-8).
				Render(inner),
		)
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("Connect provider"),
		lipgloss.NewStyle().Foreground(colFaint).Render("Add an OpenAI-compatible endpoint and discover its models."),
		"",
		strings.Join(rows, "\n"),
	)

	statusLine := lipgloss.NewStyle().Foreground(colFaint).Render("enter connect · tab next field · esc cancel")
	if m.connecting {
		statusLine = lipgloss.NewStyle().Foreground(colAmber).
			Render(fmt.Sprintf("%s discovering models from %s…", m.spinnerGlyph(), m.connectURL))
	}
	if m.connectError != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "",
			lipgloss.NewStyle().Foreground(colRed).Render("✗ "+m.connectError))
	}
	body = lipgloss.JoinVertical(lipgloss.Left, body, "", statusLine)

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colAccent).
		Padding(1, 3).
		Width(boxWidth).
		Render(body)
	return lipgloss.Place(m.width, maxInt(1, m.height), lipgloss.Center, lipgloss.Center, box)
}
