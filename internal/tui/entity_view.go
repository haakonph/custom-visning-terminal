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
	lines := []string{d.title, "j/k velg · Space marker · Shift+↑/↓ utvid · / søk · Enter velg · Esc avbryt"}
	if d.searching {
		lines = append(lines, "/ "+clean(d.query)+"▏")
	} else if d.query != "" {
		lines = append(lines, "/ "+clean(d.query))
	}
	capacity := max(1, m.height-len(lines))
	start := max(0, d.selected-capacity+1)
	for i := start; i < min(len(rows), start+capacity); i++ {
		mark := "  "
		if i == d.selected {
			mark = "> "
		}
		if _, ok := d.chosen[refID(rows[i])]; ok {
			mark += "✓ "
		}
		lines = append(lines, fmt.Sprintf("%s%s · %s · %s", mark, rows[i].Provider, rows[i].Key, clean(rows[i].Title)))
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

func (m Model) snoozeInputView() tea.View {
	state := *m.snoozes
	state.input = false
	m.snoozes = &state
	view := m.View()
	lines := strings.Split(view.Content, "\n")
	lines = lines[:min(len(lines), max(0, m.height-1))]
	prompt := "Snooze: " + state.digits + "▏ · h timer · d dager · m dager kl. 06 · Esc avbryt"
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
	r := d.rows[0]
	text := r.Provider + " · " + r.Key + "\n" + r.Title + "\n" + r.URL
	if entry, ok := m.snoozes.entries[refSnoozeID(r)]; ok {
		text += "\nUtsatt til: " + entry.Until.Local().Format("2006-01-02 15:04 MST")
	}
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, strings.Split(ansi.Hardwrap(clean(line), max(1, m.width), true), "\n")...)
	}
	capacity := max(1, m.height-1)
	start := min(d.offset, max(0, len(lines)-capacity))
	content := append([]string{}, lines[start:min(len(lines), start+capacity)]...)
	if m.height > 1 {
		content = append([]string{"j/k rull · Esc tilbake · a/e/r/t handling"}, content...)
	}
	for i := range content {
		content[i] = ansi.Truncate(content[i], max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(content, "\n"))
	view.AltScreen = true
	return view
}
