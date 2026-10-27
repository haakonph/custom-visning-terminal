package tui

import tea "charm.land/bubbletea/v2"

// Keyboard routing and internal navigation share named actions. Neither needs
// synthetic key events or a second pass through Update and usage recording.
func (m Model) searchVisible() (tea.Model, tea.Cmd) {
	switch m.visibleProvider() {
	case "task":
		return m.searchTasks()
	case "jira":
		return m.searchJira()
	case "copilot":
		return m.searchCopilot()
	case "github":
		m.filtering = true
		return m, m.input.Focus()
	}
	return m, nil
}
func (m Model) backVisible() (tea.Model, tea.Cmd) {
	if m.snoozes != nil && m.snoozes.active {
		m.snoozes.active = false
		return m, nil
	}
	switch m.visibleProvider() {
	case "task":
		return m.backTasks()
	case "jira":
		return m.backJira()
	case "copilot":
		return m.backCopilot()
	case "mattermost":
		_, cmd := m.backChat()
		return m, cmd
	case "github":
		m.input.SetValue("")
		m.relatedKey = ""
		m.help = false
		m.selected = 0
	}
	return m, nil
}
func (m Model) openVisibleDetails() (tea.Model, tea.Cmd) {
	if m.visibleProvider() == "mattermost" {
		_, cmd := m.openChatThread()
		return m, cmd
	}
	if m.visibleProvider() == "github" {
		rows := m.filtered()
		if m.selected >= 0 && m.selected < len(rows) {
			m.detail = &detailState{id: rows[m.selected].Key()}
			return m, m.loadDetails()
		}
	}
	return m, nil
}
