package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestMattermostPRText(t *testing.T) {
	m := ready(t)
	p := model.PullRequest{Repository: "skatteetaten-ghe/glsk-saksbehandling", Title: "ABC-123 fix [a]", URL: "https://example.com/(pr)"}
	want := `[\[saksbehandling\] ABC-123 fix \[a\]](https://example.com/%28pr%29)`
	if got := m.mattermostPRText(p); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

type fakeMessageSender struct {
	calls    int
	messages []string
	failAt   int
	text     string
	err      error
}

func (f *fakeMessageSender) Send(_ context.Context, text string) error {
	f.calls++
	if f.calls == f.failAt {
		return errors.New("failed")
	}
	f.messages = append(f.messages, text)
	f.text = text
	return f.err
}
func TestMattermostSendRetainsDraftAndBlocksDuplicate(t *testing.T) {
	sender := &fakeMessageSender{err: errors.New("failed")}
	m := ready(t).WithMattermost(sender)
	m.mattermost.active = true
	m.mattermost.text = "**edited draft**"
	send := "as"
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
	m, cmd := sequence(m, "as")
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
	sender := &fakeMessageSender{}
	m := ready(t).WithMattermost(sender)
	m, _ = update(m, key(' '))
	m, _ = update(m, key('j'))
	m, _ = update(m, key(' '))
	m.snapshot.Datasets[0].PRs[0].Title = "Fresh title"
	m.input.SetValue("no visible matches")
	cmd := m.composeMattermost()
	if cmd == nil || !m.mattermost.sending {
		t.Fatal("send not started immediately")
	}
	if duplicate := m.composeMattermost(); duplicate != nil {
		t.Fatal("duplicate send")
	}
	m, _ = update(m, cmd())
	if sender.calls != 2 || m.mattermost.active {
		t.Fatal("expected one message per selected PR")
	}
	if !strings.Contains(sender.messages[0], "Fresh title") || !strings.Contains(sender.messages[1], "Add endpoint") {
		t.Fatal(sender.messages)
	}
}

func TestMattermostPartialFailureRetriesOnlyRemaining(t *testing.T) {
	sender := &fakeMessageSender{failAt: 2}
	m := ready(t).WithMattermost(sender)
	cmd := m.sendMattermost([]string{"first", "second", "third"})
	m, _ = update(m, cmd())
	if !m.mattermost.active || m.mattermost.sending || strings.Join(m.mattermost.messages, ",") != "second,third" {
		t.Fatal("remaining messages lost")
	}
	sender.failAt = 0
	m, cmd = sequence(m, "as")
	m, _ = update(m, cmd())
	if m.mattermost.active || strings.Join(sender.messages, ",") != "first,second,third" {
		t.Fatal(sender.messages)
	}
}

func TestComposeReferencesSendsUncachedPRsSeparately(t *testing.T) {
	sender := &fakeMessageSender{}
	m := ready(t).WithMattermost(sender)
	refs := []model.TaskReference{
		{Provider: "github", Key: "skatteetaten-ghe/glsk-saksbehandling#3", Title: "ABC-123 fix", URL: "https://github.com/other/repo/pull/3"},
		{Provider: "github", Key: "skatteetaten-ghe/glsk-saksbehandling", Title: "ABC-123 tests", URL: "https://github.com/other/repo/pull/4"},
	}
	next, cmd := m.composeReferences(append(refs, refs[0]))
	m = next.(Model)
	if cmd == nil || !m.mattermost.sending {
		t.Fatal("send not started")
	}
	m, _ = update(m, cmd())
	if sender.calls != 2 || m.mattermost.active {
		t.Fatal("expected one message per distinct PR")
	}
	for i, ref := range refs {
		want := markdownLink("[saksbehandling] "+ref.Title, ref.URL)
		if sender.messages[i] != want {
			t.Fatalf("got %q, want %q", sender.messages[i], want)
		}
	}
}

func TestReviewAgeTracksOnlySuccessfullySentLinksAndSurvivesRestart(t *testing.T) {
	m := ready(t).WithMattermost(&fakeMessageSender{err: errors.New("offline")})
	p := m.snapshot.Datasets[0].PRs[0]
	p.Review = model.Pending
	if got := m.reviewLabel(p); got != string(model.Pending) {
		t.Fatal(got)
	}
	m.mattermost.active = true
	m.mattermost.text = "[PR](" + p.URL + ")"
	m, cmd := sequence(m, "as")
	m, _ = update(m, cmd())
	if len(m.mattermost.sent) != 0 {
		t.Fatal("failed send recorded")
	}
	m.mattermost.sender = &fakeMessageSender{}
	m, cmd = sequence(m, "as")
	m, _ = update(m, cmd())
	if got := m.reviewLabel(p); got != "now" {
		t.Fatal(got)
	}
	m = m.WithMattermost(&fakeMessageSender{})
	if got := m.reviewLabel(p); got != "now" {
		t.Fatal("restart lost send time", got)
	}
	m.mattermost.sent[p.URL] = time.Now().Add(-2 * time.Hour)
	for _, width := range []int{60, 100, 120, 160} {
		m.width = width
		if row := m.row(p, false, true); !strings.Contains(row, "2h") || strings.Contains(row, "PENDING") {
			t.Fatal(row)
		}
	}
	other := p
	other.URL += "0"
	if got := m.reviewLabel(other); got != string(model.Pending) {
		t.Fatal("unmentioned PR changed", got)
	}
	p.Review = model.Status("APPROVED")
	if got := m.reviewLabel(p); got != "APPROVED" {
		t.Fatal(got)
	}
}

func TestReviewAgePartialSendPersistsOnlySuccessfulMessages(t *testing.T) {
	sender := &fakeMessageSender{failAt: 2}
	m := ready(t).WithMattermost(sender)
	first, second := m.snapshot.Datasets[0].PRs[0], m.snapshot.Datasets[0].PRs[1]
	cmd := m.sendMattermost([]string{m.mattermostPRText(first), m.mattermostPRText(second)})
	m, _ = update(m, cmd())
	if m.mattermost.sent[first.URL].IsZero() || !m.mattermost.sent[second.URL].IsZero() {
		t.Fatal("partial send timestamps incorrect")
	}
	at := m.mattermost.sent[first.URL]
	m = m.WithMattermost(sender)
	if !m.mattermost.sent[first.URL].Equal(at) || !m.mattermost.sent[second.URL].IsZero() {
		t.Fatal("partial send timestamps not persisted")
	}
	sender.failAt = 0
	cmd = m.sendMattermost([]string{m.mattermostPRText(second)})
	m, _ = update(m, cmd())
	if !m.mattermost.sent[first.URL].Equal(at) || m.mattermost.sent[second.URL].IsZero() {
		t.Fatal("retry changed successful send timestamp")
	}
}
