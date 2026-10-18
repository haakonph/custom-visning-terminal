package tui

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestMattermostDefaultText(t *testing.T) {
	issueURL := func(key string) string { return "https://jira.example/context/browse/" + key }
	cases := []struct {
		name     string
		titles   []string
		jira     bool
		wantJira bool
	}{
		{"single", []string{"ABC-1 fix"}, true, true},
		{"same", []string{"ABC-1 fix", "ABC-1 tests"}, true, true},
		{"different", []string{"ABC-1 fix", "ABC-2 tests"}, true, false},
		{"missing", []string{"ABC-1 fix", "tests"}, true, false},
		{"ambiguous", []string{"ABC-1 ABC-2 fix", "ABC-1 tests"}, true, false},
		{"unconfigured", []string{"ABC-1 fix"}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rows []model.PullRequest
			for _, title := range tc.titles {
				rows = append(rows, model.PullRequest{Title: title, URL: "https://github.com/a/b/pull/1"})
			}
			var link func(string) string
			if tc.jira {
				link = issueURL
			}
			text := mattermostText(rows, link)
			if strings.Contains(text, "https://jira.example") != tc.wantJira {
				t.Fatal(text)
			}
			if !strings.HasPrefix(text, "["+tc.titles[0]+"](https://github.com/a/b/pull/1)") {
				t.Fatal(text)
			}
			if strings.Count(text, "\n") != len(rows)-1+btoi(tc.wantJira) {
				t.Fatal(text)
			}
		})
	}
	if got := mattermostText([]model.PullRequest{{Title: "fix [a]", URL: "https://example.com/(pr)"}}, nil); got != `[fix \[a\]](https://example.com/%28pr%29)` {
		t.Fatal(got)
	}
	if got := mattermostText(nil, issueURL); got != "" {
		t.Fatal(got)
	}
}
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

type fakeMessageSender struct {
	calls int
	text  string
	err   error
}

func (f *fakeMessageSender) Send(_ context.Context, text string) error {
	f.calls++
	f.text = text
	return f.err
}
func TestMattermostSendRetainsDraftAndBlocksDuplicate(t *testing.T) {
	sender := &fakeMessageSender{err: errors.New("failed")}
	m := ready(t).WithMattermost(sender)
	m.mattermost.active = true
	m.mattermost.text = "**edited draft**"
	send := "ac"
	m, cmd := sequence(m, send)
	if cmd == nil || !m.mattermost.sending {
		t.Fatal("send not started")
	}
	m, duplicate := sequence(m, send)
	if duplicate != nil {
		t.Fatal("duplicate send")
	}
	m, _ = update(m, cmd())
	if !m.mattermost.active || m.mattermost.text != "**edited draft**" || m.mattermost.notice != "failed" {
		t.Fatal("draft lost")
	}
	sender.err = nil
	m, cmd = sequence(m, send)
	m, _ = update(m, cmd())
	if m.mattermost.active || sender.calls != 2 || sender.text != "**edited draft**" {
		t.Fatal("send failed")
	}
}
func TestMattermostNavigationAndEmptyDraft(t *testing.T) {
	m := ready(t).WithMattermost(&fakeMessageSender{})
	m, _ = update(m, key(' '))
	m, _ = update(m, key('j'))
	m, _ = update(m, key(' '))
	if len(m.chosen) != 2 {
		t.Fatal("selection requires local worktrees")
	}
	m.mattermost.active = true
	m.mattermost.text = " \n"
	m, cmd := sequence(m, "ac")
	if cmd != nil || m.mattermost.sending {
		t.Fatal("sent empty draft")
	}
	m, _ = update(m, key('q'))
	if !m.mattermost.active {
		t.Fatal("global shortcut intercepted")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.mattermost.active {
		t.Fatal("cancel failed")
	}
}

func TestComposeMattermostUsesSelectionAndFreshData(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // Keep this unit test out of the interactive editor.
	m := ready(t).WithMattermost(&fakeMessageSender{})
	m, _ = update(m, key(' '))
	m, _ = update(m, key('j'))
	m, _ = update(m, key(' '))
	m.snapshot.Datasets[0].PRs[0].Title = "Fresh title"
	m.input.SetValue("no visible matches")
	m.composeMattermost()
	if !m.mattermost.active || !strings.HasPrefix(m.mattermost.text, "[Fresh title]") || !strings.Contains(m.mattermost.text, "[Add endpoint]") {
		t.Fatal(m.mattermost.text)
	}
	if !strings.Contains(m.mattermost.notice, "Installer") {
		t.Fatal("missing editor not explained")
	}
	m.chosen = map[string]model.PullRequest{}
	m.input.SetValue("")
	m.selected = 1
	m.composeMattermost()
	if m.mattermost.text != "[Add endpoint](https://github.com/acme/api/pull/2)" {
		t.Fatal(m.mattermost.text)
	}
}

func TestMattermostRespectsConfiguredJiraPattern(t *testing.T) {
	rows := []model.PullRequest{{Title: "ABC-1 fix", URL: "https://github.com/a/b/pull/1"}}
	text := mattermostText(rows, func(key string) string { return "https://jira.example/browse/" + key }, regexp.MustCompile(`ABC-[0-9]{3,5}`))
	if strings.Contains(text, "jira.example") {
		t.Fatal("ignored configured Jira pattern: " + text)
	}
	rows[0].Title = "ABC-123 fix"
	text = mattermostText(rows, func(key string) string { return "https://jira.example/browse/" + key }, regexp.MustCompile(`ABC-[0-9]{3,5}`))
	if !strings.Contains(text, "[ABC-123](https://jira.example/browse/ABC-123)") {
		t.Fatal(text)
	}
}
