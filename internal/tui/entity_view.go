package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"strings"
)

func (m Model) entityView() tea.View {
	d := m.dialog
	if d.mode == "snapshot" {
		return m.entitySnapshotView()
	}
	rows := d.filtered()
	lines := []string{d.title, m.keyHints("j/k velg · Space marker · Shift+↑/↓ utvid · / søk · Enter velg · Esc avbryt")}
	if d.searching {
		lines = append(lines, "/ "+clean(d.query)+"▏")
	} else if d.query != "" {
		lines = append(lines, "/ "+clean(d.query))
	}
	lines = append(lines, m.entityTableRow(model.TaskReference{}, true, false, false))
	// In short windows, reserve a line for the selected item before showing
	// the title, shortcuts or query.
	if len(lines) >= max(1, m.height) {
		header := lines[len(lines)-1]
		lines = lines[:max(0, m.height-2)]
		if m.height > 1 {
			lines = append(lines, header)
		}
	}
	capacity := max(1, m.height-len(lines))
	start := max(0, d.selected-capacity+1)
	for i := start; i < min(len(rows), start+capacity); i++ {
		_, chosen := d.chosen[refID(rows[i])]
		lines = append(lines, m.entityTableRow(rows[i], false, i == d.selected, chosen))
	}
	if len(lines) > m.height {
		lines = lines[:max(1, m.height)]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

// Resolve users from loaded provider data so navigation does not need network
// requests or changes to the persisted reference snapshots.
func (m Model) referenceUser(r model.TaskReference) string {
	switch r.Provider {
	case "github":
		for _, dataset := range m.snapshot.Datasets {
			for _, pr := range dataset.PRs {
				if pr.URL == r.URL && pr.Author != "" {
					return pr.Author
				}
			}
		}
	case "jira":
		if m.jira != nil {
			for _, issue := range m.jira.rows {
				if (r.URL != "" && issue.URL == r.URL) || (r.Key != "" && issue.Key == r.Key) {
					if issue.Assignee != "" {
						return issue.Assignee
					}
				}
			}
		}
	case "mattermost":
		if m.mmReader != nil {
			for _, post := range m.mmReader.data.Posts {
				if post.ID == r.Key {
					if post.Author != "" {
						return post.Author
					}
					if post.UserID != "" {
						return post.UserID
					}
				}
			}
		}
	}
	return "—"
}

func (m Model) entityTableRow(ref model.TaskReference, header, selected, chosen bool) string {
	r := m.displayReference(ref)
	user := ""
	if !header {
		user = m.referenceUser(ref)
	}
	type column struct {
		heading, value string
		width          int
	}
	// Keep the user and title visible as the terminal narrows.
	width := max(1, m.width-4)
	userWidth := min(16, max(1, width/4))
	columns := []column{}
	if m.width >= 80 {
		columns = append(columns, column{"TYPE", r.Provider, 10}, column{"NØKKEL", r.Key, 24})
		width -= 36
	} else if m.width >= 50 {
		columns = append(columns, column{"NØKKEL", r.Key, 16})
		width -= 17
	}
	columns = append(columns, column{"BRUKER", user, userWidth}, column{"TITTEL", r.Title, max(1, width-userWidth-1)})
	mark := "  "
	if selected {
		mark = "> "
	}
	if chosen {
		mark += "✓ "
	} else {
		mark += "  "
	}
	values := make([]string, 0, len(columns))
	for _, c := range columns {
		value := c.value
		if header {
			value = c.heading
		}
		values = append(values, cell(value, c.width))
	}
	line := ansi.Truncate(mark+strings.Join(values, " "), max(1, m.width), "…")
	if header {
		return accent.Render(line)
	}
	if selected {
		return selection.Render(line)
	}
	return line
}

func (m Model) snoozeInputView() tea.View {
	state := *m.snoozes
	state.input = false
	m.snoozes = &state
	view := m.View()
	lines := strings.Split(view.Content, "\n")
	if state.whole {
		lines = []string{"Utsett hele sammenhengen · kjente relasjoner"}
		var members []model.TaskReference
		for _, e := range state.targets {
			members = append(members, e.Members...)
		}
		lines = append(lines, contextSummary(members))
		for _, r := range members {
			r = m.displayReference(r)
			lines = append(lines, fmt.Sprintf(" %s · %s · %s", r.Provider, r.Key, clean(r.Title)))
		}
	}
	lines = lines[:min(len(lines), max(0, m.height-1))]
	prompt := "Snooze: " + state.digits + "▏ · h timer · d dager · m/D kl. 06 · uten tall: 1 · Esc avbryt"
	if !state.whole && state.digits == "" {
		prompt = "Snooze: ▏ · z hele sammenhengen · h 1 time · d 1 dag · m/D kl. 06 · Esc avbryt"
	}
	if state.notice != "" {
		prompt += " · " + state.notice
	}
	lines = append(lines, ansi.Truncate(clean(prompt), max(1, m.width), "…"))
	view.Content = strings.Join(lines, "\n")
	return view
}

func (m Model) showSnapshot(r model.TaskReference) (tea.Model, tea.Cmd) {
	restore := m.referenceArchived(r) || (m.dialog != nil && (m.dialog.mode == "archive" || m.dialog.restore))
	m.dialog = &entityDialog{title: "Detaljer", mode: "snapshot", rows: []model.TaskReference{r}, chosen: map[string]model.TaskReference{}, restore: restore}
	return m, nil
}
func (m Model) entitySnapshotView() tea.View {
	d := m.dialog
	r := m.displayReference(d.rows[0])
	text := r.Provider + " · " + r.Key + "\n" + r.Title + "\n" + r.URL
	if entry, ok := m.snoozes.entries[refSnoozeID(r)]; ok {
		text += "\nUtsatt til: " + entry.Until.Local().Format("2006-01-02 15:04 MST")
		if len(entry.Roots) > 0 {
			text += "\nHele sammenhengen: " + contextSummary(entry.Members)
			for _, member := range entry.Members {
				member = m.displayReference(member)
				text += "\n" + member.Provider + " · " + member.Key + " · " + member.Title
			}
		}
	}
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, strings.Split(ansi.Hardwrap(clean(line), max(1, m.width), true), "\n")...)
	}
	capacity := max(1, m.height-1)
	start := min(d.offset, max(0, len(lines)-capacity))
	content := append([]string{}, lines[start:min(len(lines), start+capacity)]...)
	if m.height > 1 {
		content = append([]string{m.keyHints("j/k rull · Esc tilbake · a/e/r/t handling")}, content...)
	}
	for i := range content {
		content[i] = ansi.Truncate(content[i], max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(content, "\n"))
	view.AltScreen = true
	return view
}
