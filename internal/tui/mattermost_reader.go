package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	mm "github.com/haakonph/custom-visning-terminal/internal/connectors/mattermost"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/snooze"
)

type mattermostReader struct {
	entities         *entityState
	snoozes          *snoozeState
	query            string
	searching        bool
	channelSearching bool

	open                                                          func(context.Context, string) error
	service                                                       *mm.Service
	api                                                           *mm.API
	keychain                                                      mm.TokenStore
	account, apiToken                                             string
	data                                                          model.MattermostData
	active, busy, authenticated, choosing, mentions, confirmClear bool
	tab, row, channelRow, link, scroll                            int
	root, mentionURL, notice, mode, password, jumpID              string
	input                                                         textinput.Model
}
type mmResult struct {
	data          model.MattermostData
	err           error
	authenticated bool
	notice        string
}
type mmTick struct{}

func mmTimer() tea.Cmd { return tea.Tick(30*time.Second, func(time.Time) tea.Msg { return mmTick{} }) }

func (m Model) WithMattermostReader(service *mm.Service, api *mm.API, keychain mm.TokenStore, account, token string) Model {
	input := textinput.New()
	input.CharLimit = 512
	m.mmReader = &mattermostReader{entities: m.entities, snoozes: m.snoozes, open: openMattermostBrowser, service: service, api: api, keychain: keychain, account: account, apiToken: token, data: service.Snapshot(), input: input}
	if m.source == nil {
		if !m.jiraActive && !m.copilotActive {
			m.mmReader.active = true
		}
		m.booting = false
	}
	return m
}
func (m *Model) mmCommand(operation func(context.Context) error) tea.Cmd {
	s := m.mmReader
	if s == nil || s.busy {
		return nil
	}
	s.busy = true
	authenticated := s.authenticated
	service := s.service
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		err := operation(ctx)
		if errors.Is(err, mm.ErrSession) || errors.Is(err, mm.ErrAccount) {
			authenticated = false
		}
		return mmResult{data: service.Snapshot(), err: err, authenticated: authenticated}
	}
}
func (m *Model) mmStart() tea.Cmd {
	s := m.mmReader
	if s == nil {
		return nil
	}
	s.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		token, _ := s.keychain.Load(ctx)
		// An explicitly supplied PAT takes precedence over a saved session.
		if s.apiToken != "" {
			token = s.apiToken
		}
		if token == "" {
			return mmResult{data: s.service.Snapshot(), err: mm.ErrSession}
		}
		s.api.SetToken(token)
		if _, err := s.api.Me(ctx); err != nil {
			return mmResult{data: s.service.Snapshot(), err: err, authenticated: !errors.Is(err, mm.ErrSession) && !errors.Is(err, mm.ErrAccess) && !errors.Is(err, mm.ErrAccount)}
		}
		err := s.service.Discover(ctx)
		if err == nil {
			err = s.service.Sync(ctx)
		}
		return mmResult{data: s.service.Snapshot(), authenticated: !errors.Is(err, mm.ErrSession), err: err}
	}
}
func (s *mattermostReader) channels() []model.MattermostChannel {
	var out []model.MattermostChannel
	for _, c := range s.data.Channels {
		if c.Selected {
			out = append(out, c)
		}
	}
	return out
}
func (s *mattermostReader) channel() model.MattermostChannel {
	cs := s.channels()
	if s.tab >= 0 && s.tab < len(cs) {
		return cs[s.tab]
	}
	return model.MattermostChannel{}
}
func (s *mattermostReader) posts() []model.MattermostPost {
	var out []model.MattermostPost
	c := s.channel()
	for _, p := range s.data.Posts {
		link := strings.TrimRight(s.data.Server, "/") + "/_redirect/pl/" + p.ID
		if s.entities.archived("mattermost", link) || (s.snoozes != nil && s.snoozes.hidden(snooze.Identity("mattermost", link))) || !strings.Contains(strings.ToLower(p.Message+" "+p.Author), strings.ToLower(s.query)) {
			continue
		}
		if p.DeletedAt != 0 {
			continue
		}
		if s.mentions {
			for _, link := range mm.PRLinks(p.Message) {
				if link.URL == s.mentionURL {
					out = append(out, p)
					break
				}
			}
			continue
		}
		if p.ChannelID != c.ID {
			continue
		}
		if s.root != "" && p.ID != s.root && p.RootID != s.root {
			continue
		}
		out = append(out, p)
	}
	return out
}
func (s *mattermostReader) choices() []model.MattermostChannel {
	var out []model.MattermostChannel
	q := strings.ToLower(s.input.Value())
	for _, c := range s.data.Channels {
		if strings.Contains(strings.ToLower(c.Name+" "+c.DisplayName), q) {
			out = append(out, c)
		}
	}
	return out
}
func (s *mattermostReader) clamp() {
	s.tab = max(0, min(s.tab, len(s.channels())))
	s.row = max(0, min(s.row, len(s.posts())-1))
	s.channelRow = max(0, min(s.channelRow, len(s.choices())-1))
}
func (s *mattermostReader) changeTab(delta int) {
	n := len(s.channels()) + 1
	s.tab = (s.tab + delta + n) % n
	s.row = 0
	s.root = ""
	s.scroll = 0
	s.link = 0
}
func (s *mattermostReader) prompt(mode string) tea.Cmd {
	s.mode = mode
	s.input.SetValue("")
	s.input.EchoMode = textinput.EchoPassword
	s.input.Prompt = mode + ": "
	return s.input.Focus()
}
func (m *Model) mmLogin(value string, pat bool) tea.Cmd {
	s := m.mmReader
	if s.busy {
		return nil
	}
	password := s.password
	s.password = ""
	s.mode = ""
	s.input.SetValue("")
	s.input.Blur()
	s.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		token := value
		var err error
		if !pat {
			token, err = s.api.Login(ctx, s.account, password, value)
		}
		if err != nil {
			return mmResult{data: s.service.Snapshot(), err: err}
		}
		s.api.SetToken(token)
		if _, err = s.api.Me(ctx); err != nil {
			return mmResult{data: s.service.Snapshot(), err: err}
		}
		notice := ""
		if s.keychain.Save(ctx, token) != nil {
			notice = "OS Keychain unavailable; session kept only in memory"
		}
		err = s.service.Discover(ctx)
		if err == nil {
			err = s.service.Sync(ctx)
		}
		return mmResult{data: s.service.Snapshot(), authenticated: !errors.Is(err, mm.ErrSession), err: err, notice: notice}
	}
}
func (m *Model) mmRead() tea.Cmd {
	s := m.mmReader
	c := s.channel()
	if c.ID == "" {
		return nil
	}
	at := c.ReadAt
	for _, p := range s.posts() {
		at = max(at, p.CreatedAt)
	}
	return m.mmCommand(func(context.Context) error { return s.service.MarkRead(c.ID, at) })
}
func (m *Model) mmMessage(msg tea.Msg) (bool, tea.Cmd) {
	s := m.mmReader
	if s == nil {
		return false, nil
	}
	switch msg := msg.(type) {
	case openMsg:
		if s.active {
			if msg.err != nil {
				s.notice = "Could not open the selected link"
			}
			return true, nil
		}
		return false, nil
	case mmResult:
		s.busy = false
		s.authenticated = msg.authenticated
		s.data = msg.data
		s.notice = msg.notice
		if msg.err != nil {
			s.notice = msg.err.Error()
		}
		s.clamp()
		if s.jumpID != "" {
			for _, p := range s.data.Posts {
				if p.ID == s.jumpID {
					for i, c := range s.channels() {
						if c.ID == p.ChannelID {
							s.tab = i
							break
						}
					}
					break
				}
			}
			for i, p := range s.posts() {
				if p.ID == s.jumpID {
					s.row = i
					s.jumpID = ""
					break
				}
			}
		}
		if errors.Is(msg.err, mm.ErrSession) && s.active && s.mode == "" {
			return true, s.prompt("Password")
		}
		return true, nil
	case mmTick:
		var cmd tea.Cmd
		if s.authenticated {
			cmd = m.mmCommand(s.service.Sync)
		}
		return true, tea.Batch(cmd, mmTimer())
	case tea.KeyPressMsg:
		key := msg.String()
		if !s.active {
			if key == "M" && !m.filtering && !m.tasks.active && !m.settings.active && !m.help && (m.mattermost == nil || !m.mattermost.active) && (m.jira == nil || m.jira.mode == "") && (m.copilot == nil || !m.copilot.filtering) {
				s.active = true
				return true, nil
			}
			if key == "v" && !m.jiraActive && !m.copilotActive && !m.tasks.active && !m.filtering && !m.settings.active && !m.help && (m.mattermost == nil || !m.mattermost.active) {
				rows := m.filtered()
				if len(rows) > 0 {
					s.mentionURL = strings.ToLower(rows[m.selected].URL)
					s.mentions = true
					s.active = true
					s.row = 0
				}
				return true, nil
			}
			return false, nil
		}
		if key == "ctrl+c" {
			m.cancel()
			return true, tea.Quit
		}
		if s.searching {
			switch key {
			case "esc":
				s.query = ""
				s.searching = false
				s.input.Blur()
			case "enter":
				s.searching = false
				s.input.Blur()
			default:
				var cmd tea.Cmd
				s.input, cmd = s.input.Update(msg)
				s.query = s.input.Value()
				s.row = 0
				return true, cmd
			}
			s.row = 0
			return true, nil
		}
		if s.mode != "" {
			if key == "esc" {
				s.mode = ""
				s.password = ""
				s.input.SetValue("")
				return true, nil
			}
			if key == "enter" {
				switch s.mode {
				case "Password":
					s.password = s.input.Value()
					return true, s.prompt("TOTP (empty if disabled)")
				case "PAT":
					return true, m.mmLogin(s.input.Value(), true)
				default:
					return true, m.mmLogin(s.input.Value(), false)
				}
			}
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return true, cmd
		}
		if s.confirmClear {
			s.confirmClear = false
			if key == "enter" {
				return true, m.mmCommand(func(ctx context.Context) error { err := s.service.Clear(); _ = s.keychain.Delete(ctx); return err })
			}
			return true, nil
		}
		if s.choosing {
			if s.channelSearching {
				switch key {
				case "esc":
					s.channelSearching = false
					s.input.SetValue("")
				case "enter":
					s.channelSearching = false
				default:
					var cmd tea.Cmd
					s.input, cmd = s.input.Update(msg)
					s.channelRow = 0
					return true, cmd
				}
				return true, nil
			}
			switch key {
			case "esc":
				s.choosing = false
				s.input.Blur()
			case "up", "k":
				s.channelRow = max(0, s.channelRow-1)
			case "down", "j":
				s.channelRow = min(max(0, len(s.choices())-1), s.channelRow+1)
			case "enter":
				cs := s.choices()
				if len(cs) > 0 {
					c := cs[s.channelRow]
					return true, m.mmCommand(func(ctx context.Context) error {
						if err := s.service.Select(c.ID, !c.Selected); err != nil {
							return err
						}
						if s.authenticated {
							return s.service.Sync(ctx)
						}
						return nil
					})
				}
			case "/":
				s.channelSearching = true
				return true, s.input.Focus()
			}
			return true, nil
		}
		switch key {
		case "q":
			m.cancel()
			return true, tea.Quit
		case "esc":
			if s.root != "" {
				s.root = ""
				s.row = 0
			} else {
				s.active = false
				s.mentions = false
			}
			s.scroll = 0
		case "A":
			if !s.busy {
				return true, s.prompt("Password")
			}
		case "p":
			if !s.busy {
				return true, s.prompt("PAT")
			}
		case "D":
			if !s.busy {
				s.confirmClear = true
			}
		case "R":
			if s.authenticated {
				return true, m.mmCommand(s.service.Sync)
			}
		case "+":
			s.choosing = true
			s.input.SetValue("")
			s.input.EchoMode = textinput.EchoNormal
			s.input.Prompt = "Search channels: "
			return true, tea.Batch(s.input.Focus(), m.mmCommand(s.service.Discover))
		case "tab", "right":
			s.changeTab(1)
		case "shift+tab", "left":
			s.changeTab(-1)
		case "j", "down":
			s.row = min(max(0, len(s.posts())-1), s.row+1)
			s.scroll = 0
			s.link = 0
		case "k", "up":
			s.row = max(0, s.row-1)
			s.scroll = 0
			s.link = 0
		case "pgdown":
			s.scroll += max(1, m.height/2)
		case "pgup":
			s.scroll = max(0, s.scroll-max(1, m.height/2))
		case "u":
			return true, m.mmRead()
		case "-":
			c := s.channel()
			if c.ID != "" {
				return true, m.mmCommand(func(context.Context) error { return s.service.Select(c.ID, false) })
			}
		case "enter":
			if s.mentions {
				ps := s.posts()
				if len(ps) == 0 {
					return true, nil
				}
				p := ps[s.row]
				s.mentions = false
				s.root = p.RootID
				s.jumpID = p.ID
				s.row = 0
				s.scroll = 0
				for i, c := range s.channels() {
					if c.ID == p.ChannelID {
						s.tab = i
						for j, post := range s.posts() {
							if post.ID == p.ID {
								s.row = j
								s.jumpID = ""
								break
							}
						}
						return true, nil
					}
				}
				return true, m.mmCommand(func(context.Context) error { return s.service.Select(p.ChannelID, true) })
			}
			if s.tab == len(s.channels()) && !s.mentions {
				s.choosing = true
				s.input.SetValue("")
				s.input.EchoMode = textinput.EchoNormal
				s.input.Prompt = "Search channels: "
				return true, tea.Batch(s.input.Focus(), m.mmCommand(s.service.Discover))
			}
			ps := s.posts()
			if len(ps) > 0 {
				links := messageLinks(ps[s.row].Message)
				if len(links) > 0 {
					link := links[s.link%len(links)]
					open := s.open
					if open == nil {
						open = m.open
					}
					return true, func() tea.Msg { return openMsg{err: open(m.ctx, link)} }
				}
			}
		case "]":
			s.link++
		case "[":
			s.link = max(0, s.link-1)
		case "d":
			ps := s.posts()
			if len(ps) > 0 {
				p := ps[s.row]
				root := p.RootID
				if root == "" {
					root = p.ID
				}
				if s.mentions {
					s.mentions = false
					s.jumpID = p.ID
					for i, c := range s.channels() {
						if c.ID == p.ChannelID {
							s.tab = i
							break
						}
					}
				}
				s.root = root
				s.row = 0
				s.scroll = 0
				return true, m.mmCommand(func(ctx context.Context) error {
					if err := s.service.Select(p.ChannelID, true); err != nil {
						return err
					}
					if !s.authenticated {
						return nil
					}
					return s.service.More(ctx, p.ChannelID, "", root)
				})
			}
		case "H":
			ps := s.posts()
			if len(ps) > 0 {
				p := ps[0]
				return true, m.mmCommand(func(ctx context.Context) error { return s.service.More(ctx, p.ChannelID, p.ID, "") })
			}
		case "g":
			return true, m.mmGoPR()
		}
		return true, nil
	}
	return false, nil
}

var messageURL = regexp.MustCompile(`https?://[^\s<>]+`)

func messageLinks(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, u := range messageURL.FindAllString(text, -1) {
		u = strings.TrimRight(u, ").,;!?]\"'`")
		if !seen[u] {
			out = append(out, u)
			seen[u] = true
		}
	}
	return out
}
func (m *Model) mmGoPR() tea.Cmd {
	s := m.mmReader
	ps := s.posts()
	if len(ps) == 0 {
		return nil
	}
	urls := messageLinks(ps[s.row].Message)
	if len(urls) == 0 {
		return nil
	}
	links := mm.PRLinks(urls[s.link%len(urls)])
	if len(links) == 0 {
		s.notice = "Selected link is not a GitHub PR"
		return nil
	}
	target := links[0].URL

	for v, d := range m.snapshot.Datasets {
		for _, p := range d.PRs {
			if strings.EqualFold(p.URL, target) {
				m.active = model.View(v)
				m.input.SetValue("")
				m.relatedKey = ""
				m.filtering = false
				m.jiraActive = false
				m.copilotActive = false
				s.active = false
				for i, row := range m.filtered() {
					if strings.EqualFold(row.URL, target) {
						m.selected = i
						break
					}
				}
				return nil
			}
		}
	}
	s.notice = "PR er ikke i GitHub-cachen; el viser lenken"
	return nil
}
func (m Model) mmMentionCount(u string) int {
	if m.mmReader == nil {
		return 0
	}
	n := 0
	for _, p := range m.mmReader.data.Posts {
		if p.DeletedAt != 0 {
			continue
		}
		for _, link := range mm.PRLinks(p.Message) {
			if strings.EqualFold(link.URL, u) {
				n++
				break
			}
		}
	}
	return n
}
func (m Model) mmReaderView() tea.View {
	s := m.mmReader
	lines := []string{accent.Render(" MATTERMOST · M"), ""}
	cs := s.channels()
	tabs := make([]string, 0, len(cs)+1)
	for _, c := range cs {
		unread := false
		for _, p := range s.data.Posts {
			if p.ChannelID == c.ID && p.DeletedAt == 0 && p.CreatedAt > c.ReadAt {
				unread = true
				break
			}
		}
		label := "#" + c.Name
		if unread {
			label += " ●"
		}
		tabs = append(tabs, label)
	}
	tabs = append(tabs, "+ Add channel")
	// Keep the active tab visible by shifting the left edge as the user navigates.
	start := 0
	for start < s.tab && ansi.StringWidth(strings.Join(tabs[start:s.tab+1], " │ ")) > max(1, m.width-6) {
		start++
	}
	var visible []string
	for i := start; i < len(tabs); i++ {
		label := clean(tabs[i])
		if i == s.tab {
			label = accent.Render("[" + label + "]")
		}
		visible = append(visible, label)
	}
	lines[1] = strings.Join(visible, " │ ")
	if s.mentions {
		lines[1] = fmt.Sprintf("Mattermost mentions (%d) · %s", len(s.posts()), s.mentionURL)
	}
	if s.mode != "" {
		lines = append(lines, "Sign in as "+clean(s.account), s.input.View(), "Enter next / submit · Esc cancel")
	} else if s.confirmClear {
		lines = append(lines, "Tøm alle lokale chatkontoer og lagret sesjon? Enter bekreft · Esc avbryt")
	} else if s.choosing {
		lines = append(lines, s.input.View())
		choices := s.choices()
		start := max(0, s.channelRow-max(1, m.height-7)+1)
		for i := start; i < min(len(choices), start+max(1, m.height-7)); i++ {
			c := choices[i]
			mark := "[ ] "
			if c.Selected {
				mark = "[x] "
			}
			line := mark + clean(c.DisplayName) + " #" + clean(c.Name)
			if i == s.channelRow {
				line = selection.Render(line)
			}
			lines = append(lines, line)
		}
		lines = append(lines, "j/k ↑↓ velg · / søk · Enter overvåk/avslutt overvåking · Esc tilbake")
	} else {
		ps := s.posts()
		if len(ps) == 0 {
			lines = append(lines, "No cached messages. vk velger kanaler; al logger inn; ap bruker PAT.")
		} else {
			var body []string
			selectedLine := 0
			for i := s.row; i < len(ps); i++ {
				p := ps[i]
				if i == s.row {
					selectedLine = len(body)
				}
				author := p.Author
				if author == "" {
					author = p.UserID
				}
				prefix := "  "
				if m.extraSelected(m.postRef(p), "mattermost") {
					prefix += "✓ "
				}
				if i == s.row {
					prefix = "› "
				}
				if p.CreatedAt > s.channel().ReadAt {
					prefix += "● "
				}
				channel := ""
				if s.mentions {
					for _, c := range s.data.Channels {
						if c.ID == p.ChannelID {
							channel = "#" + c.Name + " "
						}
					}
				}
				title := prefix + channel + time.UnixMilli(p.CreatedAt).Local().Format("02 Jan 15:04") + " " + author
				if p.RootID != "" {
					title += " ↳ reply (d thread)"
				}
				body = append(body, accent.Render(clean(title)))
				for _, line := range renderMessage(p.Message, max(1, m.width-4)) {
					body = append(body, "    "+line)
				}
				for _, link := range mm.PRLinks(p.Message) {
					for _, d := range m.snapshot.Datasets {
						found := false
						for _, pr := range d.PRs {
							if strings.EqualFold(pr.URL, link.URL) {
								state := "Open"
								if pr.Draft {
									state = "Draft"
								}
								body = append(body, fmt.Sprintf("    PR #%d · %s · %s · %s", pr.Number, clean(pr.DisplayTitle(m.jiraPattern)), state, status(pr.Review)))
								found = true
								break
							}
						}
						if found {
							break
						}
					}
				}
				body = append(body, "")
				if len(body) > max(1, m.height-7)+s.scroll {
					break
				}
			}
			capacity := max(1, m.height-7)
			offset := min(max(0, len(body)-1), selectedLine+s.scroll)
			lines = append(lines, body[offset:min(len(body), offset+capacity)]...)
			links := messageLinks(ps[s.row].Message)
			if len(links) > 0 {
				lines = append(lines, muted.Render(fmt.Sprintf("Link %d/%d: %s", s.link%len(links)+1, len(links), clean(links[s.link%len(links)]))))
			}
		}
	}
	if s.searching {
		lines = append(lines, s.input.View())
	} else if s.query != "" {
		lines = append(lines, "Søk: "+clean(s.query))
	}
	notice := s.notice
	if at := s.channel().LastSyncAt; at > 0 {
		notice = "Last sync " + time.UnixMilli(at).Local().Format("15:04:05") + " · " + notice
	}
	if s.busy {
		notice = "Syncing… " + notice
	}
	if !s.authenticated {
		notice = "Offline / sign in: al passord, ap PAT · " + notice
	}
	lines = append(lines, warning.Render(clean(notice)))
	if s.mentions {
		lines = append(lines, muted.Render("d åpner tråden; rc/rg følger relasjoner"))
	}
	lines = append(lines, muted.Render(m.shortcuts()))
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, m.width), "…")
	}
	if len(lines) > m.height {
		lines = append(lines[:max(0, m.height-1)], lines[len(lines)-1])
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.WindowTitle = "custom-visning · Mattermost"
	return view
}

// Render a safe terminal subset of Markdown; raw terminal controls are stripped
// before applying local styles. URLs remain available to the link navigator.
func renderMessage(text string, width int) []string {
	var out []string
	inline := func(line string) string {
		line = markdownBold.ReplaceAllStringFunc(line, func(s string) string { return accent.Render(s[2 : len(s)-2]) })
		line = markdownCode.ReplaceAllStringFunc(line, func(s string) string { return muted.Render(s[1 : len(s)-1]) })
		line = markdownItalic.ReplaceAllStringFunc(line, func(s string) string { return lipgloss.NewStyle().Italic(true).Render(s[1 : len(s)-1]) })
		return line
	}
	code := false
	for _, line := range strings.Split(text, "\n") {
		line = clean(line)
		if strings.HasPrefix(line, "```") {
			code = !code
			continue
		}
		if code {
			line = muted.Render(line)
		} else if strings.HasPrefix(line, "#") {
			line = accent.Render(strings.TrimLeft(line, "# "))
		} else if strings.HasPrefix(line, ">") {
			line = muted.Render("│ " + strings.TrimPrefix(line, "> "))
		} else {
			line = inline(line)
		}
		out = append(out, strings.Split(ansi.Hardwrap(line, width, true), "\n")...)
	}
	return out
}

var markdownBold = regexp.MustCompile(`\*\*[^*]+\*\*`)
var markdownCode = regexp.MustCompile("`[^`]+`")
var markdownItalic = regexp.MustCompile(`\*[^*]+\*`)

func openMattermostBrowser(ctx context.Context, link string) error {
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return errors.New("invalid web URL")
	}
	return launchBrowser(ctx, link)
}

// WithMattermostConfig prepares local state only; authentication runs as a command.
// A partial setup remains editable in settings and does not prevent app startup.
func (m Model) WithMattermostConfig() (Model, error) {
	return m.withMattermostConfigToken(os.Getenv("MATTERMOST_TOKEN"))
}
func (m Model) withMattermostConfigToken(token string) (Model, error) {
	c, err := mm.LoadConfig()
	if err != nil {
		return m, err
	}
	return m.withMattermostSettings(c, token)
}
func (m Model) withMattermostSettings(c mm.Config, token string) (Model, error) {
	if c.Server == "" || c.Username == "" {
		return m, nil
	}
	api, err := mm.NewAPI(c.Server)
	if err != nil {
		return m, err
	}
	api.ExpectAccount(c.Username)
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return m, err
	}
	service, err := mm.OpenService(api, filepath.Join(cacheDir, "custom-visning", "mattermost.sqlite"), c.Server, c.Username)
	if err != nil {
		return m, err
	}
	if err := mm.SaveConfig(c); err != nil {
		return m, err
	}
	return m.WithMattermostReader(service, api, mm.Keychain{Server: c.Server, Account: c.Username}, c.Username, token), nil
}
