package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	mm "github.com/haakonph/custom-visning-terminal/internal/connectors/mattermost"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

func TestReviewAgeUsesCachedChannelMessagesWithoutWebhook(t *testing.T) {
	at := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
	url := "https://github.com/acme/api/pull/1"
	data := model.MattermostData{Server: "https://chat.example", UserID: "me", Channels: []model.MattermostChannel{{ID: "c", Selected: true}}, Posts: []model.MattermostPost{{ID: "external", ChannelID: "c", UserID: "other", Message: "Kan noen reviewe [PR](https://github.com/Acme/API/pull/1/files?diff=split#discussion)?", CreatedAt: at.UnixMilli()}}}
	path := filepath.Join(t.TempDir(), "mattermost.sqlite")
	if err := storage.SaveMattermost(path, data, map[string][]string{"external": {url}}); err != nil {
		t.Fatal(err)
	}
	service, err := mm.OpenService(nil, path, data.Server, data.UserID)
	if err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), nil, nil).WithMattermostReader(service, nil, nil, data.UserID, "")
	defer m.cancel()
	m.active = model.Reviews
	p := model.PullRequest{URL: url, Review: model.Pending}
	if m.mattermost != nil {
		t.Fatal("test must work without a sending webhook")
	}
	if got := m.reviewLabel(p); got != "2h" {
		t.Fatalf("cached channel request: %q", got)
	}
	if got := m.reviewRequestedAt(url); !got.Equal(at) {
		t.Fatalf("time %s want %s", got, at)
	}
	if got := m.reviewColor(p); got != "179" {
		t.Fatal(got)
	}
	p.ReviewSummary = &model.ReviewSummary{Approved: 1, Reviewers: map[string]string{"other": "APPROVED"}}
	if m.reviewLabel(p) != "2h" || m.reviewColor(p) != "108" {
		t.Fatal("another review must retain green age")
	}

	data.Posts = append(data.Posts, model.MattermostPost{ID: "new", ChannelID: "c", RootID: "thread", Message: url + " https://github.com/acme/api/pull/2", CreatedAt: time.Now().Add(-time.Hour).UnixMilli()})
	m, _ = update(m, mmResult{data: data, authenticated: true})
	if got := m.reviewLabel(p); got != "1h" {
		t.Fatalf("new message did not update request time: %q", got)
	}
	// Editing uses creation time; deleting the newer share reveals the older one.
	data.Posts[1].UpdatedAt = time.Now().UnixMilli()
	m, _ = update(m, mmResult{data: data, authenticated: true})
	if got := m.reviewLabel(p); got != "1h" {
		t.Fatal("edit reset age", got)
	}
	data.Posts[1].DeletedAt = time.Now().UnixMilli()
	m, _ = update(m, mmResult{data: data, authenticated: true})
	if got := m.reviewLabel(p); got != "2h" {
		t.Fatal("deleted message retained", got)
	}
	data.Posts[0].Message = "link removed"
	m, _ = update(m, mmResult{data: data, authenticated: true})
	if got := m.reviewLabel(p); got != "" {
		t.Fatal("removed link retained", got)
	}
}

func TestChannelReviewIndexScopeAndLocalSendFallback(t *testing.T) {
	at := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	url := "https://github.com/acme/api/pull/1"
	data := model.MattermostData{Channels: []model.MattermostChannel{{ID: "selected", Selected: true}, {ID: "off"}}, Posts: []model.MattermostPost{
		{ChannelID: "selected", Message: url, CreatedAt: at.UnixMilli()},
		{ChannelID: "off", Message: url, CreatedAt: at.Add(time.Minute).UnixMilli()},
		{ChannelID: "selected", Message: url, CreatedAt: at.Add(2 * time.Minute).UnixMilli(), DeletedAt: 1},
		{ChannelID: "unknown", Message: url, CreatedAt: at.Add(3 * time.Minute).UnixMilli()},
	}}
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.mmReader = &mattermostReader{reviewRequests: mattermostReviewTimes(data)}
	if !m.reviewRequestedAt(url).Equal(at) {
		t.Fatal("unselected or deleted share counted")
	}
	m.mattermost = &mattermostState{sent: map[string]time.Time{url: at.Add(-time.Hour)}}
	if !m.reviewRequestedAt(url).Equal(at) {
		t.Fatal("older local send overrides channel")
	}
	m.mattermost.sent[url] = at.Add(time.Minute)
	if !m.reviewRequestedAt(url).Equal(at.Add(time.Minute)) {
		t.Fatal("new local send not shown immediately")
	}
	data.Channels[0].Selected = false
	m.mmReader.reviewRequests = mattermostReviewTimes(data)
	m.mattermost = nil
	if !m.reviewRequestedAt(url).IsZero() {
		t.Fatal("deselected channel retained")
	}
}
