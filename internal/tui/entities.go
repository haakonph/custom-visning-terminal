package tui

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

type entityState struct {
	data model.EntityState
	path string
	err  error
}
type entityDialog struct {
	offset             int
	restore            bool
	title, mode, query string
	rows, origins      []model.TaskReference
	selected           int
	chosen             map[string]model.TaskReference
	searching          bool
	commands           bool
}

func newEntities() *entityState {
	path := os.Getenv("CUSTOM_VISNING_LINKS")
	if path == "" {
		dir, _ := os.UserConfigDir()
		path = filepath.Join(dir, "custom-visning", "links.sqlite")
	}
	s := &entityState{path: path}
	s.err = storage.Load(path, &s.data)
	if errors.Is(s.err, os.ErrNotExist) {
		s.err = nil
	}
	return s
}
func refID(r model.TaskReference) string {
	if r.SnoozeID != "" {
		return "snooze:" + r.SnoozeID
	}
	return r.Provider + ":" + r.URL
}

func underlyingReferences(refs []model.TaskReference) []model.TaskReference {
	refs = append([]model.TaskReference(nil), refs...)
	for i := range refs {
		refs[i].SnoozeID = ""
	}
	return refs
}
func provider(k string) string {
	return map[string]string{"o": "task", "g": "github", "t": "jira", "a": "copilot", "c": "mattermost"}[k]
}
func (s *entityState) archived(kind, link string) bool {
	if s == nil {
		return false
	}
	for _, r := range s.data.Archived {
		if r.Provider == kind && r.URL == link {
			return true
		}
	}
	return false
}
func (m *Model) ensureTaskIDs() error {
	if m.tasks.loadFailed {
		return fmt.Errorf("oppgaver kunne ikke leses")
	}
	changed := false
	for i := range m.tasks.items {
		if m.tasks.items[i].ID == "" {
			if m.localBusy() {
				return fmt.Errorf("venter på lokal lagring")
			}
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				return err
			}
			m.tasks.items[i].ID = fmt.Sprintf("%x", id)
			changed = true
		}
	}
	if changed {
		return m.tasks.save()
	}
	return nil
}
func taskRef(t model.Task) model.TaskReference {
	return model.TaskReference{Provider: "task", URL: "task://" + t.ID, Key: t.ID, Title: t.Title}
}
func copilotRef(s model.CopilotSession) model.TaskReference {
	return model.TaskReference{Provider: "copilot", URL: "copilot://session/" + url.PathEscape(s.ID), Key: s.ID, Title: s.Name}
}
func (m Model) postRef(p model.MattermostPost) model.TaskReference {
	base := ""
	if m.mmReader != nil {
		base = strings.TrimRight(m.mmReader.data.Server, "/")
	}
	return model.TaskReference{Provider: "mattermost", URL: base + "/_redirect/pl/" + p.ID, Key: p.ID, Title: p.Message}
}
func prRef(p model.PullRequest) model.TaskReference {
	return model.TaskReference{Provider: "github", URL: p.URL, Key: fmt.Sprintf("%s#%d", p.Repository, p.Number), Title: p.Title}
}
func jiraRef(key string, m Model) model.TaskReference {
	link := ""
	if m.jira != nil && m.jira.client != nil {
		link = m.jira.client.IssueURL(key)
	}
	return model.TaskReference{Provider: "jira", URL: link, Key: key, Title: key}
}
func (m Model) visibleProvider() string {
	if m.dialog != nil {
		return "dialog"
	}
	if m.snoozes != nil && m.snoozes.active {
		return "deferred"
	}
	return m.baseProvider()
}
func (m *Model) rememberProvider(target string) {
	current := m.baseProvider()
	if current != target {
		m.previousProvider = current
	}
}
func (m Model) baseProvider() string {
	if m.mmReader != nil && m.mmReader.active {
		return "mattermost"
	}
	if m.tasks.active {
		return "task"
	}
	if m.copilotActive {
		return "copilot"
	}
	if m.jiraActive {
		return "jira"
	}
	return "github"
}
func (m *Model) visibleReferences() []model.TaskReference {
	var refs []model.TaskReference
	switch m.visibleProvider() {
	case "dialog":
		return m.dialog.filtered()
	case "deferred":
		known := map[string]model.TaskReference{}
		for _, r := range m.allReferences() {
			known[refID(r)] = r
		}
		for _, e := range m.snoozes.rows() {
			r := model.TaskReference{Provider: e.Kind, URL: e.URL, Key: strings.TrimPrefix(e.ID, "task:"), Title: e.Title}
			if e.Kind == "task" && r.URL == "" {
				r.URL = "task://" + r.Key
			}
			if fresh, ok := known[refID(r)]; ok {
				r = fresh
			} else if e.Kind == "jira" || e.Kind == "mattermost" {
				r.Key = r.URL[strings.LastIndex(r.URL, "/")+1:]
			} else if e.Kind == "copilot" {
				if u, err := url.Parse(r.URL); err == nil {
					r.Key = strings.TrimPrefix(u.Path, "/")
				}
			}
			r.SnoozeID = e.ID
			if len(e.Roots) > 0 {
				r.Title = e.Title + " · hele sammenhengen"
			}
			refs = append(refs, r)
		}

	case "task":
		if err := m.ensureTaskIDs(); err != nil {
			m.sequenceNotice = err.Error()
			return nil
		}
		if m.tasks.details {
			if len(m.tasks.visibleTasks()) > 0 {
				refs = append(refs, m.tasks.items[m.tasks.selected].References...)
			}
		} else {
			for _, i := range m.tasks.visibleTasks() {
				refs = append(refs, taskRef(m.tasks.items[i]))
			}
		}
	case "jira":
		for _, i := range m.jira.filtered() {
			refs = append(refs, model.TaskReference{Provider: "jira", URL: i.URL, Key: i.Key, Title: i.Title})
		}
	case "copilot":
		for _, s := range m.copilot.rows() {
			refs = append(refs, copilotRef(s))
		}
	case "mattermost":
		for _, p := range m.mmReader.posts() {
			refs = append(refs, m.postRef(p))
		}
	default:
		for _, p := range m.filtered() {
			refs = append(refs, prRef(p))
		}
	}
	return refs
}
func (m Model) cursor() int {
	switch m.visibleProvider() {
	case "dialog":
		return m.dialog.selected
	case "deferred":
		return m.snoozes.selected
	case "task":
		if m.tasks.details {
			return m.tasks.row
		}
		for p, i := range m.tasks.visibleTasks() {
			if i == m.tasks.selected {
				return p
			}
		}
		return 0
	case "jira":
		return m.jira.selected
	case "copilot":
		return m.copilot.selected
	case "mattermost":
		return m.mmReader.row
	default:
		return m.selected
	}
}
func (m *Model) setCursor(i int) {
	count := 0
	switch m.visibleProvider() {
	case "dialog":
		count = len(m.dialog.filtered())
	case "deferred":
		count = len(m.snoozes.rows())
	case "task":
		if m.tasks.details {
			if len(m.tasks.visibleTasks()) > 0 {
				count = len(m.tasks.items[m.tasks.selected].References)
			}
		} else {
			count = len(m.tasks.visibleTasks())
		}
	case "jira":
		count = len(m.jira.filtered())
	case "copilot":
		count = len(m.copilot.rows())
	case "mattermost":
		count = len(m.mmReader.posts())
	default:
		count = len(m.filtered())
	}
	i = max(0, min(i, count-1))
	switch m.visibleProvider() {
	case "dialog":
		m.dialog.selected = i
	case "deferred":
		m.snoozes.selected = i
	case "task":
		if m.tasks.details {
			m.tasks.row = i
		} else if rows := m.tasks.visibleTasks(); len(rows) > 0 {
			m.tasks.selected = rows[i]
		}
	case "jira":
		m.jira.selected = i
	case "copilot":
		m.copilot.selected = i
	case "mattermost":
		m.mmReader.row = i
		m.mmReader.scroll = 0
		m.mmReader.link = 0
	default:
		m.selected = i
	}
}
func sortedRefs(values map[string]model.TaskReference) []model.TaskReference {
	refs := make([]model.TaskReference, 0, len(values))
	for _, r := range values {
		refs = append(refs, r)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Provider == refs[j].Provider {
			return refs[i].URL < refs[j].URL
		}
		return refs[i].Provider < refs[j].Provider
	})
	return refs
}
func (m *Model) currentReferences() []model.TaskReference {
	kind := m.visibleProvider()
	if kind == "github" && m.detail != nil && len(m.chosen) == 0 {
		return []model.TaskReference{prRef(m.detail.pr)}
	}
	if kind == "jira" && m.jira.detail != nil && len(m.jiraChosen) == 0 {
		r := m.jira.detail
		return []model.TaskReference{{Provider: "jira", URL: r.URL, Key: r.Key, Title: r.Title}}
	}
	if kind == "github" {
		var refs []model.TaskReference
		for _, p := range m.selectedPRs() {
			r := prRef(p)
			if m.active.IsReview() {
				r.Role = "review"
			}
			refs = append(refs, r)
		}
		return refs
	}
	if kind == "jira" && len(m.jiraChosen) > 0 {
		var refs []model.TaskReference
		for _, r := range m.jiraChosen {
			refs = append(refs, model.TaskReference{Provider: "jira", URL: r.URL, Key: r.Key, Title: r.Title})
		}
		sort.Slice(refs, func(i, j int) bool { return refs[i].Key < refs[j].Key })
		return refs
	}
	if kind == "dialog" && len(m.dialog.chosen) > 0 {
		return sortedRefs(m.dialog.chosen)
	}
	selected := map[string]model.TaskReference{}
	for id, r := range m.extraChosen {
		if strings.HasPrefix(id, m.selectionScope()+"|") {
			selected[id] = r
		}
	}
	if len(selected) > 0 {
		return sortedRefs(selected)
	}
	if kind == "copilot" && m.copilot.detail != nil {
		return []model.TaskReference{copilotRef(m.copilot.detail.Session)}
	}
	refs := m.visibleReferences()
	i := m.cursor()
	if i >= 0 && i < len(refs) {
		return []model.TaskReference{refs[i]}
	}
	return nil
}
func (m *Model) selectReference(r model.TaskReference, toggle bool) {
	if m.visibleProvider() == "dialog" {
		if toggle {
			if _, ok := m.dialog.chosen[refID(r)]; ok {
				delete(m.dialog.chosen, refID(r))
				return
			}
		}
		m.dialog.chosen[refID(r)] = r
		return
	}
	if m.visibleProvider() == "task" || m.visibleProvider() == "deferred" {
		if m.extraChosen == nil {
			m.extraChosen = map[string]model.TaskReference{}
		}
		id := m.selectionScope() + "|" + refID(r)
		if _, ok := m.extraChosen[id]; ok && toggle {
			delete(m.extraChosen, id)
		} else {
			m.extraChosen[id] = r
		}
		return
	}
	switch r.Provider {
	case "github":
		for _, d := range m.snapshot.Datasets {
			for _, p := range d.PRs {
				if p.URL == r.URL {
					if toggle {
						if _, ok := m.chosen[p.Key()]; ok {
							delete(m.chosen, p.Key())
							return
						}
					}
					m.chosen[p.Key()] = p
					return
				}
			}
		}
	case "jira":
		if m.jira != nil {
			for _, issue := range m.jira.rows {
				if issue.URL == r.URL {
					if toggle {
						if _, ok := m.jiraChosen[issue.Key]; ok {
							delete(m.jiraChosen, issue.Key)
							return
						}
					}
					if m.jiraChosen == nil {
						m.jiraChosen = map[string]model.JiraIssue{}
					}
					m.jiraChosen[issue.Key] = issue
					return
				}
			}
		}
	}
	if m.extraChosen == nil {
		m.extraChosen = map[string]model.TaskReference{}
	}
	id := m.selectionScope() + "|" + refID(r)
	if _, ok := m.extraChosen[id]; ok && toggle {
		delete(m.extraChosen, id)
	} else {
		m.extraChosen[id] = r
	}
}
func (m *Model) allReferences() []model.TaskReference {
	refs := map[string]model.TaskReference{}
	add := func(r model.TaskReference) {
		if r.URL != "" {
			refs[refID(r)] = r
		}
	}
	for _, d := range m.snapshot.Datasets {
		for _, p := range d.PRs {
			add(prRef(p))
		}
	}
	if m.jira != nil {
		for _, i := range m.jira.rows {
			add(model.TaskReference{Provider: "jira", URL: i.URL, Key: i.Key, Title: i.Title})
		}
	}
	if m.copilot != nil {
		for _, s := range m.copilot.sessions {
			add(copilotRef(s))
		}
	}
	if m.mmReader != nil {
		for _, p := range m.mmReader.data.Posts {
			if p.DeletedAt == 0 {
				add(m.postRef(p))
			}
		}
	}
	if m.ensureTaskIDs() == nil {
		for _, t := range m.tasks.items {
			add(taskRef(t))
			for _, r := range t.References {
				add(r)
			}
		}
	}
	if m.entities != nil {
		for _, e := range m.entities.data.Links {
			if _, ok := refs[refID(e.Left)]; !ok && e.Left.Provider != "task" {
				add(e.Left)
			}
			if _, ok := refs[refID(e.Right)]; !ok && e.Right.Provider != "task" {
				add(e.Right)
			}
		}
	}
	return sortedRefs(refs)
}
func (m *Model) relatedReferences(origins []model.TaskReference, kind string) []model.TaskReference {
	return m.relatedReferenceKinds(origins, kind)
}

func (d *entityDialog) filtered() []model.TaskReference {
	var rows []model.TaskReference
	for _, r := range d.rows {
		if strings.Contains(strings.ToLower(r.Key+" "+r.Title), strings.ToLower(d.query)) {
			rows = append(rows, r)
		}
	}
	return rows
}
func (m Model) showEntities(title, mode string, rows, origins []model.TaskReference) (tea.Model, tea.Cmd) {
	if len(rows) == 0 {
		return m.unavailable()
	}
	m.dialog = &entityDialog{title: title, mode: mode, rows: rows, origins: origins, chosen: map[string]model.TaskReference{}}
	return m, nil
}
func (m Model) activateReference(r model.TaskReference) (tea.Model, tea.Cmd) {
	if m.referenceArchived(r) || m.snoozes.hidden(refSnoozeID(r)) {
		return m.showSnapshot(r)
	}
	m.rememberProvider(r.Provider)
	m.snoozes.active = false
	if m.jira != nil {
		m.jira.detail = nil
	}
	m.dialog = nil
	m.detail = nil
	m.tasks.active = false
	m.jiraActive = false
	m.copilotActive = false
	if m.mmReader != nil {
		m.mmReader.active = false
	}
	switch r.Provider {
	case "task":
		for i, t := range m.tasks.items {
			if t.ID == r.Key {
				m.tasks.active = true
				m.tasks.archived = t.Archived
				m.tasks.query = ""
				m.tasks.selected = i
				m.tasks.details = true
				return m, nil
			}
		}
	case "github":
		m.input.SetValue("")
		m.relatedKey = ""
		for v, d := range m.snapshot.Datasets {
			for _, p := range d.PRs {
				if p.URL == r.URL {
					m.active = model.View(v)
					for i, visible := range m.filtered() {
						if visible.URL == r.URL {
							m.selected = i
							return m, nil
						}
					}
				}
			}
		}
	case "jira":
		if m.jira != nil {
			m.jiraActive = true
			m.jira.custom = "key IN (" + r.Key + ") ORDER BY key ASC"
			m.jira.input.SetValue("")
			m.jira.input.Prompt = ""
			m.jira.mode = ""
			return m, m.refreshJira()
		}
	case "copilot":
		if m.copilot != nil {
			m.copilotActive = true
			m.copilot.input.SetValue("")
			for i, s := range m.copilot.rows() {
				if s.ID == r.Key {
					m.copilot.selected = i
					return m, m.updateCopilotDetail()
				}
			}
		}
	case "mattermost":
		if m.mmReader != nil {
			m.mmReader.active = true
			m.mmReader.mentions = false
			for _, p := range m.mmReader.data.Posts {
				if p.ID == r.Key {
					for i, c := range m.mmReader.channels() {
						if c.ID == p.ChannelID {
							m.mmReader.tab = i
							m.mmReader.root = p.RootID
							for j, post := range m.mmReader.posts() {
								if post.ID == p.ID {
									m.mmReader.row = j
									return m, nil
								}
							}
						}
					}
				}
			}
		}
	}
	return m.unavailable()
}
func (m *Model) updateCopilotDetail() tea.Cmd {
	next, cmd := m.updateCopilot(tea.KeyPressMsg{Code: tea.KeyEnter})
	*m = next.(Model)
	return cmd
}
func (m Model) linkReferences(origins, targets []model.TaskReference) (tea.Model, tea.Cmd) {
	if m.localBusy() {
		return m, nil
	}
	origins, targets = underlyingReferences(origins), underlyingReferences(targets)
	if m.entities.err != nil {
		m.sequenceNotice = "Kunne ikke lese koblinger: " + m.entities.err.Error()
		return m, nil
	}
	state := m.entities.data
	state.Links = append([]model.EntityLink(nil), state.Links...)
	seen := map[string]bool{}
	for _, e := range state.Links {
		seen[refID(e.Left)+"\n"+refID(e.Right)] = true
		seen[refID(e.Right)+"\n"+refID(e.Left)] = true
	}
	for _, a := range origins {
		for _, b := range targets {
			if refID(a) == refID(b) || seen[refID(a)+"\n"+refID(b)] {
				continue
			}
			state.Links = append(state.Links, model.EntityLink{Left: a, Right: b})
			seen[refID(a)+"\n"+refID(b)] = true
		}
	}
	tasks := cloneTasks(m.tasks.items)
	for _, a := range origins {
		for _, b := range targets {
			if refID(a) == refID(b) {
				continue
			}
			for i := range tasks {
				if a.Provider == "task" && a.Key == tasks[i].ID {
					tasks[i].Add(b)
				}
				if b.Provider == "task" && b.Key == tasks[i].ID {
					tasks[i].Add(a)
				}
			}
		}
	}
	taskChanged := false
	for _, r := range append(append([]model.TaskReference(nil), origins...), targets...) {
		if r.Provider == "task" {
			taskChanged = true
		}
	}
	if taskChanged && m.tasks.loadFailed {
		m.sequenceNotice = "Oppgaver kunne ikke leses; lagring er sperret"
		return m, nil
	}
	path, taskPath, before := m.entities.path, m.tasks.path, m.entities.data
	var partial bool
	m.sequenceNotice = "Lagrer koblinger…"
	m.enqueueLocal(func() (err error) {
		partial, err = writeLocalState(path, before, state, taskPath, tasks, taskChanged)
		return err
	}, func(m *Model, err error) {
		if err != nil {
			if partial {
				m.entities.data = state
			}
			m.sequenceNotice = "Kunne ikke lagre koblinger: " + err.Error()
			return
		}
		m.entities.data = state
		if taskChanged {
			m.tasks.items = tasks
		}
		m.dialog = nil
		m.sequenceNotice = "Koblinger lagret lokalt"
	})
	return m, nil
}
func refSnoozeID(r model.TaskReference) string {
	if r.SnoozeID != "" {
		return r.SnoozeID
	}
	if r.Provider == "task" {
		return "task:" + r.Key
	}
	return snooze.Identity(r.Provider, r.URL)
}

func (m Model) selectionScope() string {
	kind := m.visibleProvider()
	if kind == "task" && m.tasks.details && len(m.tasks.visibleTasks()) > 0 {
		return "taskrefs:" + m.tasks.items[m.tasks.selected].ID
	}
	return kind
}
func (m Model) extraSelected(r model.TaskReference, scope string) bool {
	_, ok := m.extraChosen[scope+"|"+refID(r)]
	return ok
}

func (m Model) referenceArchived(r model.TaskReference) bool {
	if r.Provider == "task" {
		for _, task := range m.tasks.items {
			if task.ID == r.Key {
				return task.Archived
			}
		}
	}
	return m.entities.archived(r.Provider, r.URL)
}
