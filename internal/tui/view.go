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
	if m.help {
		return m.helpView()
	}
	if m.snoozes != nil && m.snoozes.input {
		return m.snoozeInputView()
	}
	if m.sequence != "" || m.sequenceNotice != "" || m.jiraAction != nil {
		return m.keyboardView()
	}
	if m.settings.active {
		return m.settingsView()
	}
	if m.setup.provider != "" {
		return m.setupView()
	}
	if m.mattermost != nil && m.mattermost.active {
		return m.mattermostView()
	}
	if m.dialog != nil {
		return m.entityView()
	}
	if m.snoozes != nil && m.snoozes.active {
		return m.snoozeView()
	}
	if m.detail != nil {
		return m.detailView()
	}
	if m.visibleProvider() == "jira" && m.jira.detail != nil {
		return m.jiraDetailView()
	}
	if m.mmReader != nil && m.mmReader.active {
		return m.mmReaderView()
	}
	if m.tasks.active {
		return m.tasksView()
	}
	if m.copilotActive {
		return m.copilotView()
	}
	if m.jiraActive {
		return m.jiraView()
	}
	lines := []string{}
	user := m.snapshot.User
	if user == "" {
		user = "authenticating…"
	}
	lines = append(lines, accent.Render(" CUSTOM VISNING ")+muted.Render("GitHub · "+clean(user)+"  [vg GitHub] · vt Jira"))
	tabs := []string{}
	for v := model.Open; v < model.ViewCount; v++ {
		label := v.String()
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
			label := v.String()
			if v == m.active {
				label = "[" + label + "] " + v.String()
			}
			tabs = append(tabs, label)
		}
	}
	lines = append(lines, strings.Join(tabs, "  "))
	lines = append(lines, m.snoozeLines()...)
	if m.relatedKey != "" {
		lines = append(lines, " Relaterte PR-er: "+m.relatedKey+" · lastede GitHub-visninger · Esc tilbake")
	}
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
	if m.local != nil && m.local.running > 0 {
		lines = append(lines, fmt.Sprintf(" Worktrees: %d preparing/queued · %d completed", m.local.running+len(m.local.queue), len(m.local.results)))
	}
	if m.filtering || m.input.Value() != "" {
		lines = append(lines, m.input.View())
	}
	if m.local != nil && m.local.choosing {
		lines = append(lines, fmt.Sprintf(" Editor: %s · j/k velg · Enter åpne · Esc avbryt", []string{"IntelliJ IDEA", "VS Code", "egendefinert"}[m.local.editorChoice]))
	}
	if m.help {
		lines = append(lines, strings.Split(helpText, "\n")...)
	} else {
		rows := m.filtered()
		lines = append(lines, m.row(model.PullRequest{}, true, false))
		// Reserve space for details, both warning sources, results and footer.
		reserved := 7
		if m.local != nil {
			reserved += min(3, len(m.local.results))
		}
		capacity := max(1, m.height-len(lines)-reserved)
		start := max(0, m.selected-capacity+1)
		for i := start; i < min(len(rows), start+capacity); i++ {
			lines = append(lines, m.row(rows[i], false, i == m.selected))
		}
		if len(rows) == 0 {
			lines = append(lines, muted.Render(" No pull requests in this view."))
		}
		if len(rows) > 0 {
			p := rows[m.selected]
			lines = append(lines, muted.Render(clean(" "+reviewSummaryLabel(p))))
			lines = append(lines, " Local: "+m.localLabel(p)+" · Jira: "+strings.Join(p.JiraKeys(m.jiraPattern), ", ")+" · l relaterte saker")
			detail := fmt.Sprintf(" %d/%d · merge: %s · automerge: %s · queue: %s · reviewers: %s · teams: %s · labels: %s", m.selected+1, len(rows), p.Mergeability, autoMergeLabel(p), queueLabel(p), strings.Join(p.RequestedReviewers, ", "), m.githubTeams(p.RequestedTeams), strings.Join(p.Labels, ", "))
			lines = append(lines, muted.Render(clean(detail)))
			if m.mmReader != nil {
				lines = append(lines, fmt.Sprintf(" Mattermost mentions (%d) · v vis referanser · M kanaler", m.mmMentionCount(p.URL)))
			}
		}
	}
	if m.local != nil && len(m.local.results) > 0 {
		for _, result := range m.local.results[max(0, len(m.local.results)-3):] {
			lines = append(lines, warning.Render(clean(result)))
		}
	}
	for _, message := range []string{m.warnings[m.active], m.notice} {
		if message != "" {
			lines = append(lines, warning.Render(clean(message)))
		}
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
	title := p.DisplayTitle(m.jiraPattern)
	if _, ok := m.chosen[p.Key()]; ok {
		title = "[✓] " + title
	}
	if p.Draft {
		title = "[draft] " + title
	}
	jira := strings.Join(p.JiraKeys(m.jiraPattern), ", ")
	repository := m.githubName(p.Repository)
	if m.width >= 100 {
		columns = []column{{"REPOSITORY", repository, 18, false}, {"PR", number, 6, false}, {"JIRA", jira, 14, false}, {"TITLE", title, 1, false}, {"REVIEW", string(p.Review), 9, true}, {"CI", string(p.CI), 7, true}, {"AUTO", autoMergeLabel(p), 7, false}, {"QUEUE", queueLabel(p), 14, false}}
		if m.width >= 140 {
			columns = append(columns, column{"AUTHOR", p.Author, 12, false}, column{"UPDATED", age(p.UpdatedAt), 7, false}, column{"LOCAL", m.localLabel(p), 18, false})
		}
	} else if m.width >= 60 {
		columns = []column{{"REPOSITORY", repository, 14, false}, {"PR", number, 6, false}, {"JIRA", jira, 12, false}, {"TITLE", title, 1, false}, {"REVIEW", string(p.Review), 9, true}}
	} else {
		columns = []column{{"PR", number, 6, false}, {"JIRA", jira, min(12, max(0, m.width/4)), false}, {"TITLE", title, 1, false}}
	}
	// Give the title all remaining space, including column separators.
	used, titleIndex := len(columns)-1, 0
	for i, c := range columns {
		if c.name == "TITLE" {
			titleIndex = i
		} else {
			used += c.width
		}
	}
	columns[titleIndex].width = max(1, m.width-used)
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

func (m Model) helpView() tea.View {
	rows := strings.Split(helpText, "\n")
	version := m.version
	if version == "" {
		version = "dev"
	}
	rows[0] += " · custom-visning · Versjon " + clean(version)
	capacity := max(1, m.height-1)
	start := max(0, min(m.helpOffset, len(rows)-capacity))
	lines := append([]string{}, rows[start:min(len(rows), start+capacity)]...)
	if m.height > 1 {
		lines = append(lines, "j/k ↑/↓ scroll · ? / Esc close")
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "…")
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.WindowTitle = "custom-visning · Help"
	return v
}
