package tui

import (
	"context"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/jira"
	"github.com/haakonph/custom-visning-terminal/internal/syncstate"
)

type jiraActionState struct {
	keys             []string
	transitions      map[string][]jira.Transition
	choices          []string
	selected         int
	shortcut         string
	loading, running bool
}
type jiraActionMsg struct {
	token       syncstate.Token
	request     *jiraActionState
	transitions map[string][]jira.Transition
	err         error
	done        bool
}

// Resolve and deduplicate at the start of the action; never depend on a view
// switch or on the timing of a background search to find the intended issues.
func (m Model) actionJiraKeys() []string {
	refs := m.sequenceRefs
	if refs == nil {
		refs = m.currentReferences()
	}
	refs = m.relatedReferences(refs, "jira")
	keys := []string{}
	seen := map[string]bool{}
	for _, r := range refs {
		if r.Key != "" && !seen[r.Key] {
			seen[r.Key] = true
			keys = append(keys, r.Key)
		}
	}
	sort.Strings(keys)
	return keys
}
func (m Model) startJiraAction(operation string) (tea.Model, tea.Cmd) {
	keys := m.actionJiraKeys()
	if m.jira == nil || len(keys) == 0 {
		return m.unavailable()
	}
	state := &jiraActionState{keys: keys, loading: operation == "f", running: operation != "f"}
	m.jiraAction = state
	client, parent := m.jira.client, m.jiraSession.Context(m.ctx)
	token := m.jiraSession.Begin("action")
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		result := jiraActionMsg{token: token, request: state, transitions: map[string][]jira.Transition{}, done: operation != "f"}
		outcome := jira.PrepareWorkflow(ctx, client, keys, operation)
		result.transitions, result.err = outcome.Transitions, outcome.Err
		return result
	}
}
func transitionName(t jira.Transition) string { return jira.TransitionName(t) }
func (m Model) finishJiraAction(result jiraActionMsg) (tea.Model, tea.Cmd) {
	if m.jiraAction != result.request || (result.token.Sequence != 0 && !m.jiraSession.Match("action", result.token)) {
		return m, nil
	}
	state := m.jiraAction
	if result.err != nil {
		m.jiraAction = nil
		m.sequenceNotice = result.err.Error()
		return m, nil
	}
	if result.done {
		m.jiraSession.Mutate()
		m.jira.loading = false
		m.jiraAction = nil
		m.sequenceNotice = "Jira oppdatert: " + strings.Join(state.keys, ", ")
		if m.page == pageJira {
			return m, m.refreshJira()
		}
		return m, nil
	}
	state.loading = false
	state.transitions = result.transitions
	state.choices = jira.CommonTransitions(state.keys, state.transitions)
	if len(state.choices) == 0 {
		m.jiraAction = nil
		m.sequenceNotice = "Ingen entydige felles overganger tilgjengelig for valgte Jira-saker."
		return m, nil
	}
	if state.shortcut != "" {
		return m.jiraActionKey(state.shortcut)
	}
	return m, nil
}
func statusMatches(name, shortcut string) bool { return jira.StatusMatches(name, shortcut) }
func (m Model) jiraActionKey(k string) (tea.Model, tea.Cmd) {
	state := m.jiraAction
	if k == "ctrl+c" {
		return m, m.quit()
	}
	if state.running {
		return m, nil
	}
	if k == "esc" {
		m.jiraAction = nil
		return m, nil
	}
	if state.loading {
		if strings.Contains("quaf", k) && len(k) == 1 {
			state.shortcut = k
		}
		return m, nil
	}
	choice := ""
	switch k {
	case "j", "down":
		state.selected = min(state.selected+1, len(state.choices)-1)
	case "k", "up":
		state.selected = max(0, state.selected-1)
	case "enter":
		choice = state.choices[state.selected]
	case "q", "u", "a", "f":
		matches := []string{}
		for _, name := range state.choices {
			if statusMatches(name, k) {
				matches = append(matches, name)
			}
		}
		if len(matches) == 1 {
			choice = matches[0]
		} else {
			m.sequenceNotice = "Ingen entydig overgang for denne hurtigtasten. Velg fra listen."
		}

	}
	if choice == "" {
		return m, nil
	}
	state.running = true
	m.sequenceNotice = ""
	client, parent := m.jira.client, m.jiraSession.Context(m.ctx)
	token := m.jiraSession.Begin("action")
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		result := jiraActionMsg{token: token, request: state, done: true}
		result.err = jira.ExecuteTransitions(ctx, client, state.keys, state.transitions, choice).Err
		return result
	}
}

func (m Model) openJiraKeys(keys []string) (tea.Model, tea.Cmd) {
	if m.jira == nil || len(keys) == 0 {
		return m.unavailable()
	}
	client, ctx := m.jira.client, m.ctx
	return m, func() tea.Msg {
		for _, key := range keys {
			if err := openJiraBrowser(ctx, client, client.IssueURL(key)); err != nil {
				return openMsg{err: err}
			}
		}
		return openMsg{}
	}
}
func (m Model) openRelatedJira() (tea.Model, tea.Cmd) { return m.openJiraKeys(m.actionJiraKeys()) }
func (m Model) openRelatedPRs() (tea.Model, tea.Cmd) {
	var links []string
	seen := map[string]bool{}
	for _, key := range m.actionJiraKeys() {
		for _, p := range m.relatedPRs(key) {
			if !seen[p.URL] {
				links = append(links, p.URL)
				seen[p.URL] = true
			}
		}
	}
	if len(links) == 0 {
		return m.unavailable()
	}
	opener, ctx := m.open, m.ctx
	return m, func() tea.Msg {
		for _, link := range links {
			if err := opener(ctx, link); err != nil {
				return openMsg{err: err}
			}
		}
		return openMsg{}
	}
}
