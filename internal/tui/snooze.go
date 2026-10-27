package tui

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/app"
	"github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
	taskdomain "github.com/haakonph/custom-visning-terminal/internal/tasks"
)

type snoozeState struct {
	visibilityVersion uint64

	contextDirty bool

	store    snooze.Store
	entries  map[string]snooze.Entry
	coverage map[string][]string
	at       time.Time
	now      func() time.Time
	loadErr  error
	input    bool
	whole    bool
	origins  []model.TaskReference
	digits   string
	target   snooze.Entry
	targets  []snooze.Entry
	active   bool
	selected int
	notice   string
}

func newSnooze() *snoozeState {
	s := &snoozeState{now: time.Now, at: time.Now()}
	path := config.LocalPath()
	s.store = snooze.Store{Path: path}
	s.loadErr = storage.LoadLocal(path, config.SnoozePath(), &s.entries)
	if errors.Is(s.loadErr, os.ErrNotExist) {
		s.loadErr = nil
	}
	if s.entries == nil {
		s.entries = map[string]snooze.Entry{}
	}
	s.indexContexts()
	if s.loadErr != nil {
		s.notice = "Kunne ikke lese snooze: " + s.loadErr.Error()
	}
	return s
}

func (s *snoozeState) hidden(id string) bool {
	if s == nil {
		return false
	}
	_, ok := s.effective(id)
	return ok
}

func (s *snoozeState) effective(id string) (snooze.Entry, bool) {
	var result snooze.Entry
	if e, ok := s.entries[id]; ok && e.Until.After(s.at) {
		result = e
	}
	for _, groupID := range s.coverage[id] {
		e := s.entries[groupID]
		if e.Until.After(s.at) && (result.ID == "" || e.Until.After(result.Until) || (e.Until.Equal(result.Until) && e.ID < result.ID)) {
			result = e
		}
	}
	return result, result.ID != ""
}

func (s *snoozeState) indexContexts() {
	s.coverage = map[string][]string{}
	for _, e := range s.entries {
		for _, r := range e.Members {
			id := refSnoozeID(r)
			s.coverage[id] = append(s.coverage[id], e.ID)
		}
	}
}
func (s *snoozeState) rows() []snooze.Entry { return snooze.Active(s.entries, s.at) }

type snoozeTick struct{}

func snoozeTimer() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return snoozeTick{} })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.initProviders(), refreshTimer(), snoozeTimer(), m.mmStart(), mmTimer())
}

func (m Model) snoozeLines() []string {
	s := m.snoozes
	if s == nil {
		return nil
	}
	line := fmt.Sprintf(" %d utsatte · z snooze · zz hele sammenhengen · vz vis", len(s.rows()))
	if s.input {
		line = " Snooze: " + s.digits + "▏  h timer · d dager · m dager kl. 06 · Esc avbryt"
	}
	lines := []string{line}
	if s.notice != "" {
		lines = append(lines, warning.Render(clean(s.notice)))
	}
	return lines
}

func (m *Model) snoozeTarget() (snooze.Entry, bool) {
	s := m.snoozes
	if s.active {
		rows := s.rows()
		if s.selected < len(rows) {
			return rows[s.selected], true
		}
		return snooze.Entry{}, false
	}
	if m.page == pageTasks {
		t := &m.tasks
		if len(t.visibleTasks()) == 0 || t.loadFailed {
			return snooze.Entry{}, false
		}
		t.selectVisible()
		task := &t.items[t.selected]
		if task.ID == "" {
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				s.notice = err.Error()
				return snooze.Entry{}, false
			}
			return snooze.Entry{ID: fmt.Sprintf("task:%x", id), Kind: "task", Title: task.Title}, true
		}
		return snooze.Entry{ID: "task:" + task.ID, Kind: "task", Title: task.Title}, true
	}
	ref, ok := m.taskReference()
	if !ok {
		return snooze.Entry{}, false
	}
	id := snooze.Identity(ref.Provider, ref.URL)
	return snooze.Entry{ID: id, Kind: ref.Provider, Title: ref.Title, URL: ref.URL}, id != ""
}

// All writes complete before the in-memory filter changes. A failed write
// leaves both the current list and the previous deadline untouched.
func (s *snoozeState) commit(entry snooze.Entry, remove bool) error {
	return s.commitEntries([]snooze.Entry{entry}, remove)
}

func (s *snoozeState) commitEntries(batch []snooze.Entry, remove bool) error {
	if s.loadErr != nil {
		return s.loadErr
	}
	entries := make(map[string]snooze.Entry, len(s.entries)+1)
	for id, e := range s.entries {
		entries[id] = e
	}
	for _, entry := range batch {
		if remove {
			delete(entries, entry.ID)
		} else {
			entries[entry.ID] = entry
		}
	}
	result, err := (storage.LocalStore{Path: s.store.Path}).CommitLocal(context.Background(), app.LocalChanges{Snoozes: snooze.Changes(s.entries, entries)})
	if err != nil {
		return err
	}
	s.entries = result.Snoozes
	s.visibilityVersion++
	s.indexContexts()
	return nil
}

type snoozeAnchor struct {
	pr, jira      string
	task, taskPos int
	deferred      string
}

func (m Model) snoozeAnchor() snoozeAnchor {
	a := snoozeAnchor{task: m.tasks.selected}
	if rows := m.filtered(); m.selected >= 0 && m.selected < len(rows) {
		a.pr = rows[m.selected].URL
	}
	if m.jira != nil {
		if rows := m.jira.filtered(); m.jira.selected >= 0 && m.jira.selected < len(rows) {
			a.jira = rows[m.jira.selected].URL
		}
	}
	for pos, index := range m.tasks.visibleTasks() {
		if index == a.task {
			a.taskPos = pos
		}
	}
	if rows := m.snoozes.rows(); m.snoozes.selected < len(rows) {
		a.deferred = rows[m.snoozes.selected].ID
	}
	return a
}
func (m *Model) restoreSnoozeAnchor(a snoozeAnchor) {
	for i, p := range m.filtered() {
		if p.URL == a.pr {
			m.selected = i
			break
		}
	}
	m.clamp()
	if m.jira != nil {
		for i, r := range m.jira.filtered() {
			if r.URL == a.jira {
				m.jira.selected = i
				break
			}
		}
		m.jira.clamp()
	}
	indices := m.tasks.visibleTasks()
	found := false
	for _, i := range indices {
		if i == a.task {
			found = true
		}
	}
	if !found && len(indices) > 0 {
		m.tasks.selected = indices[min(a.taskPos, len(indices)-1)]
		m.tasks.row = 0
	}
	rows := m.snoozes.rows()
	for i, r := range rows {
		if r.ID == a.deferred {
			m.snoozes.selected = i
			break
		}
	}
	m.snoozes.selected = max(0, min(m.snoozes.selected, len(rows)-1))
}

func (m *Model) snoozeMessage(msg tea.Msg) (bool, tea.Cmd) {
	s := m.snoozes
	if s == nil {
		return false, nil
	}
	if _, ok := msg.(snoozeTick); ok {
		now := s.now()
		expired := false
		for _, e := range s.entries {
			if e.Until.After(s.at) && !e.Until.After(now) {
				expired = true
				break
			}
		}
		if !expired {
			s.at = now
			m.refreshSnoozeContexts()
			return true, snoozeTimer()
		}
		a := m.snoozeAnchor()
		s.at = now
		s.visibilityVersion++
		m.refreshSnoozeContexts()
		m.restoreSnoozeAnchor(a)
		return true, snoozeTimer()
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return false, nil
	}
	k := key.String()
	if s.input {
		switch k {
		case "esc":
			s.input, s.digits = false, ""
			s.targets, s.origins, s.whole = nil, nil, false
			s.notice = ""
		case "backspace":
			if len(s.digits) > 0 {
				s.digits = s.digits[:len(s.digits)-1]
			}
			s.notice = ""
		case "z":
			m.expandSnooze()
		case "h", "d", "m", "D":
			unit := k
			if unit == "m" {
				unit = "D"
			}
			digits := s.digits
			if digits == "" {
				digits = "1"
			}
			at, err := snooze.Deadline(s.now().In(time.Local), digits, unit)
			if err != nil {
				s.notice = err.Error()
				return true, nil
			}
			if m.localBusy() {
				s.notice = "Venter på lokal lagring…"
				return true, nil
			}
			entry := s.target
			entry.Until = at
			var tasks []model.Task
			if !s.active && (m.page == pageTasks) && len(m.tasks.items) > 0 && m.tasks.items[m.tasks.selected].ID == "" {
				tasks = cloneTasks(m.tasks.items)
				tasks[m.tasks.selected].ID = strings.TrimPrefix(entry.ID, "task:")
			}
			a := m.snoozeAnchor()
			batch := append([]snooze.Entry(nil), s.targets...)
			if len(batch) == 0 {
				batch = []snooze.Entry{entry}
			}
			for i := range batch {
				batch[i].Until = at
			}
			m.queueSnoozeTasks(batch, false, tasks, func(m *Model, err error) {
				s := m.snoozes
				if err != nil {
					s.notice = "Kunne ikke lagre snooze: " + err.Error()
					return
				}
				for _, e := range batch {
					if e.Kind == "jira" {
						for key, issue := range m.jiraChosen {
							if issue.URL == e.URL {
								delete(m.jiraChosen, key)
							}
						}
					}
					if e.Kind == "github" {
						for key := range m.chosen {
							p, _ := m.prs.Lookup(key)
							if p.URL == e.URL {
								delete(m.chosen, key)
							}
						}
					}
				}
				m.clearChosen()
				s.targets = nil
				s.origins, s.whole = nil, false
				s.input, s.digits = false, ""
				s.notice = "Utsatt til " + at.Local().Format("2006-01-02 15:04:05 MST -07:00")
				m.restoreSnoozeAnchor(a)
			})
		case "ctrl+c":
			return true, m.quit()
		default:
			if len(k) == 1 && k[0] >= '0' && k[0] <= '9' && len(s.digits) < 256 {
				s.digits += k
				s.notice = ""
			} else {
				if len(s.digits) == 256 {
					s.digits += "!"
				}
				s.notice = "Ugyldig verdi; bruk sifre og h, d eller m"
			}
		}
		return true, nil
	}
	if s.active {
		switch k {
		case "esc", "Z":
			s.active = false
		case "j", "down":
			s.selected = min(s.selected+1, max(0, len(s.rows())-1))
		case "k", "up":
			s.selected = max(0, s.selected-1)
		case "u":
			if entry, ok := m.snoozeTarget(); ok {
				a := m.snoozeAnchor()
				if m.localBusy() {
					return true, nil
				}
				m.queueSnooze([]snooze.Entry{entry}, true, func(m *Model, err error) {
					if err != nil {
						m.snoozes.notice = "Kunne ikke hente tilbake: " + err.Error()
						return
					}
					m.snoozes.notice = "Hentet tilbake: " + entry.Title
					m.restoreSnoozeAnchor(a)
				})
			}
		case "z":
			if entry, ok := m.snoozeTarget(); ok {
				s.target, s.input, s.digits, s.notice = entry, true, "", ""
				s.targets = nil
			}
		case "ctrl+c", "q":
			return true, m.quit()
		}
		return true, nil
	}
	if (m.page == pageCopilot) || (m.local != nil && m.local.choosing) || ((m.page == pageTasks) && (m.tasks.editing || m.tasks.searching)) || (!(m.page == pageTasks) && (m.filtering || ((m.page == pageJira) && m.jira.mode != ""))) {
		return false, nil
	}
	switch k {
	case "Z":
		s.active = true
		s.selected = 0
		return true, nil
	case "z":
		if s.loadErr != nil {
			s.notice = "Kunne ikke lese snooze: " + s.loadErr.Error()
			return true, nil
		}
		if !(m.page == pageTasks) {
			s.targets = nil
			for _, ref := range m.taskReferences() {
				s.targets = append(s.targets, snooze.Entry{ID: snooze.Identity(ref.Provider, ref.URL), Kind: ref.Provider, Title: ref.Title, URL: ref.URL})
			}
			if len(s.targets) > 0 {
				s.target, s.input, s.digits, s.notice = s.targets[0], true, "", ""
			}
		} else if entry, ok := m.snoozeTarget(); ok {
			s.targets = nil
			s.target, s.input, s.digits, s.notice = entry, true, "", ""
		}
		return true, nil
	}
	return false, nil
}

func (m Model) snoozeView() tea.View {
	s := m.snoozes
	lines := []string{accent.Render(" CUSTOM VISNING · UTSATT"), " TYPE · TITTEL · RETURTIDSPUNKT"}
	lines = append(lines, m.snoozeLines()...)
	rows := s.rows()
	capacity := max(1, m.height-len(lines)-1)
	start := max(0, s.selected-capacity+1)
	for i := start; i < min(len(rows), start+capacity); i++ {
		r := rows[i]
		if r.Kind == "github" {
			r.Title = m.displayReference(model.TaskReference{Provider: "github", Title: r.Title}).Title
		}
		line := fmt.Sprintf(" %s · %s · %s", r.Kind, clean(r.Title), r.Until.Local().Format("2006-01-02 15:04:05 MST -07:00"))
		if len(r.Roots) > 0 {
			line = fmt.Sprintf(" gruppe · %s · %s · %s", clean(r.Title), contextSummary(r.Members), r.Until.Local().Format("2006-01-02 15:04 MST"))
		}
		if i == s.selected {
			line = selection.Render(line)
		}
		lines = append(lines, line)
	}
	if len(rows) == 0 {
		lines = append(lines, " Ingen utsatte elementer.")
	}
	lines = append(lines, "j/k velg · z endre tidspunkt · az hent tilbake · Esc tilbake")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "…")
	}
	if len(lines) > m.height {
		lines = append(lines[:max(0, m.height-1)], lines[len(lines)-1])
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.WindowTitle = "custom-visning · Utsatt"
	return v
}

func (m *Model) queueSnooze(batch []snooze.Entry, remove bool, finish func(*Model, error)) {
	m.queueSnoozeTasks(batch, remove, nil, finish)
}
func (m *Model) queueSnoozeTasks(batch []snooze.Entry, remove bool, tasks []model.Task, finish func(*Model, error)) {
	s := m.snoozes
	if s.loadErr != nil {
		finish(m, s.loadErr)
		return
	}
	entries := make(map[string]snooze.Entry, len(s.entries)+len(batch))
	for id, e := range s.entries {
		entries[id] = e
	}
	for _, e := range batch {
		if remove {
			delete(entries, e.ID)
		} else {
			entries[e.ID] = e
		}
	}
	changes := app.LocalChanges{Snoozes: snooze.Changes(s.entries, entries)}
	if tasks != nil {
		if err := taskdomain.EnsureIDs(tasks); err != nil {
			finish(m, err)
			return
		}
		var err error
		changes.Tasks, err = taskdomain.Changes(m.tasks.saved, tasks)
		if err != nil {
			finish(m, err)
			return
		}
	}
	s.notice = "Lagrer utsettelse…"
	m.commitLocal(changes, tasks != nil, finish)
}
