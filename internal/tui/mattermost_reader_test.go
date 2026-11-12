package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	mm "github.com/haakonph/custom-visning-terminal/internal/conversations"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func readerModel(t *testing.T) Model {
	m := ready(t)
	input := textinput.New()
	m.page = pageChat
	m.mmReader = &mattermostReader{authenticated: true, input: input, data: model.MattermostData{Server: "https://chat.example", Channels: []model.MattermostChannel{{ID: "c", Name: "backend", Selected: true}, {ID: "d", Name: "frontend", Selected: true}}, Posts: []model.MattermostPost{{ID: "p", ChannelID: "c", Message: "https://github.com/acme/api/pull/1", CreatedAt: 100}, {ID: "reply", ChannelID: "d", RootID: "root", Message: "https://github.com/acme/api/pull/1 https://github.com/acme/api/pull/2", CreatedAt: 200}}}}
	return m
}

func TestMattermostChannelStartsWithNewestMessage(t *testing.T) {
	m := readerModel(t)
	m.mmReader.data.Posts = []model.MattermostPost{
		{ID: "old", ChannelID: "c", CreatedAt: 100, Message: "old message"},
		{ID: "new", ChannelID: "c", CreatedAt: 300, Message: "new message"},
		{ID: "middle", ChannelID: "c", CreatedAt: 200, Message: "middle message"},
		{ID: "deleted", ChannelID: "c", CreatedAt: 400, DeletedAt: 500},
		{ID: "other-old", ChannelID: "d", CreatedAt: 100},
		{ID: "other-new", ChannelID: "d", CreatedAt: 300},
	}
	if got := m.mmReader.posts()[m.mmReader.row].ID; got != "new" {
		t.Fatalf("initial message = %s, want newest", got)
	}
	view := ansi.Strip(m.View().Content)
	if strings.Index(view, "new message") > strings.Index(view, "old message") || !strings.Contains(view, "new message") {
		t.Fatal("newest message must be visible first", view)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.mmReader.posts()[m.mmReader.row].ID; got != "middle" {
		t.Fatalf("down selected %s, want next older message", got)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if got := m.mmReader.posts()[m.mmReader.row].ID; got != "other-new" {
		t.Fatalf("channel switch selected %s, want newest", got)
	}
}

func TestMattermostSyncFollowsNewestUnlessReadingOlderMessage(t *testing.T) {
	for _, readingOlder := range []bool{false, true} {
		t.Run(fmt.Sprint(readingOlder), func(t *testing.T) {
			m := readerModel(t)
			m.mmReader.data.Posts = []model.MattermostPost{
				{ID: "old", ChannelID: "c", CreatedAt: 100},
				{ID: "new", ChannelID: "c", CreatedAt: 200},
			}
			if readingOlder {
				m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
			}
			data := m.mmReader.data
			data.Posts = append(append([]model.MattermostPost(nil), data.Posts...), model.MattermostPost{ID: "newest", ChannelID: "c", CreatedAt: 300})
			m, _ = update(m, indexedMMResult(mmResult{data: data, authenticated: true}))
			want := "newest"
			if readingOlder {
				want = "old"
			}
			if got := m.mmReader.posts()[m.mmReader.row].ID; got != want {
				t.Fatalf("after sync selected %s, want %s", got, want)
			}
		})
	}
}

func TestMattermostThreadRetainsChronologicalOrder(t *testing.T) {
	m := readerModel(t)
	m.mmReader.root = "root"
	m.mmReader.data.Posts = []model.MattermostPost{
		{ID: "root", ChannelID: "c", CreatedAt: 100},
		{ID: "reply", RootID: "root", ChannelID: "c", CreatedAt: 200},
	}
	posts := m.mmReader.posts()
	if len(posts) != 2 || posts[0].ID != "root" || posts[1].ID != "reply" {
		t.Fatal("thread must start with its root", posts)
	}
}

type historyReader struct {
	mm.ReaderAPI
	channel, before string
}

func (r *historyReader) Me(context.Context) (string, error) { return "me", nil }
func (r *historyReader) History(_ context.Context, channel, before string) ([]model.MattermostPost, error) {
	r.channel, r.before = channel, before
	return nil, mm.ErrAccess
}

func TestMattermostHistoryLoadsBeforeOldestMessage(t *testing.T) {
	m := readerModel(t)
	m.mmReader.data.Posts = []model.MattermostPost{
		{ID: "old", ChannelID: "c", CreatedAt: 100},
		{ID: "new", ChannelID: "c", CreatedAt: 300},
		{ID: "middle", ChannelID: "c", CreatedAt: 200},
	}
	api := &historyReader{}
	m.mmReader.service = &mm.Service{API: api, Data: m.mmReader.data}
	_, cmd := m.loadChatHistory()
	if cmd == nil {
		t.Fatal("missing history command")
	}
	cmd()
	if api.channel != "c" || api.before != "old" {
		t.Fatalf("history request = %s before %s, want c before old", api.channel, api.before)
	}
}

func TestMattermostTabsAndAddTab(t *testing.T) {
	m := readerModel(t)
	for _, want := range []int{1, 2, 0} {
		m, _ = sequence(m, "l")
		if m.mmReader.tab != want {
			t.Fatalf("got %d want %d", m.mmReader.tab, want)
		}
	}
	m.mmReader.changeTab(-1)
	if m.mmReader.tab != 2 {
		t.Fatal("backwards wrap")
	}
	m, _ = update(m, mmResult{data: m.mmReader.data, authenticated: true})
	if m.mmReader.tab != 2 {
		t.Fatal("refresh must retain Add channel tab")
	}
	m.mmReader.data.Channels = nil
	m.mmReader.changeTab(-1)
	if m.mmReader.tab != 0 {
		t.Fatal("empty tabs")
	}
}
func TestMattermostPRRoundTrip(t *testing.T) {
	m := readerModel(t)
	m, _ = sequence(m, "rg")
	if (m.page == pageChat) || m.active != model.Open || m.filtered()[m.selected].Number != 1 {
		t.Fatal("message to cached PR")
	}
	if m.mmMentionCount("https://github.com/acme/api/pull/1") != 2 {
		t.Fatal("multiple channels/replies lost")
	}
	m, _ = update(m, key('r'))
	m, _ = update(m, key('c'))
	if m.dialog == nil || len(m.dialog.rows) != 2 {
		t.Fatal("reverse references")
	}
	// The first mention is already in a selected channel, so offline jump needs no I/O.
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.mmReader.mentions || m.mmReader.channel().ID != "c" || m.mmReader.posts()[m.mmReader.row].ID != "p" {
		t.Fatal("reference to exact post")
	}
}
func TestMattermostOfflineAndExpiredSession(t *testing.T) {
	m := readerModel(t)
	data := m.mmReader.data
	m, cmd := update(m, mmResult{data: data, err: mm.ErrSession})
	if m.mmReader.authenticated || m.mmReader.mode != "Password" || cmd == nil {
		t.Fatal("session renewal prompt")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.mmReader.mode != "" || len(m.mmReader.posts()) != 1 {
		t.Fatal("offline cache unavailable")
	}
	m, _ = update(m, mmResult{data: data, err: errors.New("network unavailable")})
	if !strings.Contains(m.mmReader.notice, "network") || len(m.mmReader.posts()) != 1 {
		t.Fatal("offline failure")
	}
}
func TestMattermostNarrowTabsAndControlSafety(t *testing.T) {
	m := readerModel(t)
	m.width = 32
	m.height = 12
	for i := 0; i < 30; i++ {
		m.mmReader.data.Channels = append(m.mmReader.data.Channels, model.MattermostChannel{ID: fmt.Sprint(i), Name: fmt.Sprintf("channel%d", i), Selected: true})
	}
	m.mmReader.tab = 30
	view := m.View().Content
	if !strings.Contains(ansi.Strip(view), "[#channel28]") {
		t.Fatal(view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatal("line exceeds width")
		}
	}
	lines := renderMessage("# title\n**bold**\n```\nhello\x1b]52;secret\a\n```", 20)
	if strings.Contains(strings.Join(lines, "\n"), "\x1b]52") {
		t.Fatal("terminal escape injection")
	}
}
func TestMattermostLinkNavigationWithoutGitHub(t *testing.T) {
	m := readerModel(t)
	m.useSource(nil)
	m.editPRs(func(snapshot *model.Snapshot) { *snapshot = model.Snapshot{} })
	opened := ""
	m.open = func(_ context.Context, u string) error { opened = u; return nil }
	m, _ = sequence(m, "el")
	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("missing browser command")
	}
	m, _ = update(m, cmd())
	if opened != "https://github.com/acme/api/pull/1" {
		t.Fatal(opened)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = sequence(m, "rg")
	if !(m.page == pageChat) || m.sequenceNotice == "" {
		t.Fatal("GitHub must be optional")
	}
}
func TestMattermostLoginSecretsClearedOnCancel(t *testing.T) {
	m := readerModel(t)
	m.mmReader.authenticated = false
	m, _ = sequence(m, "al")
	m.mmReader.input.SetValue("password-secret")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.mmReader.password != "password-secret" || m.mmReader.input.Value() != "" {
		t.Fatal("password prompt")
	}
	m.mmReader.input.SetValue("123456")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.mmReader.password != "" || m.mmReader.input.Value() != "" {
		t.Fatal("credentials retained after cancellation")
	}
}
