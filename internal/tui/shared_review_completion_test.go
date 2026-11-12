package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	mm "github.com/haakonph/custom-visning-terminal/internal/conversations"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type completionSource struct {
	fakeSource
	completed bool
	err       error
	calls     int
	user      string
	since     time.Time
}

func (s *completionSource) ReviewedSince(ctx context.Context, _ model.PullRequest, user string, since time.Time) (bool, error) {
	s.calls++
	s.user, s.since = user, since
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return s.completed, s.err
}

type completionChat struct {
	mm.ReaderAPI
	posts []string
	err   error
}

func (c *completionChat) Me(context.Context) (string, error) { return "mattermost-me", nil }
func (c *completionChat) ReactToReview(_ context.Context, user, post string) error {
	if user != "mattermost-me" {
		panic("reaction must use the authenticated Mattermost user")
	}
	if c.err != nil {
		return c.err
	}
	c.posts = append(c.posts, post)
	return nil
}

func completionFixture(t *testing.T) (Model, *completionSource, *completionChat, model.PullRequest) {
	t.Helper()
	m := ready(t)
	source, chat := &completionSource{completed: true}, &completionChat{}
	m.useSource(source)
	pr := model.PullRequest{URL: "https://github.com/acme/api/pull/42", Repository: "acme/api", Number: 42, Author: "bob"}
	m.editPRs(func(s *model.Snapshot) {
		s.Datasets[model.Reviews].PRs = []model.PullRequest{pr}
	})
	at := time.Now().Add(-time.Hour).UnixMilli()
	data := model.MattermostData{Channels: []model.MattermostChannel{{ID: "selected", Selected: true}, {ID: "off"}}, Posts: []model.MattermostPost{
		{ID: "older", ChannelID: "selected", Message: pr.URL, CreatedAt: at - 1000},
		{ID: "original", RootID: "thread", ChannelID: "selected", Message: "Review " + pr.URL, CreatedAt: at},
		{ID: "off", ChannelID: "off", Message: pr.URL, CreatedAt: at + 1000},
		{ID: "deleted", ChannelID: "selected", Message: pr.URL, CreatedAt: at + 2000, DeletedAt: at + 3000},
	}}
	service := &mm.Service{API: chat, Data: data}
	m = m.WithMattermostReader(service, nil, nil, "alice", "")
	m.mmReader.authenticated = true
	return m, source, chat, pr
}

func TestSharedReviewDisappearanceReactsOnlyAfterBothListsRemoveIt(t *testing.T) {
	m, source, chat, pr := completionFixture(t)
	// Completion works in the background, with filters and overlays active.
	m.settings.active = true
	m.input.SetValue("unrelated")
	m.editPRs(func(s *model.Snapshot) { s.Datasets[model.TeamReviews].PRs = []model.PullRequest{pr} })
	m, cmd := update(m, refreshMsg{view: model.Reviews, result: model.Result{Complete: true}, at: time.Now()})
	if cmd != nil || source.calls != 0 || len(chat.posts) != 0 {
		t.Fatal("PR still belongs to shared requests")
	}
	m, cmd = update(m, refreshMsg{view: model.TeamReviews, result: model.Result{Complete: true}, at: time.Now()})
	if cmd == nil || source.calls != 0 || len(chat.posts) != 0 {
		t.Fatal("network I/O must be deferred to a command")
	}
	m, _ = update(m, cmd())
	if source.calls != 1 || source.user != "alice" || !source.since.Equal(time.UnixMilli(m.mmReader.data.Posts[1].CreatedAt)) {
		t.Fatal("review verification did not use the current user and latest selected share", source)
	}
	if len(chat.posts) != 1 || chat.posts[0] != "original" {
		t.Fatal("must react on the actual shared message, including replies", chat.posts)
	}
	m, cmd = update(m, refreshMsg{view: model.TeamReviews, result: model.Result{Complete: true}, at: time.Now()})
	if cmd != nil || len(chat.posts) != 1 || m.mmReader.busy {
		t.Fatal("unchanged refresh must not repeat completion")
	}
}

func TestSharedReviewCompletionRejectsUnconfirmedDisappearance(t *testing.T) {
	for _, scenario := range []string{"incomplete", "failed", "not reviewed", "deleted message", "deselected channel", "stale response"} {
		t.Run(scenario, func(t *testing.T) {
			m, source, chat, _ := completionFixture(t)
			result := refreshMsg{view: model.Reviews, result: model.Result{Complete: true}, at: time.Now()}
			switch scenario {
			case "incomplete":
				result.result.Complete = false
			case "failed":
				result.err = errors.New("offline")
			case "not reviewed":
				source.completed = false
			case "deleted message":
				for i := range m.mmReader.data.Posts {
					m.mmReader.data.Posts[i].DeletedAt = 1
				}
				m.mmReader.reviewRequests = mattermostReviewTimes(m.mmReader.data)
			case "deselected channel":
				m.mmReader.data.Channels[0].Selected = false
				m.mmReader.reviewRequests = mattermostReviewTimes(m.mmReader.data)
			case "stale response":
				result.token = m.prs.Begin("list:2")
				m.prs.Begin("list:2")
			}
			m, cmd := update(m, result)
			if cmd != nil {
				m, _ = update(m, cmd())
			}
			if len(chat.posts) != 0 || len(m.mmReader.reviewCompletions) != 0 {
				t.Fatal("reacted without a confirmed reviewed share", chat.posts)
			}
		})
	}
}

func TestSharedReviewCompletionWaitsForChatAndRetriesFailures(t *testing.T) {
	for _, scenario := range []string{"busy", "signed out", "GitHub failure", "Mattermost failure"} {
		t.Run(scenario, func(t *testing.T) {
			m, source, chat, _ := completionFixture(t)
			switch scenario {
			case "busy":
				m.mmReader.busy = true
			case "signed out":
				m.mmReader.authenticated = false
			case "GitHub failure":
				source.err = errors.New("offline")
			case "Mattermost failure":
				chat.err = mm.ErrSession
			}
			m, cmd := update(m, refreshMsg{view: model.Reviews, result: model.Result{Complete: true}, at: time.Now()})
			if cmd != nil {
				m, cmd = update(m, cmd())
			}
			if len(chat.posts) != 0 || len(m.mmReader.reviewCompletions) != 1 || cmd != nil {
				t.Fatal("failed or blocked reaction must wait without an immediate retry loop")
			}
			source.err, chat.err = nil, nil
			m, cmd = update(m, indexedMMResult(mmResult{data: m.mmReader.service.Snapshot(), authenticated: true}))
			if cmd == nil {
				t.Fatal("successful chat operation must resume pending completion")
			}
			m, _ = update(m, cmd())
			if len(chat.posts) != 1 || len(m.mmReader.reviewCompletions) != 0 {
				t.Fatal("retry did not finish", chat.posts)
			}
		})
	}
}

func TestSharedReviewCompletionCancelsAfterAccountReplacement(t *testing.T) {
	for _, scenario := range []string{"GitHub queued", "GitHub running", "Mattermost running", "request returned"} {
		t.Run(scenario, func(t *testing.T) {
			m, _, chat, pr := completionFixture(t)
			m.mmReader.busy = scenario == "GitHub queued" || scenario == "request returned"
			m, cmd := update(m, refreshMsg{view: model.Reviews, result: model.Result{Complete: true}, at: time.Now()})
			switch scenario {
			case "GitHub queued", "GitHub running":
				m.useSource(&completionSource{completed: true})
			case "Mattermost running":
				m.chatSession.Configure(m.ctx)
			case "request returned":
				m.editPRs(func(s *model.Snapshot) { s.Datasets[model.Reviews].PRs = []model.PullRequest{pr} })
			}
			if cmd != nil {
				m, _ = update(m, cmd())
			} else {
				m.mmReader.busy = false
				if m.reactToCompletedSharedReviews() != nil {
					t.Fatal("obsolete request should be discarded")
				}
			}
			if len(chat.posts) != 0 {
				t.Fatal("old integration wrote a reaction", chat.posts)
			}
		})
	}
}
