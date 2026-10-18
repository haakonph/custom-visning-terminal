package tui

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
)

type snoozeState struct {
	store    snooze.Store
	entries  map[string]snooze.Entry
	at       time.Time
	now      func() time.Time
	loadErr  error
	input    bool
	digits   string
	target   snooze.Entry
	targets  []snooze.Entry
	active   bool
	selected int
	notice   string
}

func newSnooze() *snoozeState {
	s := &snoozeState{now: time.Now, at: time.Now()}
	path := os.Getenv("CUSTOM_VISNING_SNOOZE")
	if path == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			path = filepath.Join(dir, "custom-visning", "snooze.json")
		}
	}
	s.store = snooze.Store{Path: path}
	s.entries, s.loadErr = s.store.Load()
	if s.loadErr != nil {
		s.notice = "Kunne ikke lese snooze: " + s.loadErr.Error()
	}
	return s
}

func (s *snoozeState) hidden(id string) bool {
	if s == nil {
		return false
	}
	e, ok := s.entries[id]
	return ok && e.Until.After(s.at)
}
func (s *snoozeState) rows() []snooze.Entry { return snooze.Active(s.entries, s.at) }

type snoozeTick struct{}

func snoozeTimer() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return snoozeTick{} })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.initProviders(), snoozeTimer(), m.mmStart(), mmTimer())
}

func (m Model) snoozeLines() []string {
	s := m.snoozes
	if s == nil {
		return nil
	}
	line := fmt.Sprintf(" %d utsatte · z snooze · Z vis", len(s.rows()))
	if s.input {
		line = " Snooze: " + s.digits + "▏  h timer · d dager · D dager kl. 06 · Esc avbryt"
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
	if m.tasks.active {
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
	if err := s.store.Save(entries); err != nil {
		return err
	}
	s.entries = entries
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
		a := m.snoozeAnchor()
		s.at = s.now()
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
			s.notice = ""
		case "backspace":
			if len(s.digits) > 0 {
				s.digits = s.digits[:len(s.digits)-1]
			}
			s.notice = ""
		case "h", "d", "D":
			at, err := snooze.Deadline(s.now().In(time.Local), s.digits, k)
			if err != nil {
				s.notice = err.Error()
				return true, nil
			}
			entry := s.target
			entry.Until = at
			if !s.active && m.tasks.active && m.tasks.items[m.tasks.selected].ID == "" {
				task := &m.tasks.items[m.tasks.selected]
				task.ID = strings.TrimPrefix(entry.ID, "task:")
				if err := m.tasks.save(); err != nil {
					task.ID = ""
					s.notice = "Kunne ikke lagre oppgave-ID: " + err.Error()
					return true, nil
				}
			}
			a := m.snoozeAnchor()
			batch := append([]snooze.Entry(nil), s.targets...)
			if len(batch) == 0 {
				batch = []snooze.Entry{entry}
			}
			for i := range batch {
				batch[i].Until = at
			}
			if err := s.commitEntries(batch, false); err != nil {
				s.notice = "Kunne ikke lagre snooze: " + err.Error()
				return true, nil
			}
			for _, e := range batch {
				if e.Kind == "github" {
					for key, p := range m.chosen {
						if p.URL == e.URL {
							delete(m.chosen, key)
						}
					}
				}
			}
			s.targets = nil
			s.input, s.digits = false, ""
			s.notice = "Utsatt til " + at.Local().Format("2006-01-02 15:04:05 MST -07:00")
			m.restoreSnoozeAnchor(a)
		case "ctrl+c":
			m.cancel()
			return true, tea.Quit
		default:
			if len(k) == 1 && k[0] >= '0' && k[0] <= '9' && len(s.digits) < 256 {
				s.digits += k
				s.notice = ""
			} else {
				if len(s.digits) == 256 {
					s.digits += "!"
				}
				s.notice = "Ugyldig verdi; bruk sifre og h, d eller D"
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
				if err := s.commit(entry, true); err != nil {
					s.notice = "Kunne ikke hente tilbake: " + err.Error()
				} else {
					s.notice = "Hentet tilbake: " + entry.Title
					m.restoreSnoozeAnchor(a)
				}
			}
		case "z":
			if entry, ok := m.snoozeTarget(); ok {
				s.target, s.input, s.digits, s.notice = entry, true, "", ""
				s.targets = nil
			}
		case "ctrl+c", "q":
			m.cancel()
			return true, tea.Quit
		}
		return true, nil
	}
	if m.copilotActive || (m.local != nil && m.local.choosing) || (m.tasks.active && (m.tasks.editing || m.tasks.searching)) || (!m.tasks.active && (m.filtering || (m.jiraActive && m.jira.mode != ""))) {
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
		if !m.tasks.active && !m.jiraActive {
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
		line := fmt.Sprintf(" %s · %s · %s", r.Kind, clean(r.Title), r.Until.Local().Format("2006-01-02 15:04:05 MST -07:00"))
		if i == s.selected {
			line = selection.Render(line)
		}
		lines = append(lines, line)
	}
	if len(rows) == 0 {
		lines = append(lines, " Ingen utsatte elementer.")
	}
	lines = append(lines, "j/k velg · z endre tidspunkt · u hent tilbake · Esc tilbake")
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
