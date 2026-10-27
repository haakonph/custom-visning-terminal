package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	mm "github.com/haakonph/custom-visning-terminal/internal/conversations"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
	"github.com/haakonph/custom-visning-terminal/internal/syncstate"
)

type messageSender = mm.Sender
type mattermostState struct {
	sender          messageSender
	active, sending bool
	text, notice    string
	offset          int
	messages        []string
	sent            map[string]time.Time
}
type mattermostSent struct {
	token      syncstate.Token
	err        error
	sent       int
	timestamps map[string]time.Time
	saveErr    error
}

func (m Model) WithMattermost(sender messageSender) Model {
	m.webhookSession.Configure(m.ctx)
	m.mattermost = &mattermostState{sender: sender, sent: map[string]time.Time{}}
	if m.entities != nil {
		sent, err := storage.LoadReviewMessages(m.entities.path)
		if err != nil {
			m.notice = "Kunne ikke lese sendetider for Mattermost: " + err.Error()
		} else {
			m.mattermost.sent = sent
		}
	}
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
	sender, ctx := s.sender, m.webhookSession.Context(m.ctx)
	token := m.webhookSession.Begin("share")
	messages = append([]string(nil), messages...)
	path := ""
	if m.entities != nil {
		path = m.entities.path
	}
	return func() tea.Msg {
		result := mm.Share(ctx, sender, messages, path)
		return mattermostSent{token: token, err: result.Err, sent: result.Sent, timestamps: result.Timestamps, saveErr: result.SaveErr}
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
		if msg.token.Sequence != 0 && !m.webhookSession.Match("share", msg.token) {
			return true, nil
		}
		s.sending = false
		for link, at := range msg.timestamps {
			s.sent[link] = at
		}
		if msg.err != nil {
			s.messages = s.messages[min(msg.sent, len(s.messages)):]
			s.text = strings.Join(s.messages, "\n")
			s.notice = msg.err.Error()
		} else {
			s.active = false
			s.messages = nil
			m.notice = "Meldingene er sendt til Mattermost"
		}
		if msg.saveErr != nil {
			notice := "; kunne ikke lagre sendetid: " + msg.saveErr.Error()
			if msg.err != nil {
				s.notice += notice
			} else {
				m.notice += notice
			}
		}
		return true, nil
	case tea.KeyPressMsg:
		if !s.active {
			return false, nil
		}
		if msg.String() == "ctrl+c" {
			return true, m.quit()
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
			return m.sendMattermostDraft()
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

func (m *Model) sendMattermostDraft() (bool, tea.Cmd) {
	s := m.mattermost
	if s == nil || s.sending {
		return true, nil
	}
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
