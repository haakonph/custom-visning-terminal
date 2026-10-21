package tui

import (
	"context"
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
	messages        []string
}
type mattermostSent struct {
	err  error
	sent int
}

func (m Model) WithMattermost(sender messageSender) Model {
	m.mattermost = &mattermostState{sender: sender}
	return m
}
func markdownLink(title, link string) string {
	title = strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "\n", " ", "\r", " ").Replace(title)
	link = strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(link)
	return "[" + title + "](" + link + ")"
}
func (m Model) mattermostPRText(p model.PullRequest) string {
	return markdownLink("["+m.githubName(p.Repository)+"] "+p.Title, p.URL)
}

func (m *Model) sendMattermost(messages []string) tea.Cmd {
	s := m.mattermost
	if s.sending {
		return nil
	}
	s.messages = append([]string(nil), messages...)
	s.text = strings.Join(messages, "\n")
	s.active = true
	s.sending = true
	s.offset = 0
	s.notice = "Sender…"
	sender, ctx := s.sender, m.ctx
	return func() tea.Msg {
		result := mattermostSent{}
		for _, text := range messages {
			if err := sender.Send(ctx, text); err != nil {
				result.err = err
				break
			}
			result.sent++
		}
		return result
	}
}
func (m *Model) composeMattermost() tea.Cmd {
	if m.mattermost == nil {
		m.notice = "Sett MATTERMOST_WEBHOOK_URL for å sende til Mattermost"
		return nil
	}
	rows := m.selectedPRs()
	if len(rows) == 0 {
		return nil
	}
	messages := make([]string, len(rows))
	for i, p := range rows {
		messages[i] = m.mattermostPRText(p)
	}
	return m.sendMattermost(messages)
}
func (m *Model) mattermostMessage(msg tea.Msg) (bool, tea.Cmd) {
	s := m.mattermost
	if s == nil {
		return false, nil
	}
	switch msg := msg.(type) {
	case mattermostSent:
		s.sending = false
		if msg.err != nil {
			s.messages = s.messages[min(msg.sent, len(s.messages)):]
			s.text = strings.Join(s.messages, "\n")
			s.notice = msg.err.Error()
		} else {
			s.active = false
			s.messages = nil
			m.notice = "Meldingene er sendt til Mattermost"
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
		case "j", "down":
			s.offset = min(s.offset+1, max(0, len(strings.Split(s.text, "\n"))-1))
		case "k", "up":
			s.offset = max(0, s.offset-1)
		case "ctrl+s":
			if strings.TrimSpace(s.text) == "" {
				s.notice = "Meldingen er tom"
				return true, nil
			}
			messages := s.messages
			if len(messages) == 0 {
				messages = []string{s.text}
			}
			return true, m.sendMattermost(messages)
		}
		return true, nil
	}
	return false, nil
}
func (m Model) mattermostView() tea.View {
	s := m.mattermost
	lines := []string{accent.Render(" MATTERMOST · Markdown"), " as prøv usendte meldinger igjen · Esc lukk · j/k rull", ""}
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
