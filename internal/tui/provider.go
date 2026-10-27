package tui

import tea "charm.land/bubbletea/v2"

// Switching preserves each provider's state and never waits for a network call.
func (m Model) switchProvider(toJira bool) (tea.Model, tea.Cmd) {
	if toJira == (m.page == pageJira) && !(m.page == pageCopilot) {
		return m, nil
	}
	if toJira {
		if m.jira == nil {
			m.notice = "Set JIRA_BASE_URL and JIRA_TOKEN to enable Jira"
			return m, nil
		}
		m.setPageFlag(pageCopilot, false)
		m.setPageFlag(pageJira, true)
		if m.jira.query == "" {
			return m, m.refreshJira()
		}
		if m.jira.mode != "" {
			return m, m.jira.input.Focus()
		}
	} else {
		if m.prs.Source() == nil {
			if m.jira != nil {
				m.jira.notice = "GitHub er ikke konfigurert. Sett GITHUB_TOKEN eller kjør gh auth login og start appen på nytt."
			}
			return m, nil
		}
		m.setPageFlag(pageCopilot, false)
		m.setPageFlag(pageJira, false)
		if m.filtering {
			return m, m.input.Focus()
		}
	}
	return m, nil
}
