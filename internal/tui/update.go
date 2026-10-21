package tui

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
)

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(notificationResult); ok {
		if result.failed {
			m.notice = "Kunne ikke vise eller åpne skrivebordsvarsel. Kontroller varslingstillatelser og varslingsverktøy."
			if runtime.GOOS == "darwin" {
				m.notice += " Installer terminal-notifier med: brew install terminal-notifier"
			}
		}
		return m, nil
	}
	if next, cmd, handled := m.autoMessage(msg); handled {
		return next, cmd
	}
	if r, ok := msg.(setupValidated); ok {
		return m.finishSetupValidation(r)
	}
	if m.setup.wizard != nil {
		if key, ok := msg.(tea.KeyPressMsg); !ok || key.String() != "ctrl+c" {
			if _, resize := msg.(tea.WindowSizeMsg); !resize {
				switch msg.(type) {
				case tea.KeyPressMsg, tea.PasteMsg:
					return m.updateSetupWizard(msg)
				}
			}
		}
	}
	if _, ok := msg.(tea.KeyPressMsg); ok && m.sequence == "" && m.jiraAction == nil {
		m.sequenceNotice = ""
	}
	if next, cmd, handled := m.updateKeyboard(msg); !m.internalKey && handled {
		return next, cmd
	}
	m.internalKey = false
	if handled, cmd := m.localMessage(msg); handled {
		return m, cmd
	}
	if m.snoozes != nil && m.snoozes.input {
		if handled, cmd := m.snoozeMessage(msg); handled {
			return m, cmd
		}
	}
	if m.settings.active {
		switch msg.(type) {
		case tea.WindowSizeMsg, bootMsg, refreshMsg, cacheMsg, jiraResult, detailMsg, copilotTick, copilotResult, copilotOpenMsg, mmTick, mmResult:
		default:
			return m.updateSettings(msg)
		}
	}
	if m.mattermost != nil && m.mattermost.active {
		if handled, cmd := m.mattermostMessage(msg); handled {
			return m, cmd
		}
	}
	if result, ok := msg.(detailMsg); ok {
		if m.detail != nil && m.detail.request == result.request {
			m.detail.loading = false
			if result.err == nil {
				m.detail.data = retainDetailSections(m.detail.data, result.data)
				if len(result.data.Warnings) == 0 {
					m.detail.synced = time.Now()
				}
			}
			m.detail.err = result.err
		}
		return m, nil
	}
	if m.detail != nil {
		if result, ok := msg.(openMsg); ok {
			m.detail.err = result.err
			return m, nil
		}
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				m.detail = nil
			case "j", "down":
				m.detail.offset = min(m.detail.offset+1, max(0, len(m.detailLines())-max(1, m.height)))
			case "k", "up":
				m.detail.offset = max(0, m.detail.offset-1)

			}
			return m, nil
		}
	}

	if handled, cmd := m.mmMessage(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.mattermostMessage(msg); handled {
		return m, cmd
	}
	if m.settings.active {
		switch msg.(type) {
		case tea.WindowSizeMsg, bootMsg, refreshMsg, cacheMsg, jiraResult, detailMsg, copilotTick, copilotResult, copilotOpenMsg, mmTick, mmResult:
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
	if _, ok := msg.(copilotOpenMsg); ok {
		return m.updateCopilot(msg)
	}
	if _, ok := msg.(copilotResult); ok {
		return m.updateCopilot(msg)
	}
	if _, ok := msg.(copilotTick); ok {
		return m.updateCopilot(msg)
	}
	if m.copilotActive {
		switch msg.(type) {
		case tea.WindowSizeMsg, bootMsg, refreshMsg, cacheMsg, jiraResult, detailMsg, copilotTick, copilotResult, copilotOpenMsg, mmTick, mmResult:
		default:
			return m.updateCopilot(msg)
		}
	}

	if key, ok := msg.(tea.KeyPressMsg); ok && m.local != nil && m.local.choosing {
		switch key.String() {
		case "j", "down":
			m.local.editorChoice = min(2, m.local.editorChoice+1)
		case "k", "up":
			m.local.editorChoice = max(0, m.local.editorChoice-1)
		case "enter":
			m.local.choosing = false
			return m, m.openLocal([]string{"intellij", "vscode", "custom"}[m.local.editorChoice])
		case "esc":
			m.local.choosing = false
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && m.tasks.active {
		return m.updateTasks(key)
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
		if msg.autoMergeVersion != m.autoMergeVersion {
			return m, m.refresh(msg.view)
		}
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
			if m.detail != nil {
				for _, pr := range data.PRs {
					if pr.Key() == m.detail.pr.Key() {
						m.detail.pr = pr
						break
					}
				}
			}
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
		return m, tea.Batch(m.persist(), m.refreshLocal(), m.notifyPullRequests(msg))
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
			rows := m.filtered()
			if len(rows) > 0 {
				p := rows[m.selected]
				if _, ok := m.chosen[p.Key()]; ok {
					delete(m.chosen, p.Key())
				} else {
					m.chosen[p.Key()] = p
				}
			}
		case "d":
			rows := m.filtered()
			if len(rows) > 0 {
				m.detail = &detailState{pr: rows[m.selected]}
				return m, m.loadDetails()
			}
		case "j", "down":
			m.selected++
			m.clamp()
		case "k", "up":
			m.selected--
			m.clamp()
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
		if m.active == model.TeamReviews && m.relatedKey == "" && strings.EqualFold(p.Author, m.snapshot.User) {
			continue
		}
		if !m.snoozes.hidden(snooze.Identity("github", p.URL)) && !m.entities.archived("github", p.URL) {
			visible = append(visible, p)
		}
	}
	rows = visible
	filter := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if filter == "" {
		return m.groupPRs(rows)
	}
	result := []model.PullRequest{}
	for _, p := range rows {
		text := fmt.Sprintf("%s %d %s %s %s %s %s %s", p.Repository, p.Number, p.Title, p.Author, strings.Join(p.Labels, " "), strings.Join(p.RequestedTeams, " "), p.Review, p.CI)
		if strings.Contains(strings.ToLower(text), filter) {
			result = append(result, p)
		}
	}
	return m.groupPRs(result)
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
