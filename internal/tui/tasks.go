package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type taskState struct {
	items         []model.Task
	active        bool
	selected, row int
	input         textinput.Model
	editing       bool
	path, notice  string
	loadFailed    bool
}

func newTasks() taskState {
	t := taskState{input: textinput.New()}
	t.input.Prompt = "Tittel: "
	t.input.CharLimit = 256
	t.path = os.Getenv("CUSTOM_VISNING_TASKS")
	if t.path == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			t.path = filepath.Join(dir, "custom-visning", "tasks.json")
		}
	}
	data, err := os.ReadFile(t.path)
	if errors.Is(err, os.ErrNotExist) {
		return t
	}
	if err == nil {
		err = json.Unmarshal(data, &t.items)
	}
	if err != nil {
		t.items = nil
		t.loadFailed = true
		t.notice = "Kunne ikke lese oppgaver; rett oppgavefilen og start på nytt før du lagrer"
	}
	return t
}

func (t *taskState) save() {
	if t.loadFailed {
		return
	}
	err := func() error {
		if t.path == "" {
			return errors.New("missing path")
		}
		if err := os.MkdirAll(filepath.Dir(t.path), 0700); err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(t.path), ".tasks-*.json")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if err = json.NewEncoder(f).Encode(t.items); err != nil {
			f.Close()
			return err
		}
		if err = f.Sync(); err != nil {
			f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		return os.Rename(f.Name(), t.path)
	}()
	if err != nil {
		t.notice = "Kunne ikke lagre oppgaver; endringene finnes kun i minnet"
	} else {
		t.notice = "Oppgaver lagret lokalt"
	}
}

func (m Model) taskReference() (model.TaskReference, bool) {
	if m.jiraActive {
		rows := m.jira.filtered()
		if m.jira.selected < 0 || m.jira.selected >= len(rows) {
			return model.TaskReference{}, false
		}
		r := rows[m.jira.selected]
		return model.TaskReference{Provider: "jira", URL: r.URL, Key: r.Key, Title: r.Title}, true
	}
	rows := m.filtered()
	if m.selected < 0 || m.selected >= len(rows) {
		return model.TaskReference{}, false
	}
	p := rows[m.selected]
	role := ""
	if m.active == model.Reviews || m.active == model.TeamReviews {
		role = "review"
	}
	return model.TaskReference{Provider: "github", URL: p.URL, Key: fmt.Sprintf("%s#%d", p.Repository, p.Number), Title: p.Title, Role: role}, true
}

func (m Model) collectTask(create bool) (tea.Model, tea.Cmd) {
	t := &m.tasks
	if t.loadFailed {
		t.active = true
		return m, nil
	}
	ref, ok := m.taskReference()
	if !ok {
		t.active = true
		t.notice = "Velg en Jira-sak eller PR først, eller trykk n for en tom oppgave"
		return m, nil
	}
	if create || len(t.items) == 0 {
		t.items = append(t.items, model.Task{Title: ref.Title})
		t.selected = len(t.items) - 1
		t.row = 0
	}
	t.items[t.selected].Add(ref)
	t.active = true
	t.save()
	return m, nil
}

func (m Model) updateTasks(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	t := &m.tasks
	k := key.String()
	if k == "ctrl+c" {
		m.cancel()
		return m, tea.Quit
	}
	if t.editing {
		switch k {
		case "esc":
			t.editing = false
			t.input.Blur()
			return m, nil
		case "enter":
			if title := strings.TrimSpace(t.input.Value()); title != "" {
				t.items[t.selected].Title = title
				t.save()
			}
			t.editing = false
			t.input.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(key)
		return m, cmd
	}
	switch k {
	case "q":
		m.cancel()
		return m, tea.Quit
	case "esc", "T":
		t.active = false
	case "g", "J":
		t.active = false
		return m.switchProvider(k == "J")
	case "tab", "right", "shift+tab", "left":
		if len(t.items) > 0 {
			delta := 1
			if k == "shift+tab" || k == "left" {
				delta = -1
			}
			t.selected = (t.selected + delta + len(t.items)) % len(t.items)
			t.row = 0
		}
	case "n":
		if t.loadFailed {
			return m, nil
		}
		t.items = append(t.items, model.Task{Title: "Ny oppgave"})
		t.selected = len(t.items) - 1
		t.row = 0
		t.save()
		fallthrough
	case "e":
		if len(t.items) > 0 && !t.loadFailed {
			t.editing = true
			t.input.SetWidth(max(1, m.width-10))
			t.input.SetValue(t.items[t.selected].Title)
			return m, t.input.Focus()
		}
	case "j", "down":
		if len(t.items) > 0 {
			t.row = min(t.row+1, max(0, len(t.items[t.selected].References)-1))
		}
	case "k", "up":
		t.row = max(0, t.row-1)
	case "D":
		if len(t.items) > 0 && !t.loadFailed {
			t.items = append(t.items[:t.selected], t.items[t.selected+1:]...)
			t.selected = max(0, min(t.selected, len(t.items)-1))
			t.row = 0
			t.save()
		}
	case "d":
		if len(t.items) > 0 && !t.loadFailed {
			refs := &t.items[t.selected].References
			if t.row < len(*refs) {
				*refs = append((*refs)[:t.row], (*refs)[t.row+1:]...)
				t.row = max(0, min(t.row, len(*refs)-1))
				t.save()
			}
		}
	case "enter":
		if len(t.items) == 0 || t.row >= len(t.items[t.selected].References) {
			return m, nil
		}
		ref := t.items[t.selected].References[t.row]
		if ref.Provider == "github" {
			return m, func() tea.Msg { return openMsg{err: m.open(m.ctx, ref.URL)} }
		}
		if ref.Provider == "jira" && m.jira != nil {
			return m, func() tea.Msg { return openMsg{err: openJiraBrowser(m.ctx, m.jira.client, ref.URL)} }
		}
		t.notice = "Konfigurer Jira for å åpne denne referansen"
	}
	return m, nil
}

func (m Model) tasksView() tea.View {
	t := m.tasks
	lines := []string{accent.Render(" CUSTOM VISNING · OPPGAVER"), " Tab / Shift+Tab velger oppgave · g GitHub · J Jira · Esc tilbake"}
	if len(t.items) == 0 {
		lines = append(lines, " Ingen oppgaver ennå. Bruk t på en Jira-sak eller PR, eller n for en tom oppgave.")
	} else {
		task := t.items[t.selected]
		lines = append(lines, accent.Render(clean(fmt.Sprintf(" %d/%d · %s", t.selected+1, len(t.items), task.Title))))
		if t.editing {
			lines = append(lines, t.input.View())
		}
		lines = append(lines, " g / J: finn flere saker eller PR-er, trykk a for å legge til i denne oppgaven.")
		capacity := max(1, m.height-len(lines)-3)
		start := max(0, t.row-capacity+1)
		for i := start; i < min(len(task.References), start+capacity); i++ {
			r := m.liveReference(task.References[i])
			line := clean(fmt.Sprintf(" %s · %s · %s %s", r.Provider, r.Key, r.Title, r.Role))
			if i == t.row {
				line = selection.Render(line)
			}
			lines = append(lines, line)
		}
		if len(task.References) == 0 {
			lines = append(lines, " Ingen referanser. Oppgaven kan fylles ut senere.")
		}
	}
	if t.notice != "" {
		lines = append(lines, warning.Render(clean(t.notice)))
	}
	lines = append(lines, "n ny · e tittel · d fjern kobling · D slett oppgave · j/k velg · Enter åpne · T/Esc tilbake · q avslutt")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "…")
	}
	if len(lines) > m.height {
		lines = append(lines[:max(0, m.height-1)], lines[len(lines)-1])
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.WindowTitle = "custom-visning · Oppgaver"
	return v
}

// Prefer loaded provider data while retaining the saved reference when offline.
func (m Model) liveReference(ref model.TaskReference) model.TaskReference {
	if ref.Provider == "github" {
		for _, data := range m.snapshot.Datasets {
			for _, p := range data.PRs {
				if p.URL == ref.URL {
					ref.Title = p.Title + " · review: " + string(p.Review) + " · CI: " + string(p.CI)
					return ref
				}
			}
		}
	}
	if ref.Provider == "jira" && m.jira != nil {
		for _, issue := range m.jira.rows {
			if issue.URL == ref.URL {
				ref.Title = issue.Title + " · " + issue.Status
				return ref
			}
		}
	}
	return ref
}
