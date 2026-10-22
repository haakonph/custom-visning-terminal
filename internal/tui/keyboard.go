package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) keyboardView() tea.View {
	sequence, notice, action := m.sequence, m.sequenceNotice, m.jiraAction
	m.sequence, m.sequenceNotice, m.jiraAction = "", "", nil
	view := m.View()
	lines := []string{}
	if sequence != "" {
		lines = append(lines, "Tastsekvens: "+sequence+" · "+sequenceHint(sequence)+" · Esc avbryt")
	}
	if notice != "" {
		lines = append(lines, notice)
	}
	if action != nil {
		if action.running {
			lines = append(lines, "Jira: utfører handling…")
		} else if action.loading {
			lines = append(lines, "Jira: henter tilgjengelige overganger… · q QA · u under arbeid · a avklaring · f ferdig · Esc avbryt")
		} else {
			lines = append(lines, "Jira: velg overgang · j/k · Enter · q QA · u under arbeid · a avklaring · f ferdig · Esc avbryt")
			start := max(0, action.selected-max(1, m.height-2)+1)
			for i := start; i < min(len(action.choices), start+max(1, m.height-2)); i++ {
				choice := action.choices[i]
				mark := "  "
				if i == action.selected {
					mark = "> "
				}
				lines = append(lines, fmt.Sprintf("%s%d %s", mark, i+1, clean(choice)))
			}
		}
	}
	if len(lines) > m.height {
		lines = lines[:max(1, m.height)]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(clean(lines[i]), max(1, m.width), "…")
	}
	// Keep the prompt visible even in a small terminal.
	content := strings.Split(view.Content, "\n")
	keep := max(0, m.height-len(lines))
	if len(content) > keep {
		content = content[:keep]
	}
	view.Content = strings.Join(append(content, lines...), "\n")
	return view
}
func (m Model) jiraDetailLines() []string {
	issue := m.jira.detail
	text := fmt.Sprintf("%s · %s\n%s · %s · %s\n\n%s", issue.Key, issue.Title, issue.Status, issue.Priority, issue.Assignee, m.jiraPRSummary(issue.Key).detail()+"\n\n"+issue.Description)
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, strings.Split(lipgloss.NewStyle().Width(max(1, m.width)).Render(clean(line)), "\n")...)
	}
	return lines
}
func (m Model) jiraDetailView() tea.View {
	lines := m.jiraDetailLines()
	start := min(m.jira.detailOffset, max(0, len(lines)-1))
	end := min(len(lines), start+max(1, m.height-1))
	content := append([]string{}, lines[start:end]...)
	if m.height > 1 {
		content = append([]string{"Esc tilbake · j/k ↑/↓ bla"}, content...)
	}
	for i := range content {
		content[i] = ansi.Truncate(content[i], max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(content, "\n"))
	view.AltScreen = true
	return view
}
