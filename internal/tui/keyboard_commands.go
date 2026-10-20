package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
	"net/url"
	"strings"
)

func (m Model) executeSequence(prefix, k string) (tea.Model, tea.Cmd) {
	refs := m.sequenceRefs
	if m.mattermost != nil && m.mattermost.active {
		if prefix == "e" && k == "e" {
			return m, m.editMattermost()
		}
		if prefix == "a" && k == "s" {
			_, cmd := m.mattermostMessage(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
			return m, cmd
		}
		return m.unavailable()
	}
	if prefix == "ee" {
		switch k {
		case "e", "w":
			return m.openReferencesWithTarget(refs, true, k)
		}
		return m.unavailable()
	}
	op := prefix[:1]
	if op == "v" {
		return m.viewProvider(k)
	}
	if op == "r" {
		kind := provider(k)
		if kind != "" {
			rows := m.relatedReferences(refs, kind)
			if len(rows) == 1 {
				return m.activateReference(rows[0])
			}
			return m.showEntities("Relatert: "+kind, "related", rows, nil)
		}
		return m.unavailable()
	}
	if op == "t" {
		kind := provider(k)
		if kind != "" {
			var rows []model.TaskReference
			for _, r := range m.allReferences() {
				if r.Provider == kind {
					rows = append(rows, r)
				}
			}
			if len(refs) == 0 {
				return m.unavailable()
			}
			return m.showEntities("Koble til: "+kind, "link", rows, refs)
		}
		return m.unavailable()
	}
	if op == "a" {
		switch k {
		case "+", "-":
			return m.startAutoMerge(refs, k == "+")
		case "f", "m", "r":
			if allProvider(refs, "jira") {
				m.sequenceRefs = refs
				return m.startJiraAction(k)
			}
		case "s":
			return m.composeReferences(refs)
		case "k":
			return m.archiveReferences(refs)
		case "d":
			return m.deleteReferences(refs)
		case "u":
			return m.refreshVisible()
		case "n":
			return m.newTaskFrom(refs)
		case "e":
			if allProvider(refs, "task") && m.visibleProvider() != "task" && len(refs) == 1 {
				next, _ := m.activateReference(refs[0])
				m = next.(Model)
			}
			if m.visibleProvider() == "task" && len(m.tasks.visibleTasks()) > 0 {
				return m.updateTasks(tea.KeyPressMsg{Code: 'E', Text: "E"})
			}
		case "x":
			if m.visibleProvider() == "jira" {
				m.jira.filters = jira.Filters{}
				m.jira.custom = ""
				m.jira.input.SetValue("")
				m.jira.input.Prompt = ""
				return m, m.refreshJira()
			}
		case "q":
			if m.jira != nil && m.visibleProvider() == "jira" {
				return m, m.jira.edit("name", "Lagre søk som: ", "")
			}
		case "l", "p":
			if m.mmReader != nil && m.visibleProvider() == "mattermost" && !m.mmReader.busy {
				mode := "Password"
				if k == "p" {
					mode = "PAT"
				}
				return m, m.mmReader.prompt(mode)
			}
		case "h":
			if m.visibleProvider() == "mattermost" {
				return m.dispatchKey("H")
			}
		case "v":
			if m.visibleProvider() == "mattermost" {
				return m, m.mmRead()
			}
		case "w":
			if m.visibleProvider() == "mattermost" {
				return m.dispatchKey("-")
			}
		case "b":
			if m.visibleProvider() == "mattermost" {
				return m.dispatchKey("D")
			}
		case "down", "up":
			if m.visibleProvider() == "mattermost" {
				delta := max(1, m.height/2)
				if k == "up" {
					delta = -delta
				}
				m.mmReader.scroll = max(0, m.mmReader.scroll+delta)
				return m, nil
			}
		case "z":
			if m.snoozes.active {
				return m.restoreDeferred(refs)
			}
		}
	}
	if op == "e" {
		switch k {
		case "e":
			if prefix == "e" && m.visibleProvider() == "copilot" {
				m.sequence = "ee"
				return m, nil
			}
			return m.openReferences(refs, true)
		case "n":
			return m.openReferences(refs, false)
		case "v":
			if m.local != nil && m.visibleProvider() == "github" {
				m.local.editorChoice = 0
				m.local.choosing = true
				return m, nil
			}
		case "l":
			var links []model.TaskReference
			for _, r := range refs {
				for _, link := range messageLinks(r.Title) {
					links = append(links, model.TaskReference{Provider: "external", URL: link, Title: link, Key: link})
				}
			}
			return m.showEntities("Lenker", "external", links, nil)
		}
	}
	if kind := provider(k); kind != "" {
		targets := m.relatedReferences(refs, kind)
		if len(targets) == 0 {
			return m.unavailable()
		}
		m.sequenceRefs = targets
		m.sequence = prefix + k
		return m, nil
	}
	return m.unavailable()
}
func (m Model) viewProvider(k string) (tea.Model, tea.Cmd) {
	if k == "h" {
		kind := m.visibleProvider()
		if m.setup.provider != "" {
			kind = m.setup.provider
		}
		return m.WithSetup(kind), nil
	}
	if k != "s" {
		m.setup = setupState{}
	}
	if k == "r" {
		k = map[string]string{"task": "o", "github": "g", "jira": "t", "copilot": "a", "mattermost": "c"}[m.previousProvider]
		if k == "" {
			return m.unavailable()
		}
	}
	switch k {
	case "s":
		next, cmd := m.openSettings()
		result := next.(Model)
		path := map[string]string{"jira": "jira.base_url", "mattermost": "mattermost.server", "copilot": "copilot.session_root", "editor": "repository_paths"}[m.setup.provider]
		for i, field := range settingFields {
			if field.path == path {
				result.settings.selected = i
				break
			}
		}
		return result, cmd
	case "z":
		m.snoozes.active = true
		m.snoozes.selected = 0
		m.dialog = nil
		return m, nil
	case "b":
		rows := append([]model.TaskReference(nil), m.entities.data.Archived...)
		if err := m.ensureTaskIDs(); err != nil {
			return m.unavailable()
		}
		for _, t := range m.tasks.items {
			if t.Archived {
				rows = append(rows, taskRef(t))
			}
		}
		return m.showEntities("Arkiv · ak gjenopprett", "archive", rows, nil)
	case "i":
		if m.copilot != nil {
			m.rememberProvider("copilot")
			m.snoozes.active = false
			m.detail = nil
			m.tasks.active = false
			m.jiraActive = false
			m.copilotActive = true
			m.copilot.diagnosticsOpen = true
			if m.mmReader != nil {
				m.mmReader.active = false
			}
			m.dialog = nil
			return m, nil
		}
		return m.WithSetup("copilot"), nil
	case "f", "j":
		if m.jira != nil {
			m.rememberProvider("jira")
			m.snoozes.active = false
			m.detail = nil
			m.jira.detail = nil
			m.tasks.active = false
			m.copilotActive = false
			m.jiraActive = true
			m.dialog = nil
			if m.mmReader != nil {
				m.mmReader.active = false
			}
			if k == "f" {
				return m.updateJira(tea.KeyPressMsg{Code: 'f', Text: "f"})
			}
			return m, m.jira.edit("jql", "JQL: ", m.jira.currentQuery())
		}
		return m.WithSetup("jira"), nil
	case "k":
		if m.mmReader != nil {
			m.rememberProvider("mattermost")
			m.snoozes.active = false
			m.tasks.active = false
			m.jiraActive = false
			m.copilotActive = false
			m.detail = nil
			m.mmReader.active = true
			m.dialog = nil
			return m.dispatchKey("+")
		}
		return m.viewProvider("c")
	}
	kind := provider(k)
	if kind == "" {
		return m.unavailable()
	}
	var mmCmd tea.Cmd
	if kind == "mattermost" && m.mmReader == nil {
		var err error
		m, err = m.WithMattermostConfig()
		if err != nil {
			return m.WithSetupProblem("mattermost", "Kunne ikke klargjøre Mattermost: "+err.Error()), nil
		}
		if m.mmReader != nil {
			m.mmReader.active = false
			mmCmd = m.mmStart()
		}
	}
	if (kind == "jira" && m.jira == nil) || (kind == "copilot" && m.copilot == nil) || (kind == "mattermost" && m.mmReader == nil) || (kind == "github" && m.source == nil) {
		return m.WithSetup(kind), nil
	}
	m.rememberProvider(kind)
	m.dialog = nil
	m.detail = nil
	m.tasks.active = false
	m.jiraActive = false
	m.copilotActive = false
	m.snoozes.active = false
	if m.mmReader != nil {
		m.mmReader.active = false
	}
	switch kind {
	case "task":
		m.tasks.active = true
		m.tasks.archived = false
		m.tasks.selectVisible()
		return m, nil
	case "jira":
		m.jiraActive = true
		m.jira.detail = nil
		if m.jira.query == "" {
			return m, m.refreshJira()
		}
	case "copilot":
		m.copilotActive = true
		return m, m.refreshCopilot()
	case "mattermost":
		m.mmReader.active = true
		return m, mmCmd
	case "github":
		m.relatedKey = ""
	}
	return m, nil
}
func (m Model) refreshVisible() (tea.Model, tea.Cmd) {
	if m.detail != nil {
		return m, m.loadDetails()
	}
	switch m.visibleProvider() {
	case "jira":
		return m, m.refreshJira()
	case "copilot":
		return m, m.refreshCopilot()
	case "mattermost":
		return m.dispatchKey("R")
	case "github":
		if m.snapshot.User == "" && !m.booting {
			m.booting = true
			return m, m.bootstrap()
		}
		return m, m.refresh(m.active)
	case "deferred":
		m.snoozes.at = m.snoozes.now()
		return m, nil
	case "task":
		fresh := newTasks()
		if fresh.loadFailed {
			m.sequenceNotice = fresh.notice
			return m, nil
		}
		m.tasks.items = fresh.items
		m.tasks.selectVisible()
		return m, nil
	}
	return m.unavailable()
}
func (m Model) newTaskFrom(refs []model.TaskReference) (tea.Model, tea.Cmd) {
	refs = underlyingReferences(refs)
	if m.tasks.loadFailed {
		return m.unavailable()
	}
	title := "Ny oppgave"
	if m.visibleProvider() == "task" {
		refs = nil
	}
	if len(refs) > 0 {
		title = refs[0].Title
	}
	m.rememberProvider("task")
	m.jiraActive = false
	m.copilotActive = false
	if m.mmReader != nil {
		m.mmReader.active = false
	}
	m.snoozes.active = false
	m.detail = nil
	m.tasks.items = append(m.tasks.items, model.Task{Title: title, References: append([]model.TaskReference(nil), refs...)})
	m.tasks.selected = len(m.tasks.items) - 1
	m.tasks.row = 0
	m.tasks.active = true
	m.tasks.archived = false
	m.tasks.query = ""
	m.tasks.details = false
	m.dialog = nil
	if err := m.ensureTaskIDs(); err != nil {
		return m, nil
	}
	if len(refs) == 0 {
		return m.updateTasks(tea.KeyPressMsg{Code: 'E', Text: "E"})
	}
	return m, nil
}
func (m Model) composeReferences(refs []model.TaskReference) (tea.Model, tea.Cmd) {
	if m.mattermost == nil {
		return m.WithSetup("webhook"), nil
	}
	if len(refs) == 0 {
		return m.unavailable()
	}
	var lines []string
	for _, r := range refs {
		if strings.HasPrefix(r.URL, "https://") {
			lines = append(lines, markdownLink(r.Title, r.URL))
		} else {
			lines = append(lines, clean(r.Title)+" ("+r.Key+")")
		}
	}
	m.mattermost.text = strings.Join(lines, "\n")
	allPR := true
	for _, r := range refs {
		if r.Provider != "github" {
			allPR = false
		}
	}
	if allPR {
		var rows []model.PullRequest
		for _, r := range refs {
			found := false
			for _, d := range m.snapshot.Datasets {
				for _, p := range d.PRs {
					if p.URL == r.URL && !found {
						rows = append(rows, p)
						found = true
					}
				}
			}
		}
		var issueURL func(string) string
		if m.jira != nil && m.jira.client != nil {
			issueURL = m.jira.client.IssueURL
		}
		if len(rows) == len(refs) {
			m.mattermost.text = mattermostText(rows, issueURL, m.jiraPattern)
		}
	}
	m.mattermost.active = true
	m.mattermost.offset = 0
	m.mattermost.notice = ""
	m.dialog = nil
	return m, m.editMattermost()
}
func (m Model) openReferences(refs []model.TaskReference, editor bool) (tea.Model, tea.Cmd) {
	return m.openReferencesWithTarget(refs, editor, "")
}

func (m Model) openReferencesWithTarget(refs []model.TaskReference, editor bool, target string) (tea.Model, tea.Cmd) {
	if len(refs) == 0 {
		return m.unavailable()
	}
	if editor {
		allPR, allAI := true, true
		for _, r := range refs {
			allPR = allPR && r.Provider == "github"
			allAI = allAI && r.Provider == "copilot"
		}
		if allPR {
			original := m.chosen
			m.chosen = map[string]model.PullRequest{}
			for _, r := range refs {
				for _, d := range m.snapshot.Datasets {
					for _, p := range d.PRs {
						if p.URL == r.URL {
							m.chosen[p.Key()] = p
						}
					}
				}
			}
			if len(m.chosen) != len(refs) {
				m.chosen = original
				return m.unavailable()
			}
			cmd := m.openLocal("intellij")
			m.chosen = original
			if cmd == nil && m.notice != "" {
				m.sequenceNotice = m.notice
			}
			return m, cmd
		}
		if allAI && (m.copilot == nil || m.copilot.openIDE == nil) {
			return m.unavailable()
		}
		if allAI && m.copilot != nil {
			if m.copilot.openingIDE {
				return m, nil
			}
			m.copilot.openingIDE = true
			m.copilot.ideNotice = "Åpner prosjektet i IntelliJ…"
			var sessions []model.CopilotSession
			for _, r := range refs {
				if m.copilot.detail != nil && m.copilot.detail.Session.ID == r.Key {
					sessions = append(sessions, m.copilot.detail.Session)
					continue
				}
				for _, s := range m.copilot.sessions {
					if s.ID == r.Key {
						sessions = append(sessions, s)
					}
				}
			}
			if len(sessions) != len(refs) {
				m.copilot.openingIDE = false
				m.copilot.ideNotice = "Ingen lastet AI-sesjon for valget"
				return m.unavailable()
			}
			opener, ctx := m.copilot.openIDE, m.ctx
			return m, func() tea.Msg {
				for _, s := range sessions {
					if target != "" {
						var err error
						s, err = sessionEditorTarget(s, target)
						if err != nil {
							return copilotOpenMsg{err: err}
						}
					}
					if err := opener(ctx, s); err != nil {
						return copilotOpenMsg{err: err}
					}
				}
				return copilotOpenMsg{}
			}
		}
		supported := map[string]model.TaskReference{}
		for _, r := range refs {
			if r.Provider == "github" || r.Provider == "copilot" {
				supported[refID(r)] = r
			} else {
				for _, kind := range []string{"github", "copilot"} {
					for _, target := range m.relatedReferences([]model.TaskReference{r}, kind) {
						supported[refID(target)] = target
					}
				}
			}
		}
		if len(supported) == 0 {
			return m.unavailable()
		}
		var prs, ai []model.TaskReference
		for _, r := range sortedRefs(supported) {
			if r.Provider == "github" {
				prs = append(prs, r)
			} else {
				ai = append(ai, r)
			}
		}
		cmds := []tea.Cmd{}
		for _, group := range [][]model.TaskReference{prs, ai} {
			if len(group) > 0 {
				next, cmd := m.openReferencesWithTarget(group, true, target)
				m = next.(Model)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
		return m, tea.Batch(cmds...)

	}
	links := []model.TaskReference{}
	for _, r := range refs {
		if r.Provider == "task" {
			for _, task := range m.tasks.items {
				if task.ID == r.Key {
					links = append(links, task.References...)
				}
			}
		} else if r.Provider == "copilot" {
			m.sequenceNotice = "AI-sesjoner har ingen nettleseradresse; bruk ee for prosjektet."
			return m, nil
		} else {
			links = append(links, r)
		}
	}
	for _, r := range links {
		u, err := url.Parse(r.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			m.sequenceNotice = "Elementet har ingen gyldig HTTPS-adresse."
			return m, nil
		}
	}
	opener, ctx, j := m.open, m.ctx, m.jira
	return m, func() tea.Msg {
		var failures []string
		for _, r := range links {
			var err error
			switch r.Provider {
			case "github":
				err = opener(ctx, r.URL)
			case "jira":
				if j == nil {
					err = fmt.Errorf("Jira er ikke konfigurert")
				} else {
					err = openJiraBrowser(ctx, j.client, r.URL)
				}
			default:
				if m.mmReader != nil {
					open := m.mmReader.open
					if open == nil {
						open = opener
					}
					err = open(ctx, r.URL)
				} else {
					err = openMattermostBrowser(ctx, r.URL)
				}
			}
			if err != nil {
				failures = append(failures, err.Error())
			}
		}
		if len(failures) > 0 {
			return openMsg{err: fmt.Errorf("%s", strings.Join(failures, "; "))}
		}
		return openMsg{}
	}
}
func (m Model) archiveReferences(refs []model.TaskReference) (tea.Model, tea.Cmd) {
	refs = underlyingReferences(refs)
	if len(refs) == 0 || m.entities.err != nil {
		return m.unavailable()
	}
	restore := (m.dialog != nil && (m.dialog.mode == "archive" || m.dialog.restore)) || (m.visibleProvider() == "task" && m.tasks.archived)
	state := m.entities.data
	state.Archived = append([]model.TaskReference(nil), state.Archived...)
	tasks := append([]model.Task(nil), m.tasks.items...)
	for _, r := range refs {
		if r.Provider == "task" {
			for i := range m.tasks.items {
				if m.tasks.items[i].ID == r.Key {
					m.tasks.items[i].Archived = !restore
				}
			}
		} else {
			var kept []model.TaskReference
			for _, existing := range state.Archived {
				if refID(existing) != refID(r) {
					kept = append(kept, existing)
				}
			}
			if !restore {
				kept = append(kept, r)
			}
			state.Archived = kept
		}
	}
	if err := storage.Save(m.entities.path, state); err != nil {
		m.tasks.items = tasks
		m.sequenceNotice = err.Error()
		return m, nil
	}
	taskChanged := false
	for _, r := range refs {
		if r.Provider == "task" {
			taskChanged = true
		}
	}
	if taskChanged {
		if err := m.tasks.save(); err != nil {
			m.tasks.items = tasks
			if rollback := storage.Save(m.entities.path, m.entities.data); rollback != nil {
				m.entities.data = state
				m.sequenceNotice = "Delvis arkivoppdatering: " + err.Error() + "; tilbakeføring feilet: " + rollback.Error()
			} else {
				m.sequenceNotice = err.Error()
			}
			return m, nil
		}
	}
	m.entities.data = state
	m.clearChosen()
	m.dialog = nil
	m.tasks.selectVisible()
	m.clamp()
	if m.jira != nil {
		m.jira.clamp()
	}
	m.detail = nil
	m.tasks.details = false
	if m.jira != nil {
		m.jira.detail = nil
	}
	if m.copilot != nil {
		m.copilot.detail = nil
		m.copilot.selected = max(0, min(m.copilot.selected, len(m.copilot.rows())-1))
	}
	if m.mmReader != nil {
		m.mmReader.clamp()
	}
	m.sequenceNotice = "Arkivet er oppdatert lokalt"
	return m, nil
}
func (m Model) deleteReferences(refs []model.TaskReference) (tea.Model, tea.Cmd) {
	if m.visibleProvider() != "task" && !allProvider(refs, "task") {
		m.sequenceNotice = "Sletting i eksterne systemer er ikke definert. ad sletter lokale oppgaver eller oppgavereferanser."
		return m, nil
	}
	if len(refs) == 0 {
		return m.unavailable()
	}
	remove := map[string]bool{}
	for _, r := range refs {
		remove[refID(r)] = true
	}
	before := append([]model.Task(nil), m.tasks.items...)
	owner := ""
	referenceMode := m.visibleProvider() == "task" && m.tasks.details
	if referenceMode {
		if len(m.tasks.visibleTasks()) == 0 {
			return m.unavailable()
		}
		task := &m.tasks.items[m.tasks.selected]
		owner = refID(taskRef(*task))
		var kept []model.TaskReference
		for _, r := range task.References {
			if !remove[refID(r)] {
				kept = append(kept, r)
			}
		}
		task.References = kept
		m.tasks.row = 0
	} else {
		var kept []model.Task
		for _, t := range m.tasks.items {
			if !remove[refID(taskRef(t))] {
				kept = append(kept, t)
			}
		}
		m.tasks.items = kept
	}
	if err := m.tasks.save(); err != nil {
		m.tasks.items = before
		m.sequenceNotice = err.Error()
		return m, nil
	}
	state := m.entities.data
	state.Links = nil
	for _, edge := range m.entities.data.Links {
		left, right := refID(edge.Left), refID(edge.Right)
		discard := false
		if referenceMode {
			discard = (left == owner && remove[right]) || (right == owner && remove[left])
		} else {
			discard = remove[left] || remove[right]
		}
		if !discard {
			state.Links = append(state.Links, edge)
		}
	}
	if len(state.Links) != len(m.entities.data.Links) {
		if err := storage.Save(m.entities.path, state); err != nil {
			m.tasks.items = before
			rollback := m.tasks.save()
			m.sequenceNotice = "Kunne ikke slette koblingene: " + err.Error()
			if rollback != nil {
				m.sequenceNotice += "; tilbakeføring feilet: " + rollback.Error()
			}
			return m, nil
		}
		m.entities.data = state
	}
	m.clearChosen()
	m.tasks.selectVisible()
	return m, nil
}
func (m Model) beginSnooze() (tea.Model, tea.Cmd) {
	refs := m.currentReferences()
	if len(refs) == 0 || m.snoozes.loadErr != nil {
		return m.unavailable()
	}
	m.snoozes.targets = nil
	m.snoozes.origins = append([]model.TaskReference(nil), refs...)
	m.snoozes.whole = false
	for _, r := range refs {
		id := refSnoozeID(r)
		if id == "" {
			return m.unavailable()
		}
		entry := snooze.Entry{ID: id, Kind: r.Provider, URL: r.URL, Title: r.Title}
		if saved, ok := m.snoozes.entries[id]; ok && len(saved.Roots) > 0 {
			entry = saved
			m.snoozes.whole = true
		}
		m.snoozes.targets = append(m.snoozes.targets, entry)
	}
	m.dialog = nil
	m.snoozes.target = m.snoozes.targets[0]
	m.snoozes.input = true
	m.snoozes.digits = ""
	m.snoozes.notice = ""
	return m, nil
}
func (m Model) acceptEntities() (tea.Model, tea.Cmd) {
	rows := m.currentReferences()
	if len(rows) == 0 {
		return m.unavailable()
	}
	switch m.dialog.mode {
	case "link":
		return m.linkReferences(m.dialog.origins, rows)
	case "external":
		return m.openReferences(rows, false)
	case "archive", "deferred":
		return m.showSnapshot(rows[0])
	case "snapshot":
		return m, nil
	default:
		return m.activateReference(rows[0])
	}
}

func allProvider(refs []model.TaskReference, kind string) bool {
	if len(refs) == 0 {
		return false
	}
	for _, r := range refs {
		if r.Provider != kind {
			return false
		}
	}
	return true
}

func (m Model) restoreDeferred(refs []model.TaskReference) (tea.Model, tea.Cmd) {
	entries := []snooze.Entry{}
	for _, r := range refs {
		if entry, ok := m.snoozes.entries[refSnoozeID(r)]; ok {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return m.unavailable()
	}
	anchor := m.snoozeAnchor()
	if err := m.snoozes.commitEntries(entries, true); err != nil {
		m.sequenceNotice = "Kunne ikke hente tilbake: " + err.Error()
		return m, nil
	}
	m.clearChosen()
	m.dialog = nil
	m.restoreSnoozeAnchor(anchor)
	m.snoozes.notice = fmt.Sprintf("Hentet tilbake %d elementer", len(entries))
	return m, nil
}
