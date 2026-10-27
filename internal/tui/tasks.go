package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
	"github.com/haakonph/custom-visning-terminal/internal/tasks"
)

type taskState struct {
	io *localIOState

	snoozes            *snoozeState
	items              []model.Task
	saved              []model.Task
	archived           bool
	selected, row      int
	input              textinput.Model
	editing            bool
	details, searching bool
	query              string
	path, notice       string
	loadFailed         bool
}

func newTasks() taskState {
	t := taskState{input: textinput.New()}
	t.input.Prompt = "Tittel: "
	t.input.CharLimit = 256
	t.path = config.LocalPath()
	err := storage.LoadLocal(t.path, config.TaskPath(), &t.items)
	if errors.Is(err, os.ErrNotExist) {
		return t
	}
	if err != nil {
		t.items = nil
		t.loadFailed = true
		t.notice = "Kunne ikke lese oppgaver; rett oppgavefilen og start på nytt før du lagrer"
	}
	t.saved = tasks.Clone(t.items)
	return t
}

func (t *taskState) save() error {
	if t.loadFailed {
		return errors.New("oppgaver kunne ikke leses")
	}
	if err := tasks.EnsureIDs(t.items); err != nil {
		return err
	}
	changes, err := tasks.Changes(t.saved, t.items)
	if err != nil {
		return err
	}
	store := storage.TaskStore{Path: t.path}
	if t.io == nil {
		result, err := store.ApplyTaskChanges(context.Background(), changes)
		if err == nil {
			t.items = result
			t.saved = tasks.Clone(result)
		}
		return err
	}
	t.notice = "Lagrer oppgaver…"
	var result []model.Task
	t.io.enqueue(func() (err error) { result, err = store.ApplyTaskChanges(context.Background(), changes); return err }, func(m *Model, err error) {
		if err != nil {
			m.tasks.notice = "Kunne ikke lagre oppgaver; endringene finnes kun i minnet: " + err.Error()
		} else {
			m.tasks.items = result
			m.tasks.saved = tasks.Clone(result)
			m.tasks.notice = "Oppgaver lagret lokalt"
		}
	})
	return nil
}

func (m Model) taskReference() (model.TaskReference, bool) {
	if m.page == pageJira {
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
	if m.active.IsReview() {
		role = "review"
	}
	return model.TaskReference{Provider: "github", URL: p.URL, Key: fmt.Sprintf("%s#%d", p.Repository, p.Number), Title: p.Title, Role: role}, true
}

func (m Model) collectTask(create bool) (tea.Model, tea.Cmd) {
	if m.localBusy() {
		return m, nil
	}
	t := &m.tasks
	if t.loadFailed {
		m.setPageFlag(pageTasks, true)
		return m, nil
	}
	refs := m.taskReferences()
	if len(refs) == 0 {
		m.setPageFlag(pageTasks, true)
		t.notice = "Velg en Jira-sak eller PR først, eller trykk n for en tom oppgave"
		return m, nil
	}
	if create || len(t.items) == 0 {
		t.query = ""
		t.items = append(t.items, model.Task{Title: refs[0].Title})
		t.selected = len(t.items) - 1
		t.row = 0
	}
	for _, ref := range refs {
		t.items[t.selected].Add(ref)
	}
	m.setPageFlag(pageTasks, true)
	t.archived = false
	t.items[t.selected].Archived = false
	t.save()
	return m, nil
}

func (m Model) updateTasks(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	t := &m.tasks
	if !t.editing && !t.searching {
		t.selectVisible()
	}
	k := key.String()
	if (k == "n" || k == "D" || k == "d" || (t.editing && k == "enter")) && m.localBusy() {
		return m, nil
	}
	if k == "ctrl+c" {
		return m, m.quit()
	}
	if t.searching {
		if k == "enter" || k == "esc" {
			t.searching = false
			t.input.Blur()
			if k == "esc" {
				t.query = ""
			}
			return m, nil
		}
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(key)
		t.query = t.input.Value()
		t.selectVisible()
		return m, cmd
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
	if len(t.visibleTasks()) == 0 {
		switch k {
		case "enter", "right", "d", "D", "e":
			return m, nil
		}
	}
	switch k {
	case "q":
		return m, m.quit()
	case "esc":
		return m.backTasks()

	case "T":
		m.setPageFlag(pageTasks, false)
	case "g", "J":
		m.setPageFlag(pageTasks, false)
		return m.switchProvider(k == "J")
	case "right":
		t.details = true
	case "left":
		t.details = false
	case "/":
		return m.searchTasks()

	case "tab", "shift+tab":
		delta := 1
		if k == "shift+tab" {
			delta = -1
		}
		t.moveTask(delta, true)
	case "n":
		if t.loadFailed {
			return m, nil
		}
		t.query = ""
		t.archived = false
		t.items = append(t.items, model.Task{Title: "Ny oppgave"})
		t.selected = len(t.items) - 1
		t.row = 0
		t.save()
		fallthrough
	case "E":
		return m.editTaskTitle()

	case "j", "down":
		if !t.details {
			t.moveTask(1, false)
			return m, nil
		}
		if len(t.items) > 0 {
			t.row = min(t.row+1, max(0, len(t.items[t.selected].References)-1))
		}
	case "k", "up":
		if !t.details {
			t.moveTask(-1, false)
			return m, nil
		}
		t.row = max(0, t.row-1)
	case "D":
		if len(t.items) > 0 && !t.loadFailed {
			t.items = append(t.items[:t.selected], t.items[t.selected+1:]...)
			t.selected = max(0, min(t.selected, len(t.items)-1))
			t.row = 0
			t.selectVisible()
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
		if !t.details {
			t.details = true
			return m, nil
		}
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

func (t taskState) visibleTasks() []int {
	var indices []int
	for i, task := range t.items {
		if task.Archived != t.archived {
			continue
		}
		if task.ID != "" && t.snoozes.hidden("task:"+task.ID) {
			continue
		}
		text := task.Title
		for _, ref := range task.References {
			text += " " + ref.Key + " " + ref.Title
		}
		if strings.Contains(strings.ToLower(text), strings.ToLower(strings.TrimSpace(t.query))) {
			indices = append(indices, i)
		}
	}
	return indices
}

func (t *taskState) selectVisible() {
	indices := t.visibleTasks()
	for _, i := range indices {
		if i == t.selected {
			return
		}
	}
	if len(indices) > 0 {
		t.selected = indices[0]
		t.row = 0
	}
}

func (t *taskState) moveTask(delta int, wrap bool) {
	indices := t.visibleTasks()
	if len(indices) == 0 {
		return
	}
	pos := 0
	for i, index := range indices {
		if index == t.selected {
			pos = i
			break
		}
	}
	if wrap {
		pos = (pos + delta + len(indices)) % len(indices)
	} else {
		pos = max(0, min(pos+delta, len(indices)-1))
	}
	t.selected = indices[pos]
	t.row = 0
}

func (m Model) tasksView() tea.View {
	t := m.tasks
	lines := []string{accent.Render(" CUSTOM VISNING · OPPGAVER"), "vg GitHub · vt Jira · / søk · h/l oppgave · d detaljer · ae endre tittel · ak arkiver · vb vis arkiv"}
	if t.archived {
		lines = append(lines, " Arkiverte oppgaver · ak gjenopprett · vo aktive")
	}
	lines = append(lines, m.snoozeLines()...)
	if t.searching || t.editing {
		lines = append(lines, t.input.View())
	} else if t.query != "" {
		lines = append(lines, "Søk: "+clean(t.query))
	}
	footer := "j/k velg · d detaljer · en åpne · an ny · ae tittel · d detaljer · ad slett · ak arkiver · vb vis arkiv · Esc tilbake · Ctrl+C avslutt"
	capacity := max(1, m.height-len(lines)-2)
	indices := t.visibleTasks()
	list := []string{accent.Render(fmt.Sprintf(" OPPGAVER (%d/%d)", len(indices), len(t.items)))}
	pos := 0
	for i, index := range indices {
		if index == t.selected {
			pos = i
		}
	}
	start := max(0, pos-max(1, capacity-1)+1)
	for i := start; i < min(len(indices), start+max(1, capacity-1)); i++ {
		index := indices[i]
		prefix := "  "
		if index == t.selected {
			prefix = "› "
		}
		if m.extraSelected(taskRef(t.items[index]), "task") {
			prefix += "✓ "
		}
		line := clean(fmt.Sprintf("%s%s (%d)", prefix, t.items[index].Title, len(t.items[index].References)))
		list = append(list, line)
	}
	if len(indices) == 0 {
		list = append(list, " Ingen oppgaver funnet.", " an lager en ny oppgave")
	}
	var details []string
	if len(indices) > 0 {
		task := t.items[t.selected]
		details = append(details, accent.Render(clean(task.Title)), "vg/vt: finn referanser, to legger til her")
		refsCapacity := max(1, capacity-2)
		refStart := max(0, t.row-refsCapacity+1)
		for i := refStart; i < min(len(task.References), refStart+refsCapacity); i++ {
			r := m.displayReference(m.liveReference(task.References[i]))
			if entry, ok := m.snoozes.effective(refSnoozeID(r)); ok {
				r.Title += " · utsatt til " + entry.Until.Local().Format("2006-01-02 15:04 MST")
				if len(entry.Roots) > 0 {
					r.Title += " via " + entry.Title
				}
			}
			prefix := "  "
			if i == t.row {
				prefix = "› "
			}
			if m.extraSelected(r, "taskrefs:"+task.ID) {
				prefix += "✓ "
			}
			line := clean(fmt.Sprintf("%s%s · %s · %s %s", prefix, r.Provider, r.Key, r.Title, r.Role))
			details = append(details, line)
		}
		if len(task.References) == 0 {
			details = append(details, " Ingen referanser ennå.")
		}
	}
	if m.width >= 90 {
		left := min(38, m.width/3)
		right := max(1, m.width-left-3)
		for i := 0; i < capacity; i++ {
			l, r := "", ""
			if i < len(list) {
				l = list[i]
			}
			if i < len(details) {
				r = details[i]
			}
			l = ansi.Truncate(l, left, "…")
			l += strings.Repeat(" ", max(0, left-ansi.StringWidth(l)))
			r = ansi.Truncate(r, right, "…")
			if !t.details && i > 0 && i-1+start == pos && len(indices) > 0 {
				l = selection.Render(l)
			}
			if t.details && i == 2+t.row-max(0, t.row-max(1, capacity-2)+1) && len(indices) > 0 && len(t.items[t.selected].References) > 0 {
				r = selection.Render(r)
			}
			lines = append(lines, l+muted.Render(" │ ")+r)
		}
	} else {
		panel := list
		if t.details {
			panel = details
		}
		for _, line := range panel {
			lines = append(lines, line)
		}
	}
	if t.notice != "" {
		lines = append(lines, warning.Render(clean(t.notice)))
	}
	lines = append(lines, footer)
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
		for _, data := range m.prs.Snapshot().Datasets {
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

func (m Model) backTasks() (tea.Model, tea.Cmd) {
	t := &m.tasks

	if t.details {
		t.details = false
	} else if t.query != "" {
		t.query = ""
	} else {
		m.setPageFlag(pageTasks, false)
	}
	return m, nil
}

func (m Model) searchTasks() (tea.Model, tea.Cmd) {
	t := &m.tasks

	t.searching = true
	t.details = false
	t.input.Prompt = "/ "
	t.input.SetValue(t.query)
	return m, t.input.Focus()
}

func (m Model) editTaskTitle() (tea.Model, tea.Cmd) {
	t := &m.tasks

	if len(t.items) > 0 && !t.loadFailed {
		t.editing = true
		t.input.Prompt = "Tittel: "
		t.input.SetWidth(max(1, m.width-10))
		t.input.SetValue(t.items[t.selected].Title)
		return m, t.input.Focus()
	}
	return m, nil
}
