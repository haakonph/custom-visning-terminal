package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

var (
	accent    = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	muted     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selection = lipgloss.NewStyle().Background(lipgloss.Color("24")).Foreground(lipgloss.Color("255"))
	warning   = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
)

func clean(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}
func cell(value string, width int) string {
	value = ansi.Truncate(clean(value), max(0, width), "…")
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}
func status(s model.Status) string {
	color := "245"
	switch s {
	case model.Approved, model.Pass:
		color = "108"
	case model.Fail, model.ChangesRequested:
		color = "174"
	case model.Pending:
		color = "179"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(string(s))
}
func (m Model) View() tea.View {
	lines := []string{}
	user := m.snapshot.User
	if user == "" {
		user = "authenticating…"
	}
	lines = append(lines, accent.Render(" CUSTOM VISNING ")+muted.Render("GitHub · "+clean(user)))
	tabs := []string{}
	for v := model.Open; v < model.ViewCount; v++ {
		label := fmt.Sprintf("%d %s", v+1, v.String())
		if m.loading[v] {
			label += " ↻"
		}
		if v == m.active {
			label = accent.Render("[" + label + "]")
		} else {
			label = muted.Render(label)
		}
		tabs = append(tabs, label)
	}
	if m.width < 95 {
		tabs = nil
		for v := model.Open; v < model.ViewCount; v++ {
			label := fmt.Sprintf("%d", v+1)
			if v == m.active {
				label = "[" + label + "] " + v.String()
			}
			tabs = append(tabs, label)
		}
	}
	lines = append(lines, strings.Join(tabs, "  "))
	sync := "never"
	if at := m.snapshot.Datasets[m.active].SyncedAt; !at.IsZero() {
		sync = at.Local().Format("2006-01-02 15:04:05")
	}
	state := "ready"
	if m.loading[m.active] {
		state = "refreshing…"
	}
	if m.booting {
		state = "authenticating…"
	}
	lines = append(lines, muted.Render(fmt.Sprintf(" %s · last full sync: %s", state, sync)))
	if m.filtering || m.input.Value() != "" {
		lines = append(lines, m.input.View())
	}
	if m.help {
		lines = append(lines, strings.Split(helpText, "\n")...)
	} else {
		rows := m.filtered()
		lines = append(lines, m.row(model.PullRequest{}, true, false))
		// Reserve room for a selected-PR detail line, warning and footer.
		capacity := max(1, m.height-len(lines)-4)
		start := max(0, m.selected-capacity+1)
		for i := start; i < min(len(rows), start+capacity); i++ {
			lines = append(lines, m.row(rows[i], false, i == m.selected))
		}
		if len(rows) == 0 {
			lines = append(lines, muted.Render(" No pull requests in this view."))
		}
		if len(rows) > 0 {
			p := rows[m.selected]
			detail := fmt.Sprintf(" %d/%d · merge: %s · reviewers: %s · teams: %s · labels: %s", m.selected+1, len(rows), p.Mergeability, strings.Join(p.RequestedReviewers, ", "), strings.Join(p.RequestedTeams, ", "), strings.Join(p.Labels, ", "))
			lines = append(lines, muted.Render(clean(detail)))
		}
	}
	message := m.warnings[m.active]
	if message == "" {
		message = m.notice
	}
	if message != "" {
		lines = append(lines, warning.Render(clean(message)))
	}
	lines = append(lines, muted.Render(shortcuts))
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "…")
	}
	// Keep the footer visible even in very short windows.
	if len(lines) > m.height {
		lines = append(lines[:max(0, m.height-1)], lines[len(lines)-1])
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.WindowTitle = "custom-visning · GitHub PRs"
	return view
}
func (m Model) row(p model.PullRequest, header, selected bool) string {
	type column struct {
		name, value string
		width       int
		isStatus    bool
	}
	columns := []column{}
	number := fmt.Sprintf("#%d", p.Number)
	title := p.Title
	if p.Draft {
		title = "[draft] " + title
	}
	if m.width >= 100 {
		columns = []column{{"REPOSITORY", p.Repository, 20, false}, {"PR", number, 7, false}, {"TITLE", title, m.width - 74, false}, {"AUTHOR", p.Author, 12, false}, {"REVIEW", string(p.Review), 9, true}, {"CI", string(p.CI), 7, true}, {"UPDATED", age(p.UpdatedAt), 7, false}}
	} else if m.width >= 60 {
		columns = []column{{"REPOSITORY", p.Repository, 16, false}, {"PR", number, 7, false}, {"TITLE", title, m.width - 34, false}, {"REVIEW", string(p.Review), 8, true}}
	} else {
		columns = []column{{"PR", number, 7, false}, {"TITLE", title, max(1, m.width-8), false}}
	}
	values := []string{}
	for _, c := range columns {
		value := c.value
		if header {
			value = c.name
		}
		value = cell(value, c.width)
		if c.isStatus && !header && !selected {
			value = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(value)
			if model.Status(c.value) != model.Unknown {
				value = strings.Replace(value, c.value, status(model.Status(c.value)), 1)
			}
		}
		values = append(values, value)
	}
	line := strings.Join(values, " ")
	if header {
		return accent.Render(line)
	}
	if selected {
		return selection.Render(line)
	}
	return line
}
func age(at time.Time) string {
	if at.IsZero() {
		return "?"
	}
	elapsed := max(0, time.Since(at))
	if elapsed < time.Minute {
		return "now"
	}
	if elapsed < time.Hour {
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	}
	if elapsed < 24*time.Hour {
		return fmt.Sprintf("%dh", int(elapsed.Hours()))
	}
	return fmt.Sprintf("%dd", int(elapsed.Hours()/24))
}
