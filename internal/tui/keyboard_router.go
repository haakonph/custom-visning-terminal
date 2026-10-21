package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"strings"
)

// The only public navigation entry point. Unknown keys are consumed here, so
// provider-specific implementation helpers cannot expose legacy shortcuts.
func (m Model) keyboardNavigation() bool {
	kind := m.visibleProvider()
	if m.settings.active || (m.local != nil && m.local.choosing) || (m.snoozes != nil && m.snoozes.input) {
		return false
	}
	if m.mattermost != nil && m.mattermost.active {
		return true
	}
	if m.dialog != nil {
		return !m.dialog.searching
	}
	switch kind {
	case "task":
		return !m.tasks.editing && !m.tasks.searching
	case "jira":
		return m.jira != nil && m.jira.mode == ""
	case "copilot":
		return m.copilot != nil && !m.copilot.filtering
	case "mattermost":
		return m.mmReader.mode == "" && !m.mmReader.choosing && !m.mmReader.confirmClear && !m.mmReader.searching
	case "github":
		return !m.filtering
	}
	return true
}
func (m Model) updateKeyboard(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if result, ok := msg.(autoMergeMsg); ok {
		next, cmd := m.finishAutoMerge(result)
		return next, cmd, true
	}
	if result, ok := msg.(copilotReviewRequested); ok {
		m.reviewRequestBusy = false
		m.sequenceNotice = fmt.Sprintf("Ny Copilot-review forespurt for %d PR-er; au henter status", result.count)
		if result.err != nil {
			m.sequenceNotice = result.err.Error()
		}
		return m, nil, true
	}
	if m.internalKey {
		return m, nil, false
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
	if k == "ctrl+c" {
		m.cancel()
		return m, tea.Quit, true
	}
	if m.usageView {
		switch k {
		case "esc":
			m.usageView = false
		case "j", "down":
			m.usageOffset = min(m.usageOffset+1, m.usageMaxOffset())
		case "k", "up":
			m.usageOffset = max(0, m.usageOffset-1)
		case "h", "left":
			m.usagePeriod = (m.usagePeriod + 2) % 3
			m.usageOffset = 0
		case "l", "right":
			m.usagePeriod = (m.usagePeriod + 1) % 3
			m.usageOffset = 0
		}
		return m, nil, true
	}
	if m.local != nil && len(m.local.removePending) > 0 {
		return m.removeLocalKey(k)
	}
	if m.jiraAction != nil {
		next, cmd := m.jiraActionKey(k)
		return next, cmd, true
	}
	if m.sequence != "" {
		if k == "esc" {
			m.sequence = ""
			m.sequenceRefs = nil
			return m, nil, true
		}
		prefix := m.sequence
		m.sequence = ""
		next, cmd := m.executeSequence(prefix, k)
		result := next.(Model)
		if result.sequence == "" {
			result.sequenceRefs = nil
		}
		return result, cmd, true
	}
	if m.setup.provider != "" && !m.settings.active && !m.help {
		next, cmd := m.updateSetup(k)
		return next, cmd, true
	}
	if m.help {
		switch k {
		case "?", "esc":
			m.help = false
		case "j", "down":
			m.helpOffset = min(m.helpOffset+1, max(0, len(strings.Split(helpText, "\n"))-max(1, m.height-1)))
		case "k", "up":
			m.helpOffset = max(0, m.helpOffset-1)
		}
		return m, nil, true
	}
	if m.dialog != nil && m.dialog.searching {
		switch k {
		case "enter":
			m.dialog.searching = false
		case "esc":
			m.dialog.searching = false
			m.dialog.query = ""
		case "backspace":
			r := []rune(m.dialog.query)
			if len(r) > 0 {
				m.dialog.query = string(r[:len(r)-1])
			}
		default:
			m.dialog.query += key.Text
		}
		m.dialog.selected = 0
		return m, nil, true
	}
	if !m.keyboardNavigation() {
		return m, nil, false
	}
	if m.visibleProvider() == "copilot" && m.copilot != nil && m.copilot.diagnosticsOpen && k == "y" {
		m.copilot.notice = "Kopiering sendt til terminalen (krever støtte for OSC 52)."
		return m, tea.SetClipboard(m.copilot.otlpExporterConfig()), true
	}
	if m.mattermost != nil && m.mattermost.active {
		switch k {
		case "a", "e":
			m.sequence = k
			m.sequenceRefs = nil
			return m, nil, true
		case "j", "k", "up", "down", "esc":
			handled, cmd := m.mattermostMessage(msg)
			return m, cmd, handled
		case "?":
			m.help = true
			return m, nil, true
		}
		return m, nil, true
	}
	switch k {
	case "r", "v", "t", "e", "a":
		m.sequence = k
		m.sequenceRefs = m.currentReferences()
		return m, nil, true
	case "?":
		m.help = true
		m.helpOffset = 0
		return m, nil, true
	case "x":
		m.clearChosen()
		return m, nil, true
	case "space", " ":
		refs := m.visibleReferences()
		i := m.cursor()
		if i < len(refs) && i >= 0 {
			m.selectReference(refs[i], true)
		}
		return m, nil, true
	case "shift+up", "shift+down":
		refs := m.visibleReferences()
		i := m.cursor()
		if i >= 0 && i < len(refs) {
			m.selectReference(refs[i], false)
			delta := 1
			if k == "shift+up" {
				delta = -1
			}
			m.setCursor(i + delta)
			m.selectReference(refs[m.cursor()], false)
		}
		return m, nil, true
	case "j", "k", "up", "down":
		if m.dialog != nil {
			if m.dialog.mode == "snapshot" {
				delta := 1
				if k == "k" || k == "up" {
					delta = -1
				}
				m.dialog.offset = max(0, m.dialog.offset+delta)
				return m, nil, true
			}
			delta := 1
			if k == "k" || k == "up" {
				delta = -1
			}
			m.setCursor(m.cursor() + delta)
			return m, nil, true
		}
		if m.detail != nil || (m.jira != nil && m.jira.detail != nil && m.visibleProvider() == "jira") || (m.copilot != nil && m.visibleProvider() == "copilot" && (m.copilot.detail != nil || m.copilot.diagnosticsOpen)) {
			return m.scrollDetails(k)
		}
		delta := 1
		if k == "k" || k == "up" {
			delta = -1
		}
		m.setCursor(m.cursor() + delta)
		return m, nil, true
	case "h", "l", "left", "right":
		delta := 1
		if k == "h" || k == "left" {
			delta = -1
		}
		next, cmd := m.changeTab(delta)
		return next, cmd, true
	case "d":
		next, cmd := m.showDetails()
		return next, cmd, true
	case "/":
		if m.visibleProvider() == "deferred" {
			rows := m.visibleReferences()
			next, _ := m.showEntities("Søk i utsatte", "deferred", rows, nil)
			m = next.(Model)
			if m.dialog != nil {
				m.dialog.searching = true
			}
			return m, nil, true
		}
		if m.dialog != nil {
			m.dialog.searching = true
			return m, nil, true
		}
		if m.visibleProvider() == "mattermost" {
			s := m.mmReader
			s.searching = true
			s.input.SetValue(s.query)
			s.input.Prompt = "/ "
			return m, s.input.Focus(), true
		}
		next, cmd := m.dispatchKey("/")
		return next, cmd, true
	case "esc":
		if m.dialog != nil {
			m.dialog = nil
			return m, nil, true
		}
		if m.detail != nil {
			m.detail = nil
			return m, nil, true
		}
		if m.visibleProvider() == "jira" && m.jira.detail != nil {
			m.jira.detail = nil
			return m, nil, true
		}
		if m.tasks.active && m.tasks.details {
			m.tasks.details = false
			return m, nil, true
		}
		if m.visibleProvider() == "copilot" && (m.copilot.detail != nil || m.copilot.diagnosticsOpen) {
			next, cmd := m.dispatchKey("esc")
			return next, cmd, true
		}
		if m.hasChosen() {
			m.clearChosen()
			return m, nil, true
		}
		if m.visibleProvider() == "task" && m.tasks.query == "" && m.previousProvider != "" {
			next, cmd := m.viewProvider("r")
			return next, cmd, true
		}
		next, cmd := m.dispatchKey("esc")
		return next, cmd, true
	case "z":
		next, cmd := m.beginSnooze()
		return next, cmd, true
	case "enter":
		if m.dialog != nil {
			next, cmd := m.acceptEntities()
			return next, cmd, true
		}
	}
	return m, nil, true
}
func (m *Model) clearChosen() {
	clear(m.chosen)
	clear(m.jiraChosen)
	clear(m.extraChosen)
	if m.dialog != nil {
		clear(m.dialog.chosen)
	}
}
func (m Model) hasChosen() bool {
	return len(m.chosen)+len(m.jiraChosen)+len(m.extraChosen) > 0 || (m.dialog != nil && len(m.dialog.chosen) > 0)
}
func (m Model) dispatchKey(k string) (tea.Model, tea.Cmd) {
	code := rune(0)
	switch k {
	case "enter":
		code = tea.KeyEnter
	case "esc":
		code = tea.KeyEscape
	default:
		if len([]rune(k)) == 1 {
			code = []rune(k)[0]
		}
	}
	key := tea.KeyPressMsg{Code: code, Text: k}
	if m.snoozes != nil && m.snoozes.active {
		_, cmd := m.snoozeMessage(key)
		return m, cmd
	}
	switch m.visibleProvider() {
	case "task":
		return m.updateTasks(key)
	case "copilot":
		return m.updateCopilot(key)
	case "jira":
		return m.updateJira(key)
	case "mattermost":
		_, cmd := m.mmMessage(key)
		return m, cmd
	}
	m.internalKey = true
	return m.Update(key)
}
func (m Model) scrollDetails(k string) (tea.Model, tea.Cmd, bool) {
	delta := 1
	if k == "k" || k == "up" {
		delta = -1
	}
	if m.detail != nil {
		m.detail.offset = max(0, min(m.detail.offset+delta, max(0, len(m.detailLines())-max(1, m.height))))
		return m, nil, true
	}
	if m.visibleProvider() == "jira" {
		m.jira.detailOffset = max(0, min(m.jira.detailOffset+delta, max(0, len(m.jiraDetailLines())-max(1, m.height-1))))
		return m, nil, true
	}
	next, cmd := m.dispatchKey(k)
	return next, cmd, true
}
func (m Model) changeTab(delta int) (tea.Model, tea.Cmd) {
	switch m.visibleProvider() {
	case "task":
		m.tasks.moveTask(delta, true)
	case "jira":
		m.jira.active = (m.jira.active + delta + len(m.jira.tabs)) % len(m.jira.tabs)
		m.jira.custom = ""
		m.jira.selected = 0
		return m, m.refreshJira()
	case "mattermost":
		m.mmReader.changeTab(delta)
	case "github":
		m.active = model.View((int(m.active) + delta + int(model.ViewCount)) % int(model.ViewCount))
		m.selected = 0
	}
	return m, nil
}
func (m Model) showDetails() (tea.Model, tea.Cmd) {
	if m.dialog != nil {
		if m.dialog.mode == "snapshot" {
			return m, nil
		}
		if m.dialog.mode == "archive" {
			refs := m.currentReferences()
			if len(refs) > 0 {
				return m.showSnapshot(refs[0])
			}
			return m.unavailable()
		}
		refs := m.currentReferences()
		if len(refs) > 0 {
			return m.activateReference(refs[0])
		}
		return m.unavailable()
	}
	switch m.visibleProvider() {
	case "task":
		m.tasks.details = true
		return m, nil
	case "copilot":
		return m.updateCopilot(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "jira":
		rows := m.jira.filtered()
		if len(rows) > 0 {
			issue := rows[m.jira.selected]
			m.jira.detail = &issue
			m.jira.detailOffset = 0
		}
		return m, nil
	case "deferred":
		refs := m.currentReferences()
		if len(refs) > 0 {
			return m.showSnapshot(refs[0])
		}
		return m.unavailable()
	case "mattermost":
		return m.dispatchKey("d")
	case "github":
		return m.dispatchKey("d")
	}
	return m.unavailable()
}
func (m Model) unavailable() (tea.Model, tea.Cmd) {
	m.sequenceRefs = nil
	m.sequenceNotice = "Ingen tilgjengelige mål eller handlinger for dette valget."
	return m, nil
}
func sequenceHint(prefix string) string {
	if prefix == "ee" {
		return "e chattens arbeidsmappe · w worktree"
	}
	switch prefix[0] {
	case 'r':
		return "o oppgaver · g GitHub · t Jira · a AI · c chat"
	case 't':
		return "o oppgaver · g GitHub · t Jira · a AI · c chat (velg koblingsmål)"
	case 'v':
		return "o/g/t/a/c visning · m bruksstatistikk · s innstillinger · z utsatte · b arkiv · i diagnostikk · f filtre · j JQL · k kanaler · h oppsett"
	case 'e':
		return "e editor · n nettleser · o/g/t/a/c relatert mål · v editorvalg · l lenker"
	case 'a':
		return "+ automerge på · - automerge av · r ny Copilot-review (GitHub) · s melding · k arkiver · d slett · w slett worktree (GitHub) · f flytt Jira · m meg · r fjern meg · u oppdater · n ny · e tittel"
	}
	return fmt.Sprint(prefix)
}
