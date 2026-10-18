package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func (m Model) relatedPRs(key string) []model.PullRequest {
	var rows []model.PullRequest
	for _, dataset := range m.snapshot.Datasets {
		rows = model.Merge(rows, dataset.PRs)
	}
	var matched []model.PullRequest
	for _, p := range rows {
		if p.HasJiraKey(key, m.jiraPattern) {
			matched = append(matched, p)
		}
	}
	return matched
}

func (m Model) navigateRelated() (tea.Model, tea.Cmd) {
	if m.jiraActive {
		if m.source == nil {
			m.jira.notice = "GitHub er ikke konfigurert"
			return m, nil
		}
		rows := m.jira.filtered()
		if len(rows) == 0 {
			return m, nil
		}
		m.relatedKey = rows[m.jira.selected].Key
		m.selected = 0
		m.input.SetValue("")
		m.filtering = false
		m.help = false
		m.jiraActive = false
		return m, nil
	}
	if m.jira == nil {
		m.notice = "Sett JIRA_BASE_URL og JIRA_TOKEN for å vise relaterte saker"
		return m, nil
	}
	rows := m.filtered()
	if len(rows) == 0 {
		return m, nil
	}
	keys := rows[m.selected].JiraKeys(m.jiraPattern)
	if len(keys) == 0 {
		m.notice = "Ingen Jira-nøkler i PR-tittel, beskrivelse eller branchnavn"
		return m, nil
	}
	m.jira.custom = "key IN (" + strings.Join(keys, ",") + ") ORDER BY key ASC"
	m.jira.input.SetValue("")
	m.jira.input.Prompt = ""
	m.jira.mode = ""
	m.jira.selected = 0
	m.jiraActive = true
	return m, m.refreshJira()
}
