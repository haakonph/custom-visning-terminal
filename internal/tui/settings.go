package tui

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
)

type patternSettings struct {
	active bool
	input  textinput.Model
	notice string
}

func (m *Model) loadJiraPattern() {
	config, err := jira.LoadConfig(jira.ConfigPath())
	if err == nil && config.Pattern != "" {
		m.jiraPattern, err = regexp.Compile(config.Pattern)
	}
	if err != nil {
		m.notice = "Kunne ikke lese Jira-mønster: " + err.Error()
	}
}

func (m Model) editJiraPattern() (tea.Model, tea.Cmd) {
	m.patternSettings.active = true
	m.patternSettings.notice = ""
	m.patternSettings.input = textinput.New()
	m.patternSettings.input.CharLimit = 8192
	m.patternSettings.input.SetWidth(max(1, m.width-8))
	m.patternSettings.input.Prompt = "> "
	m.patternSettings.input.Placeholder = "KAKE-[0-9]{3,5}"
	if m.jiraPattern != nil {
		m.patternSettings.input.SetValue(m.jiraPattern.String())
	}
	return m, m.patternSettings.input.Focus()
}

func (m Model) updatePatternSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "esc":
			m.patternSettings.active = false
			m.patternSettings.input.Blur()
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.patternSettings.input.Value())
			var pattern *regexp.Regexp
			var err error
			if value != "" {
				pattern, err = regexp.Compile(value)
			}
			if err != nil {
				m.patternSettings.notice = "Ugyldig mønster: " + err.Error()
				return m, nil
			}
			path := jira.ConfigPath()
			config, err := jira.LoadConfig(path)
			if err == nil {
				config.Pattern = value
				err = jira.SaveConfig(path, config)
			}
			if err != nil {
				m.patternSettings.notice = "Kunne ikke lagre konfigurasjonen: " + err.Error()
				return m, nil
			}
			m.jiraPattern = pattern
			m.selected = 0
			if m.jira != nil {
				m.jira.config = config
			}
			m.patternSettings.notice = "Lagret"
			m.patternSettings.active = false
			m.patternSettings.input.Blur()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.patternSettings.input, cmd = m.patternSettings.input.Update(msg)
	return m, cmd
}

func (m Model) patternSettingsView() tea.View {
	content := fmt.Sprintf("Jira-mønster\n\nEksempel: KAKE-[0-9]{3,5}\nTom verdi bruker standardmønsteret.\n\n%s\n\n%s\n\nEnter lagre · Esc avbryt\nKonfigurasjon: %s", m.patternSettings.input.View(), clean(m.patternSettings.notice), clean(jira.ConfigPath()))
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "custom-visning · Jira-mønster"
	return view
}
