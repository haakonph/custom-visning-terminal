package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type messageSender interface {
	Send(context.Context, string) error
}
type mattermostState struct {
	sender          messageSender
	active, sending bool
	text, notice    string
	offset          int
}
type mattermostEdited struct {
	text string
	err  error
}
type mattermostSent struct{ err error }

func (m Model) WithMattermost(sender messageSender) Model {
	m.mattermost = &mattermostState{sender: sender}
	return m
}
func markdownLink(title, link string) string {
	title = strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "\n", " ", "\r", " ").Replace(title)
	link = strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(link)
	return "[" + title + "](" + link + ")"
}
func mattermostText(rows []model.PullRequest, issueURL func(string) string) string {
	var lines []string
	common := ""
	for i, p := range rows {
		lines = append(lines, markdownLink(p.Title, p.URL))
		keys := p.JiraKeys()
		if len(keys) != 1 {
			common = ""
			issueURL = nil
		} else if i == 0 {
			common = keys[0]
		} else if common != keys[0] {
			issueURL = nil
		}
	}
	if common != "" && issueURL != nil {
		lines = append(lines, markdownLink(common, issueURL(common)))
	}
	return strings.Join(lines, "\n")
}
func (m *Model) composeMattermost() tea.Cmd {
	if m.mattermost == nil {
		m.notice = "Sett MATTERMOST_WEBHOOK_URL for å sende til Mattermost"
		return nil
	}
	rows := []model.PullRequest{}
	for _, p := range m.chosen {
		rows = append(rows, p)
	}
	if len(rows) == 0 {
		visible := m.filtered()
		if len(visible) == 0 {
			return nil
		}
		rows = append(rows, visible[m.selected])
	}
	// Resolve selections against the newest available dashboard data.
	for i, p := range rows {
		for _, d := range m.snapshot.Datasets {
			for _, fresh := range d.PRs {
				if fresh.Key() == p.Key() && !fresh.UpdatedAt.Before(rows[i].UpdatedAt) {
					rows[i] = fresh
				}
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Repository == rows[j].Repository {
			return rows[i].Number < rows[j].Number
		}
		return rows[i].Repository < rows[j].Repository
	})
	var issueURL func(string) string
	if m.jira != nil {
		issueURL = m.jira.client.IssueURL
	}
	s := m.mattermost
	s.text = mattermostText(rows, issueURL)
	s.active = true
	s.offset = 0
	s.notice = ""
	return m.editMattermost()
}
func (m *Model) editMattermost() tea.Cmd {
	s := m.mattermost
	binary := ""
	for _, name := range []string{"nvim", "vim", "vi"} {
		if path, err := exec.LookPath(name); err == nil {
			binary = path
			break
		}
	}
	if binary == "" {
		s.notice = "Installer nvim, vim eller vi for å redigere Markdown-meldingen"
		return nil
	}
	dir, err := os.MkdirTemp("", "custom-visning-message-*")
	if err != nil {
		s.notice = "Kunne ikke opprette meldingsfil"
		return nil
	}
	path := filepath.Join(dir, "message.md")
	if err := os.WriteFile(path, []byte(s.text), 0600); err != nil {
		os.RemoveAll(dir)
		s.notice = "Kunne ikke skrive meldingsfil"
		return nil
	}
	command := exec.Command(binary, "-c", "set filetype=markdown", "-c", "set noswapfile nobackup nowritebackup noundofile", "--", path)
	return tea.ExecProcess(command, func(err error) tea.Msg {
		defer os.RemoveAll(dir)
		if err != nil {
			return mattermostEdited{err: fmt.Errorf("Vim ble avsluttet uten å fullføre redigeringen")}
		}
		b, err := os.ReadFile(path)
		return mattermostEdited{text: string(b), err: err}
	})
}
func (m *Model) mattermostMessage(msg tea.Msg) (bool, tea.Cmd) {
	s := m.mattermost
	if s == nil {
		return false, nil
	}
	switch msg := msg.(type) {
	case mattermostEdited:
		if msg.err != nil {
			s.notice = "Kunne ikke lese redigert melding; utkastet er beholdt"
		} else {
			s.text = msg.text
			s.notice = ""
			s.offset = 0
		}
		return true, nil
	case mattermostSent:
		s.sending = false
		if msg.err != nil {
			s.notice = msg.err.Error()
		} else {
			s.active = false
			m.notice = "Meldingen er sendt til Mattermost"
		}
		return true, nil
	case tea.KeyPressMsg:
		if !s.active {
			return false, nil
		}
		if msg.String() == "ctrl+c" {
			m.cancel()
			return true, tea.Quit
		}
		if s.sending {
			return true, nil
		}
		switch msg.String() {
		case "esc":
			s.active = false
		case "e":
			return true, m.editMattermost()
		case "j", "down":
			s.offset = min(s.offset+1, max(0, len(strings.Split(s.text, "\n"))-1))
		case "k", "up":
			s.offset = max(0, s.offset-1)
		case "ctrl+s":
			if strings.TrimSpace(s.text) == "" {
				s.notice = "Meldingen er tom"
				return true, nil
			}
			s.sending = true
			s.notice = "Sender…"
			sender, text, ctx := s.sender, s.text, m.ctx
			return true, func() tea.Msg { return mattermostSent{sender.Send(ctx, text)} }
		}
		return true, nil
	}
	return false, nil
}
func (m Model) mattermostView() tea.View {
	s := m.mattermost
	lines := []string{accent.Render(" MATTERMOST · Markdown"), " e rediger i Vim · :wq lagre i Vim · Ctrl+S send · Esc avbryt · j/k rull", ""}
	content := strings.Split(s.text, "\n")
	start := min(s.offset, max(0, len(content)-1))
	for _, line := range content[start:min(len(content), start+max(1, m.height-5))] {
		lines = append(lines, ansi.Truncate(clean(line), m.width, "…"))
	}
	lines = append(lines, warning.Render(clean(s.notice)))
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.WindowTitle = "custom-visning · Mattermost"
	return view
}
