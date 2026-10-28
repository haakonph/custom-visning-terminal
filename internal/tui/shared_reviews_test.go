package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestSharedReviewsCombineOnlyMessagedRequests(t *testing.T) {
	m := ready(t)
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.User = "alice" })
	m.active = model.SharedReviews
	at := time.Now().Add(-time.Hour)
	personal := model.PullRequest{URL: "https://github.com/acme/api/pull/1", Repository: "acme/api", Title: "Personal", Author: "bob"}
	team := model.PullRequest{URL: "https://github.com/acme/api/pull/2", Repository: "acme/api", Title: "Team", Author: "carol"}
	unshared := model.PullRequest{URL: "https://github.com/acme/api/pull/3", Author: "bob"}
	own := model.PullRequest{URL: "https://github.com/acme/api/pull/4", Author: "Alice"}
	unrelated := model.PullRequest{URL: "https://github.com/acme/api/pull/5", Author: "bob"}
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[model.Reviews] = model.Dataset{PRs: []model.PullRequest{personal, unshared}, SyncedAt: at}
	})
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[model.TeamReviews] = model.Dataset{PRs: []model.PullRequest{personal, team, own}, SyncedAt: at.Add(-time.Minute)}
	})
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[model.Open].PRs = []model.PullRequest{unrelated} })
	m.mmReader = &mattermostReader{reviewRequests: map[string]time.Time{personal.URL: at, own.URL: at, unrelated.URL: at}}
	m.mattermost = &mattermostState{sent: map[string]time.Time{team.URL: at}}
	rows := m.filtered()
	if len(rows) != 2 {
		t.Fatalf("expected two unique shared requests, got %+v", rows)
	}
	if m.reviewLabel(personal) != "1h" {
		t.Fatal("missing review age")
	}
	m.input.SetValue("Team")
	if rows := m.filtered(); len(rows) != 1 || rows[0].URL != team.URL {
		t.Fatalf("filter: %+v", rows)
	}
	m.input.SetValue("")
	delete(m.mmReader.reviewRequests, personal.URL)
	if rows := m.filtered(); len(rows) != 1 || rows[0].URL != team.URL {
		t.Fatalf("removed message: %+v", rows)
	}
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[model.TeamReviews].PRs = nil })
	if len(m.filtered()) != 0 {
		t.Fatal("removed request retained")
	}
}

func TestSharedReviewTabNavigationAndState(t *testing.T) {
	m := ready(t)
	m.active = model.Reviews
	m.width = 60
	next, _ := m.changeTab(1)
	m = next.(Model)
	if m.active != model.SharedReviews {
		t.Fatal("shared requests must follow personal requests")
	}
	m.loading[model.TeamReviews] = true
	m.warnings[model.Reviews] = "partial"
	if !m.githubLoading(m.active) || !strings.Contains(m.githubWarning(m.active), "partial") {
		t.Fatal("source status missing")
	}
	if !strings.Contains(m.View().Content, model.SharedReviews.String()) {
		t.Fatal("missing tab")
	}
	next, _ = m.changeTab(1)
	if next.(Model).active != model.Reviewed {
		t.Fatal("reviewed PRs must follow shared requests")
	}
}

func TestSharedReviewsRefreshesBothSources(t *testing.T) {
	m := ready(t)
	m.active = model.SharedReviews
	if m.refresh(m.active) == nil || !m.loading[model.Reviews] || !m.loading[model.TeamReviews] {
		t.Fatal("manual refresh must fetch both review lists")
	}
	m.loading[model.Reviews], m.loading[model.TeamReviews] = false, false
	now := time.Now()
	m.auto.options = defaultRefreshOptions()
	for v := model.Open; v < model.ViewCount; v++ {
		m.auto.github[v] = refreshJob{Finished: now}
	}
	m.auto.github[model.TeamReviews].Finished = now.Add(-time.Minute)
	m, _ = update(m, refreshTick(now))
	if !m.loading[model.TeamReviews] {
		t.Fatal("team list must refresh at the active interval")
	}
}

func TestReviewedTabNavigationAndDataset(t *testing.T) {
	m := ready(t)
	m.active = model.SharedReviews
	next, _ := m.changeTab(1)
	m = next.(Model)
	if m.active != model.Reviewed {
		t.Fatal("reviewed tab must follow shared reviews")
	}
	p := model.PullRequest{URL: "https://github.com/acme/api/pull/99", Title: "Already reviewed", Author: "bob"}
	m.editPRs(func(s *model.Snapshot) { s.Datasets[model.Reviewed] = model.Dataset{PRs: []model.PullRequest{p}} })
	if rows := m.filtered(); len(rows) != 1 || rows[0].URL != p.URL {
		t.Fatalf("reviewed dataset: %+v", rows)
	}
	m.width = 60
	if !strings.Contains(m.View().Content, model.Reviewed.String()) {
		t.Fatal("active reviewed tab must be visible")
	}
	if m.refresh(m.active) == nil || !m.loading[model.Reviewed] {
		t.Fatal("reviewed tab must refresh independently")
	}
	next, _ = m.changeTab(1)
	if next.(Model).active != model.TeamReviews {
		t.Fatal("team reviews must follow reviewed PRs")
	}
	next, _ = next.(Model).changeTab(1)
	if next.(Model).active != model.Open {
		t.Fatal("last tab must wrap to open PRs")
	}
	next, _ = next.(Model).changeTab(-1)
	if next.(Model).active != model.TeamReviews {
		t.Fatal("reverse wrap must reach team reviews")
	}
}
