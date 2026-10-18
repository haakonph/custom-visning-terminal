package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

// Prefixes are consumed only in navigation mode. Editors and modal dialogs own
// their input, including letters that also happen to be shortcut prefixes.
func (m Model) keyboardNavigation() bool {
	return m.detail == nil && !m.settings.active && !m.help &&
		(m.snoozes == nil || (!m.snoozes.active && !m.snoozes.input)) &&
		(m.local == nil || !m.local.choosing) &&
		(m.mattermost == nil || !m.mattermost.active) &&
		(m.mmReader == nil || !m.mmReader.active || (m.mmReader.mode == "" && !m.mmReader.choosing && !m.mmReader.confirmClear)) &&
		(!m.tasks.active || (!m.tasks.editing && !m.tasks.searching)) &&
		(!m.jiraActive || (m.jira != nil && m.jira.mode == "" && !m.jira.help)) &&
		(!m.copilotActive || (m.copilot != nil && !m.copilot.filtering)) && !m.filtering
}

func (m Model) keyboardView() tea.View {
	sequence, notice, action := m.sequence, m.sequenceNotice, m.jiraAction
	m.sequence, m.sequenceNotice, m.jiraAction = "", "", nil
	view := m.View()
	lines := []string{}
	if sequence != "" {
		lines = append(lines, "Tastsekvens: "+sequence+" · "+sequenceHint(sequence)+" · Esc avbryt")
	}
	if notice != "" {
		lines = append(lines, notice)
	}
	if action != nil {
		if action.running {
			lines = append(lines, "Jira: utfører handling…")
		} else if action.loading {
			lines = append(lines, "Jira: henter tilgjengelige overganger… · q QA · u under arbeid · a avklaring · f ferdig · Esc avbryt")
		} else {
			lines = append(lines, "Jira: velg overgang · j/k · Enter · q QA · u under arbeid · a avklaring · f ferdig · Esc avbryt")
			start := max(0, action.selected-max(1, m.height-2)+1)
			for i := start; i < min(len(action.choices), start+max(1, m.height-2)); i++ {
				choice := action.choices[i]
				mark := "  "
				if i == action.selected {
					mark = "> "
				}
				lines = append(lines, fmt.Sprintf("%s%d %s", mark, i+1, clean(choice)))
			}
		}
	}
	if len(lines) > m.height {
		lines = lines[:max(1, m.height)]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(clean(lines[i]), max(1, m.width), "…")
	}
	// Keep the prompt visible even in a small terminal.
	content := strings.Split(view.Content, "\n")
	keep := max(0, m.height-len(lines))
	if len(content) > keep {
		content = content[:keep]
	}
	view.Content = strings.Join(append(content, lines...), "\n")
	return view
}
func sequenceHint(prefix string) string {
	switch prefix {
	case "r":
		return "o oppgave · g PR · t Jira · c omtaler"
	case "v":
		return "o oppgaver · g GitHub · t Jira · a Copilot · c Mattermost"
	case "t":
		return "o legg til i oppgave · n ny oppgave"
	case "e":
		return "e editor · n nettleser · g/t velg relatert system"
	case "eg", "et":
		return "n nettleser"
	case "a":
		return "t Jira · c send melding · d slett oppgave · a arkiver · f flytt Jira · m meg · r fjern meg"
	case "at":
		return "f flytt · m tilordne meg · r fjern ansvarlig"
	}
	return "Esc avbryt"
}

func (m Model) updateKeyboard(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if m.jiraActive && !m.tasks.active && !m.copilotActive && (m.mmReader == nil || !m.mmReader.active) && m.jira != nil && m.jira.detail != nil {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				m.jira.detail = nil
			case "j", "down":
				m.jira.detailOffset = min(m.jira.detailOffset+1, max(0, len(m.jiraDetailLines())-max(1, m.height-1)))
			case "k", "up":
				m.jira.detailOffset = max(0, m.jira.detailOffset-1)
			case "q", "ctrl+c":
				m.cancel()
				return m, tea.Quit, true
			}
			return m, nil, true
		}
	}
	if result, ok := msg.(jiraActionMsg); ok {
		next, cmd := m.finishJiraAction(result)
		return next, cmd, true
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil, false
	}
	k := key.String()
	if m.jiraAction != nil {
		next, cmd := m.jiraActionKey(k)
		return next, cmd, true
	}
	if m.help {
		switch k {
		case "esc", "?":
			m.help = false
		case "j", "down":
			m.helpOffset = min(m.helpOffset+1, max(0, len(strings.Split(helpText, "\n"))-max(1, m.height-1)))
		case "k", "up":
			m.helpOffset = max(0, m.helpOffset-1)
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit, true
		}
		return m, nil, true
	}
	if m.sequence != "" {
		if k == "ctrl+c" {
			m.cancel()
			return m, tea.Quit, true
		}
		if k == "esc" {
			m.sequence = ""
			m.sequenceNotice = ""
			return m, nil, true
		}
		prefix := m.sequence
		m.sequence = ""
		m.sequenceNotice = ""
		next, cmd := m.executeSequence(prefix, k)
		return next, cmd, true
	}
	if !m.keyboardNavigation() {
		return m, nil, false
	}
	m.sequenceNotice = ""
	if m.mmReader != nil && m.mmReader.active {
		switch k {
		case "r", "v", "t", "e", "a", "?", "x", "h", "l", "left", "right":
		case "R":
			_, cmd := m.mmMessage(key)
			return m, cmd, true
		default:
			return m, nil, false
		}
	}
	switch k {
	case "r", "v", "t", "e", "a":
		m.sequence = k
		return m, nil, true
	case "?":
		m.help = true
		m.helpOffset = 0
		return m, nil, true
	case "x":
		m.clearChosen()
		return m, nil, true
	case "esc":
		if m.tasks.active && m.tasks.details {
			m.tasks.details = false
			return m, nil, true
		}
		if m.copilotActive && !m.tasks.active && (m.copilot.detail != nil || m.copilot.diagnosticsOpen) {
			return m, nil, false
		}
		if len(m.chosen) > 0 || len(m.jiraChosen) > 0 {
			m.clearChosen()
			return m, nil, true
		}
	case "h", "l", "left", "right":
		delta := 1
		if k == "h" || k == "left" {
			delta = -1
		}
		if m.mmReader != nil && m.mmReader.active {
			m.mmReader.changeTab(delta)
			return m, nil, true
		}
		if m.jiraActive && !m.tasks.active && !m.copilotActive {
			m.jira.active = (m.jira.active + delta + len(m.jira.tabs)) % len(m.jira.tabs)
			m.jira.custom = ""
			m.jira.selected = 0
			return m, m.refreshJira(), true
		}
		if m.tasks.active {
			m.tasks.moveTask(delta, true)
			return m, nil, true
		}
		if !m.copilotActive {
			m.active = model.View((int(m.active) + delta + int(model.ViewCount)) % int(model.ViewCount))
			m.selected = 0
			return m, nil, true
		}
	case "space", " ":
		if m.jiraActive && !m.tasks.active && !m.copilotActive {
			m.toggleJira()
			return m, nil, true
		}
	case "shift+up", "shift+down", "shift+j", "shift+k", "K":
		delta := 1
		if k == "shift+up" || k == "shift+k" || k == "K" {
			delta = -1
		}
		if m.jiraActive && !m.tasks.active && !m.copilotActive {
			m.markJira()
			m.jira.selected += delta
			m.jira.clamp()
			m.markJira()
			return m, nil, true
		}
		if !m.tasks.active && !m.copilotActive && (m.mmReader == nil || !m.mmReader.active) {
			m.markPR()
			m.selected += delta
			m.clamp()
			m.markPR()
			return m, nil, true
		}
	case "d":
		if m.copilotActive && !m.tasks.active {
			next, cmd := m.updateCopilot(tea.KeyPressMsg{Code: tea.KeyEnter})
			return next, cmd, true
		}
		if m.jiraActive && !m.tasks.active && !m.copilotActive {
			rows := m.jira.filtered()
			if len(rows) > 0 {
				issue := rows[m.jira.selected]
				m.jira.detail = &issue
				m.jira.detailOffset = 0
			}
			return m, nil, true
		}
		if m.tasks.active {
			m.tasks.details = true
			return m, nil, true
		}
	case "A":
		if m.tasks.active {
			m.tasks.archived = !m.tasks.archived
			m.tasks.selectVisible()
			return m, nil, true
		}
	case "R":
		if m.tasks.active {
			return m, nil, true
		}
		if m.jiraActive && !m.tasks.active && !m.copilotActive {
			return m, m.refreshJira(), true
		}
		if m.copilotActive && !m.tasks.active {
			return m, m.refreshCopilot(), true
		}
		if !m.tasks.active {
			if m.snapshot.User == "" && !m.booting {
				m.booting = true
				return m, m.bootstrap(), true
			}
			return m, m.refresh(m.active), true
		}
	}
	return m, nil, false
}
func (m *Model) clearChosen() { m.chosen = map[string]model.PullRequest{}; m.jiraChosen = nil }
func (m *Model) markPR() {
	rows := m.filtered()
	if m.selected >= 0 && m.selected < len(rows) {
		if m.chosen == nil {
			m.chosen = map[string]model.PullRequest{}
		}
		p := rows[m.selected]
		m.chosen[p.Key()] = p
	}
}
func (m *Model) markJira() {
	rows := m.jira.filtered()
	if m.jira.selected >= 0 && m.jira.selected < len(rows) {
		if m.jiraChosen == nil {
			m.jiraChosen = map[string]jira.Issue{}
		}
		r := rows[m.jira.selected]
		m.jiraChosen[r.Key] = r
	}
}
func (m *Model) toggleJira() {
	rows := m.jira.filtered()
	if m.jira.selected >= 0 && m.jira.selected < len(rows) {
		key := rows[m.jira.selected].Key
		if _, ok := m.jiraChosen[key]; ok {
			delete(m.jiraChosen, key)
		} else {
			m.markJira()
		}
	}
}
func (m Model) unavailable() (tea.Model, tea.Cmd) {
	m.sequenceNotice = "Handlingen er ikke tilgjengelig for dette elementet."
	return m, nil
}

func (m Model) executeSequence(prefix, k string) (tea.Model, tea.Cmd) {
	switch prefix {
	case "v":
		return m.viewProvider(k)
	case "r":
		if k == "o" && !m.tasks.active && !m.copilotActive && (m.mmReader == nil || !m.mmReader.active) {
			refs := m.taskReferences()
			for i, task := range m.tasks.items {
				for _, existing := range task.References {
					for _, ref := range refs {
						if existing.Provider == ref.Provider && existing.URL == ref.URL {
							m.tasks.active = true
							m.tasks.archived = task.Archived
							m.tasks.query = ""
							m.tasks.selected = i
							m.tasks.details = true
							return m, nil
						}
					}
				}
			}
			return m.unavailable()
		}
		if k == "g" && m.mmReader != nil && m.mmReader.active {
			return m, m.mmGoPR()
		}
		if k == "t" && !m.jiraActive && !m.tasks.active && !m.copilotActive && (m.mmReader == nil || !m.mmReader.active) {
			return m.navigateRelated()
		}
		if k == "g" && m.jiraActive && !m.tasks.active && (m.mmReader == nil || !m.mmReader.active) {
			return m.navigateRelated()
		}
		if k == "c" && !m.jiraActive && !m.tasks.active && !m.copilotActive && m.mmReader != nil {
			rows := m.filtered()
			if len(rows) > 0 {
				m.mmReader.mentionURL = strings.ToLower(rows[m.selected].URL)
				m.mmReader.mentions = true
				m.mmReader.active = true
				m.mmReader.row = 0
				return m, nil
			}
		}
	case "t":
		if (k == "o" || k == "n") && !m.tasks.active && !m.copilotActive && (m.mmReader == nil || !m.mmReader.active) {
			return m.collectTask(k == "n")
		}
	case "e":
		if k == "g" || k == "t" {
			m.sequence = "e" + k
			return m, nil
		}
		if k == "e" && m.copilotActive && !m.tasks.active && (m.mmReader == nil || !m.mmReader.active) {
			return m.updateCopilot(tea.KeyPressMsg{Code: 'i', Text: "i"})
		}
		if k == "e" && !m.jiraActive && !m.tasks.active && !m.copilotActive && (m.mmReader == nil || !m.mmReader.active) {
			return m, m.openLocal("")
		}
		if k == "n" {
			return m.openKeyboardBrowser()
		}
	case "et":
		if k == "n" {
			return m.openRelatedJira()
		}
	case "eg":
		if k == "n" && !m.tasks.active && !m.copilotActive {
			if m.jiraActive && !m.tasks.active && !m.copilotActive {
				return m.openRelatedPRs()
			}
			return m.openKeyboardBrowser()
		}
	case "a":
		if k == "t" {
			m.sequence = "at"
			return m, nil
		}
		if k == "f" || k == "m" || k == "r" {
			if m.jiraActive && !m.tasks.active && !m.copilotActive {
				return m.startJiraAction(k)
			}
		}
		if k == "c" && !m.jiraActive && !m.tasks.active && !m.copilotActive && (m.mmReader == nil || !m.mmReader.active) {
			return m, m.composeMattermost()
		}
		if k == "a" && m.tasks.active && len(m.tasks.visibleTasks()) > 0 && !m.tasks.loadFailed {
			task := &m.tasks.items[m.tasks.selected]
			task.Archived = !task.Archived
			if err := m.tasks.save(); err != nil {
				task.Archived = !task.Archived
			}
			m.tasks.selectVisible()
			return m, nil
		}
		if k == "d" && m.tasks.active {
			if m.tasks.details {
				return m.updateTasks(tea.KeyPressMsg{Code: 'd', Text: "d"})
			}
			return m.updateTasks(tea.KeyPressMsg{Code: 'D', Text: "D"})
		}
	case "at":
		if k == "f" || k == "m" || k == "r" {
			return m.startJiraAction(k)
		}
	}
	return m.unavailable()
}
func (m Model) viewProvider(k string) (tea.Model, tea.Cmd) {
	// Check availability before changing the visible provider.
	switch k {
	case "o":
		m.tasks.active = true
		m.tasks.selectVisible()
	case "g", "t":
		if (k == "t" && m.jira == nil) || (k == "g" && m.source == nil) {
			return m.unavailable()
		}
		m.tasks.active = false
		if m.mmReader != nil {
			m.mmReader.active = false
		}
		return m.switchProvider(k == "t")
	case "a":
		if m.copilot == nil {
			return m.unavailable()
		}
		m.tasks.active = false
		if m.mmReader != nil {
			m.mmReader.active = false
		}
		return m.switchCopilot()
	case "c":
		if m.mmReader == nil {
			return m.unavailable()
		}
		m.mmReader.active = true
		m.tasks.active = false
	default:
		return m.unavailable()
	}
	if k == "o" && m.mmReader != nil {
		m.mmReader.active = false
	}
	return m, nil
}
func (m Model) openKeyboardBrowser() (tea.Model, tea.Cmd) {
	if m.tasks.active {
		if !m.tasks.details {
			return m.unavailable()
		}
		return m.updateTasks(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	if m.copilotActive && !m.tasks.active {
		return m.unavailable()
	}
	if m.mmReader != nil && m.mmReader.active {
		s := m.mmReader
		posts := s.posts()
		if s.row >= 0 && s.row < len(posts) {
			links := messageLinks(posts[s.row].Message)
			if len(links) > 0 {
				link := links[s.link%len(links)]
				opener := s.open
				if opener == nil {
					opener = m.open
				}
				return m, func() tea.Msg { return openMsg{err: opener(m.ctx, link)} }
			}
		}
		return m.unavailable()
	}
	if m.jiraActive && !m.tasks.active && !m.copilotActive {
		return m.openJiraKeys(m.actionJiraKeys())
	}
	rows, opener, ctx := m.selectedPRs(), m.open, m.ctx
	return m, func() tea.Msg {
		for _, p := range rows {
			if err := opener(ctx, p.URL); err != nil {
				return openMsg{err: err}
			}
		}
		return openMsg{}
	}
}

func (m Model) jiraDetailLines() []string {
	issue := m.jira.detail
	text := fmt.Sprintf("%s · %s\n%s · %s · %s\n\n%s", issue.Key, issue.Title, issue.Status, issue.Priority, issue.Assignee, issue.Description)
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, strings.Split(lipgloss.NewStyle().Width(max(1, m.width)).Render(clean(line)), "\n")...)
	}
	return lines
}
func (m Model) jiraDetailView() tea.View {
	lines := m.jiraDetailLines()
	start := min(m.jira.detailOffset, max(0, len(lines)-1))
	end := min(len(lines), start+max(1, m.height-1))
	content := append([]string{}, lines[start:end]...)
	if m.height > 1 {
		content = append([]string{"Esc tilbake · j/k ↑/↓ bla"}, content...)
	}
	for i := range content {
		content[i] = ansi.Truncate(content[i], max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(content, "\n"))
	view.AltScreen = true
	return view
}
