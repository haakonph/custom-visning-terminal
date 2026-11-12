package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	mm "github.com/haakonph/custom-visning-terminal/internal/conversations"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type sharedReviewReader struct {
	posts []model.MattermostPost
	err   error
	syncs int
}

func (r *sharedReviewReader) Me(context.Context) (string, error) { return "me", r.err }
func (r *sharedReviewReader) Channels(context.Context) ([]model.MattermostChannel, error) {
	return []model.MattermostChannel{{ID: "reviews", Selected: true}}, nil
}
func (r *sharedReviewReader) Posts(context.Context, string, int64) ([]model.MattermostPost, error) {
	r.syncs++
	return r.posts, r.err
}
func (r *sharedReviewReader) Thread(context.Context, string) ([]model.MattermostPost, error) {
	return nil, nil
}
func (r *sharedReviewReader) History(context.Context, string, string) ([]model.MattermostPost, error) {
	return nil, nil
}
func (r *sharedReviewReader) Author(context.Context, string) (string, error) {
	return "bob", nil
}

func TestSharedReviewsManualRefreshFetchesNewMessages(t *testing.T) {
	m := ready(t)
	m.active = model.SharedReviews
	p := model.PullRequest{URL: "https://github.com/acme/api/pull/42", Title: "New request", Author: "bob"}
	m.useSource(&fakeSource{result: model.Result{PRs: []model.PullRequest{p}, Complete: true}})
	m.editPRs(func(s *model.Snapshot) { s.Datasets[model.Reviews].PRs = []model.PullRequest{p} })
	reader := &sharedReviewReader{}
	service, err := mm.OpenService(reader, filepath.Join(t.TempDir(), "chat.sqlite"), "https://chat.example", "me")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Discover(m.ctx); err != nil {
		t.Fatal(err)
	}
	m = m.WithMattermostReader(service, nil, nil, "me", "")
	m.mmReader.authenticated = true
	// Warm the real view cache before a new share arrives.
	if rows := m.filtered(); len(rows) != 0 {
		t.Fatal(rows)
	}
	reader.posts = []model.MattermostPost{{ID: "share", ChannelID: "reviews", UserID: "bob", Message: p.URL, CreatedAt: time.Now().UnixMilli()}}
	next, cmd := m.refreshVisible()
	m = next.(Model)
	if !m.mmReader.busy || !m.loading[model.Reviews] || !m.loading[model.TeamReviews] {
		t.Fatal("manual refresh must fetch messages and both GitHub lists")
	}
	for _, fetch := range cmd().(tea.BatchMsg) {
		next, _ = m.Update(fetch())
		m = next.(Model)
	}
	if reader.syncs != 1 {
		t.Fatalf("message syncs = %d, want 1", reader.syncs)
	}
	if rows := m.filtered(); len(rows) != 1 || rows[0].URL != p.URL {
		t.Fatalf("new share missing after refresh: %+v", rows)
	}
	reader.err = errors.New("network unavailable")
	next, cmd = m.refreshVisible()
	m = next.(Model)
	for _, fetch := range cmd().(tea.BatchMsg) {
		next, _ = m.Update(fetch())
		m = next.(Model)
	}
	if warning := m.githubWarning(m.active); !strings.Contains(warning, "Mattermost: network unavailable") {
		t.Fatalf("hidden message sync failure: %q", warning)
	}
	if rows := m.filtered(); len(rows) != 1 || rows[0].URL != p.URL {
		t.Fatalf("failed sync damaged cached requests: %+v", rows)
	}
	// A background message edit must also invalidate the cached shared list.
	reader.err = nil
	reader.posts[0].Message = "link removed"
	reader.posts[0].UpdatedAt = reader.posts[0].CreatedAt + 1
	_, cmd = m.refreshChat()
	next, _ = m.Update(cmd())
	m = next.(Model)
	if rows := m.filtered(); len(rows) != 0 {
		t.Fatalf("removed share still cached: %+v", rows)
	}
}

func TestSharedReviewsRefreshRetriesOfflineReaderAndAvoidsOverlap(t *testing.T) {
	m := ready(t)
	m.active = model.SharedReviews
	m.mmReader = &mattermostReader{}
	if warning := m.githubWarning(m.active); !strings.Contains(warning, "Mattermost: offline") {
		t.Fatalf("missing offline status: %q", warning)
	}
	if m.refresh(m.active) == nil || !m.mmReader.busy {
		t.Fatal("offline reader must retry authentication")
	}
	if !m.githubLoading(m.active) {
		t.Fatal("message refresh must contribute to shared loading status")
	}
	m.loading[model.Reviews], m.loading[model.TeamReviews] = false, false
	if !m.githubLoading(m.active) {
		t.Fatal("message refresh must remain visible after GitHub finishes")
	}
	if _, cmd := m.refreshChat(); cmd != nil {
		t.Fatal("in-flight authentication must not be restarted")
	}
	m.mmReader.authenticated = true
	if _, cmd := m.refreshChat(); cmd != nil {
		t.Fatal("in-flight sync must not be restarted")
	}
}
