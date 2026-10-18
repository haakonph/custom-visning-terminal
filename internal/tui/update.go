package tui

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.settings.active {
		switch msg.(type) {
		case tea.WindowSizeMsg, bootMsg, refreshMsg, cacheMsg, jiraResult:
		default:
			return m.updateSettings(msg)
		}
	}
	if m.help && !m.tasks.active && !m.jiraActive && !m.copilotActive {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc", "?":
				m.help = false
			case "j", "down":
				m.helpOffset = min(m.helpOffset+1, max(0, len(strings.Split(helpText, "\n"))-max(1, m.height-1)))
			case "k", "up":
				m.helpOffset = max(0, m.helpOffset-1)
			case "q", "ctrl+c":
				m.cancel()
				return m, tea.Quit
			}
			return m, nil
		}
	}
	if handled, cmd := m.snoozeMessage(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.localMessage(msg); handled {
		return m, cmd
	}
	if _, ok := msg.(copilotOpenMsg); ok {
		return m.updateCopilot(msg)
	}
	if _, ok := msg.(copilotResult); ok {
		return m.updateCopilot(msg)
	}
	if _, ok := msg.(copilotTick); ok {
		return m.updateCopilot(msg)
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "," && !m.tasks.active && !m.filtering && (m.jira == nil || m.jira.mode == "") && (m.copilot == nil || !m.copilot.filtering) {
		return m.openSettings()
	}
	if m.copilotActive {
		switch msg.(type) {
		case tea.WindowSizeMsg, bootMsg, refreshMsg, cacheMsg, jiraResult:
		default:
			return m.updateCopilot(msg)
		}
	}

	if key, ok := msg.(tea.KeyPressMsg); ok && m.local != nil && m.local.choosing {
		switch key.String() {
		case "1":
			m.local.choosing = false
			return m, m.openLocal("intellij")
		case "2":
			m.local.choosing = false
			return m, m.openLocal("vscode")
		case "3":
			m.local.choosing = false
			return m, m.openLocal("custom")
		case "esc":
			m.local.choosing = false
			return m, nil
		case "ctrl+c", "q":
			m.cancel()
			return m, tea.Quit
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.tasks.active {
			return m.updateTasks(key)
		}
		editing := m.filtering
		if m.jiraActive {
			editing = m.jira.mode != ""
		}
		if !editing {
			switch key.String() {
			case "P", ",":
				return m.openSettings()
			case "T":
				m.tasks.active = true
				m.tasks.selectVisible()
				return m, nil
			case "t", "a":
				return m.collectTask(key.String() == "t")
			case "c":
				return m.switchCopilot()
			case "l":
				return m.navigateRelated()
			case "g":
				return m.switchProvider(false)
			case "J":
				return m.switchProvider(true)
			}
		}
	}
	if m.tasks.active && (m.tasks.editing || m.tasks.searching) {
		switch msg.(type) {
		case tea.WindowSizeMsg, bootMsg, refreshMsg, cacheMsg, jiraResult, openMsg:
		default:
			var cmd tea.Cmd
			m.tasks.input, cmd = m.tasks.input.Update(msg)
			if m.tasks.searching {
				m.tasks.query = m.tasks.input.Value()
				m.tasks.selectVisible()
			}
			return m, cmd
		}
	}
	if result, ok := msg.(openMsg); ok && m.tasks.active {
		if result.err != nil {
			m.tasks.notice = "Kunne ikke åpne referansen: " + result.err.Error()
		}
		return m, nil
	}
	if _, ok := msg.(jiraResult); ok {
		return m.updateJira(msg)
	}
	if m.jiraActive {
		switch msg.(type) {
		case tea.KeyPressMsg:
			return m.updateJira(msg)
		case tea.WindowSizeMsg, bootMsg, refreshMsg, cacheMsg:
		default:
			return m.updateJira(msg)
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(1, msg.Width)
		m.height = max(1, msg.Height)
		m.input.SetWidth(max(1, m.width-4))
		m.tasks.input.SetWidth(max(1, m.width-10))
		if m.copilot != nil {
			m.copilot.input.SetWidth(max(1, m.width-4))
		}
		if m.jira != nil {
			m.jira.input.SetWidth(max(1, m.width-4))
		}
		m.clamp()
	case bootMsg:
		m.booting = false
		if msg.err != nil {
			m.notice = msg.err.Error()
			return m, nil
		}
		m.snapshot = msg.snapshot
		m.notice = msg.warning
		cmds := []tea.Cmd{}
		for view := model.Open; view < model.ViewCount; view++ {
			cmds = append(cmds, m.refresh(view))
		}
		cmds = append(cmds, m.refreshLocal())
		return m, tea.Batch(cmds...)
	case refreshMsg:
		m.loading[msg.view] = false
		warnings := append([]string{}, msg.result.Warnings...)
		if msg.err != nil {
			warnings = append(warnings, msg.err.Error())
		}
		m.warnings[msg.view] = strings.Join(warnings, "; ")
		selectedURL := ""
		if rows := m.filtered(); m.selected < len(rows) {
			selectedURL = rows[m.selected].URL
		}
		data := &m.snapshot.Datasets[msg.view]
		if msg.err == nil && msg.result.Complete {
			data.PRs = msg.result.PRs
			model.Sort(data.PRs)
			data.SyncedAt = msg.at
		} else {
			data.PRs = model.Merge(data.PRs, msg.result.PRs)
			if m.warnings[msg.view] == "" {
				m.warnings[msg.view] = "Partial results; cached rows retained"
			}
		}
		if msg.view == m.active || m.relatedKey != "" {
			for i, p := range m.filtered() {
				if p.URL == selectedURL {
					m.selected = i
					break
				}
			}
			m.clamp()
		}
		return m, tea.Batch(m.persist(), m.refreshLocal())
	case cacheMsg:
		m.saving = false
		if msg.err != nil {
			m.notice = "Cache write failed; data is available in memory"
		}
		if m.dirty {
			return m, m.persist()
		}
	case openMsg:
		if msg.err != nil {
			m.notice = "Could not open browser; URL: " + msg.err.Error()
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			m.cancel()
			return m, tea.Quit
		}
		if m.filtering {
			if key == "esc" {
				m.filtering = false
				m.input.Blur()
				m.input.SetValue("")
				m.selected = 0
				return m, nil
			}
			if key == "enter" {
				m.filtering = false
				m.input.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.selected = 0
			return m, cmd
		}
		switch key {

		case "space", " ":
			if m.local != nil {
				rows := m.filtered()
				if len(rows) > 0 {
					p := rows[m.selected]
					if _, ok := m.local.chosen[p.Key()]; ok {
						delete(m.local.chosen, p.Key())
					} else {
						m.local.chosen[p.Key()] = p
					}
				}
			}
		case "o":
			return m, m.openLocal("")
		case "O":
			if m.local != nil {
				m.local.choosing = true
			}
		case "q":
			m.cancel()
			return m, tea.Quit
		case "1", "2", "3", "4":
			m.active = model.View(key[0] - '1')
			m.selected = 0
		case "j", "down":
			m.selected++
			m.clamp()
		case "k", "up":
			m.selected--
			m.clamp()
		case "r":
			if m.snapshot.User == "" && !m.booting {
				m.booting = true
				m.notice = ""
				return m, m.bootstrap()
			}
			return m, m.refresh(m.active)
		case "/":
			m.filtering = true
			return m, m.input.Focus()
		case "esc":
			m.input.SetValue("")
			m.relatedKey = ""
			m.help = false
			m.selected = 0
		case "?":
			m.help = !m.help
			m.helpOffset = 0
		case "enter":
			rows := m.filtered()
			if len(rows) == 0 {
				return m, nil
			}
			link, opener, ctx := rows[m.selected].URL, m.open, m.ctx
			return m, func() tea.Msg { return openMsg{err: opener(ctx, link)} }
		}
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}
func (m Model) filtered() []model.PullRequest {
	rows := m.snapshot.Datasets[m.active].PRs
	if m.relatedKey != "" {
		rows = m.relatedPRs(m.relatedKey)
	}
	visible := make([]model.PullRequest, 0, len(rows))
	for _, p := range rows {
		if !m.snoozes.hidden(snooze.Identity("github", p.URL)) {
			visible = append(visible, p)
		}
	}
	rows = visible
	filter := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if filter == "" {
		return rows
	}
	result := []model.PullRequest{}
	for _, p := range rows {
		text := fmt.Sprintf("%s %d %s %s %s %s %s %s", p.Repository, p.Number, p.Title, p.Author, strings.Join(p.Labels, " "), strings.Join(p.RequestedTeams, " "), p.Review, p.CI)
		if strings.Contains(strings.ToLower(text), filter) {
			result = append(result, p)
		}
	}
	return result
}
func (m *Model) clamp() { m.selected = max(0, min(m.selected, len(m.filtered())-1)) }
func openBrowser(ctx context.Context, link string) error {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil {
		return fmt.Errorf("invalid GitHub PR URL")
	}
	return launchBrowser(ctx, link)
}
func openJiraBrowser(ctx context.Context, client *jira.Client, link string) error {
	if !client.ValidIssueURL(link) {
		return fmt.Errorf("invalid Jira issue URL")
	}
	return launchBrowser(ctx, link)
}
func launchBrowser(ctx context.Context, link string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
		args = []string{link}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", link}
	default:
		command = "xdg-open"
		args = []string{link}
	}
	if err := exec.CommandContext(ctx, command, args...).Run(); err != nil {
		return fmt.Errorf("%s", link)
	}
	return nil
}
