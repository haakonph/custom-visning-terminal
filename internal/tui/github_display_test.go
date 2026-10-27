package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	appconfig "github.com/haakonph/custom-visning-terminal/internal/config"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestGitHubDisplaySettingsAndIdentity(t *testing.T) {
	t.Setenv("CUSTOM_VISNING_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	p := model.PullRequest{Repository: "skatteetaten-ghe/glsk-service", Title: "[ABC-123] Fix login", URL: "https://github.com/skatteetaten-ghe/glsk-service/pull/1", Number: 1, AutoMerge: "ON", MergeQueue: "QUEUED", QueuePosition: 2}
	for _, width := range []int{40, 60, 100, 120, 140, 200} {
		m.width = width
		header, row := ansi.Strip(m.row(p, true, false)), ansi.Strip(m.row(p, false, false))
		if !strings.Contains(header, "JIRA") || !strings.Contains(row, "ABC-123") || strings.Contains(row, "skatteetaten-ghe") || strings.Contains(row, "glsk-") || strings.Contains(row, "[ABC-123]") {
			t.Fatalf("width=%d\n%s\n%s", width, header, row)
		}
		if ansi.StringWidth(row) > width {
			t.Fatalf("row exceeds width %d: %q", width, row)
		}
		if strings.Contains(header, "AUTO") {
			t.Fatal(header)
		}
		if width >= 100 && (!strings.Contains(header, "INFO") || strings.Contains(header, "QUEUE") || !strings.Contains(row, "#2 QUEUED · auto")) {
			t.Fatal(row)
		}
	}
	ref := m.displayReference(prRef(p))
	if ref.Key != "service#1" || ref.URL != p.URL || ref.Title != "Fix login · Jira: ABC-123" {
		t.Fatal(ref)
	}
	if m.githubName("other/skatteetaten-ghe/service") != "other/skatteetaten-ghe/service" {
		t.Fatal("removed non-prefix")
	}
	for name, want := range map[string]string{
		"glsk-service":                   "service",
		"other/glsk-service":             "other/service",
		"other/glsk-service#1":           "other/service#1",
		"other/service-glsk-api":         "other/service-glsk-api",
		"glsk-owner/service":             "glsk-owner/service",
		"skatteetaten-ghe/glsk-glsk-api": "glsk-api",
	} {
		if got := m.githubName(name); got != want {
			t.Fatalf("githubName(%q) = %q, want %q", name, got, want)
		}
	}
	if got := m.githubTeams([]string{"skatteetaten-ghe/glsk-team", "other/glsk-team"}); got != "glsk-team, other/glsk-team" {
		t.Fatal(got)
	}
	opened, _ := m.openSettings()
	m = opened.(Model)
	index := -1
	for i, field := range settingFields {
		if field.path == "github.hidden_prefixes" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("GitHub display setting missing")
	}
	m = editSetting(t, m, index, `[]`)
	if m.settings.editing {
		t.Fatal(m.settings.notice)
	}
	if m.githubName(p.Repository) != "skatteetaten-ghe/service" {
		t.Fatal("empty configuration should show organization and hide repository prefix")
	}
	if err := appconfig.Update(appconfig.Path(), map[string]any{"github": map[string]any{"hidden_prefixes": []string{"other"}}}); err != nil {
		t.Fatal(err)
	}
	m.loadGitHubDisplay()
	if m.githubName("other/glsk-service") != "service" || m.githubName(p.Repository) != "skatteetaten-ghe/service" {
		t.Fatal("custom prefix ignored")
	}
}

func TestPRInfoPrioritizesConflicts(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	for _, tc := range []struct {
		name     string
		merge    model.Status
		queue    model.Status
		position int
		want     string
	}{
		{"conflict without queue", model.Conflicting, "NONE", 0, "CONFLICT"},
		{"conflict in queue", model.Conflicting, "QUEUED", 2, "CONFLICT"},
		{"conflict with unknown queue", model.Conflicting, model.Unknown, 0, "CONFLICT"},
		{"queued", model.Mergeable, "QUEUED", 2, "#2 QUEUED"},
		{"no queue", model.Mergeable, "NONE", 0, ""},
		{"unknown", model.Unknown, model.Unknown, 0, "UNKNOWN"},
		{"missing", "", "", 0, "UNKNOWN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := model.PullRequest{Title: "Example", Mergeability: tc.merge, MergeQueue: tc.queue, QueuePosition: tc.position}
			if got := prInfoLabel(p); got != tc.want {
				t.Fatalf("info = %q, want %q", got, tc.want)
			}
			for _, width := range []int{100, 120, 140, 200} {
				m.width = width
				for _, selected := range []bool{false, true} {
					row := ansi.Strip(m.row(p, false, selected))
					if !strings.Contains(row, tc.want) || ansi.StringWidth(row) > width {
						t.Fatalf("width=%d selected=%t: %q", width, selected, row)
					}
				}
			}
			m.width = 60
			m.prs.Upsert(p)
			m.detail = &detailState{id: p.Key()}
			details := strings.Join(m.detailLines(), "\n")
			if strings.Contains(details, "CONFLICT:") != (tc.merge == model.Conflicting) {
				t.Fatalf("incorrect conflict explanation: %s", details)
			}
			if !strings.Contains(details, "Queue: "+queueLabel(p)) {
				t.Fatalf("queue status missing from details: %s", details)
			}
		})
	}
}

func TestTeamReviewHidesCachedOwnPRs(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.User = "alice" })
	m.active = model.TeamReviews
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[model.TeamReviews].PRs = []model.PullRequest{{Author: "Alice", URL: "https://github.com/acme/repo/pull/1"}, {Author: "bob", URL: "https://github.com/acme/repo/pull/2"}}
	})
	if rows := m.filtered(); len(rows) != 1 || rows[0].Author != "bob" {
		t.Fatal(rows)
	}
	m.active = model.Open
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[model.Open].PRs = snapshot.Datasets[model.TeamReviews].PRs
	})
	if len(m.filtered()) != 2 {
		t.Fatal("own PR hidden outside Team Review")
	}
}

func TestReviewSummaryRendering(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.width = 180
	p := model.PullRequest{Title: "Stacked PR", Review: model.ChangesRequested, ReviewSummary: &model.ReviewSummary{Approved: 2, Changes: 1, Pending: 1, PendingTeams: 1, Complete: true, Fallback: true}}
	m.prs.Upsert(p)
	m.detail = &detailState{id: p.Key()}
	text := strings.Join(m.detailLines(), "\n")
	for _, want := range []string{"Review: CHANGES (vurderinger)", "2 godkjent · 1 endringer · 1 venter", "1 team venter", "GitHubs regler avgjør"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	p.ReviewSummary.Fallback = false
	p.ReviewSummary.Complete = false
	if label := reviewSummaryLabel(p); !strings.Contains(label, "(GitHub)") || !strings.Contains(label, "delvis opptelling") {
		t.Fatal(label)
	}
	p.Review = model.NoReview
	p.ReviewSummary = nil
	for _, width := range []int{60, 100, 140} {
		m.width = width
		if row := ansi.Strip(m.row(p, false, false)); !strings.Contains(row, "NO REVIEW") {
			t.Fatal(row)
		}
	}
}

func TestCopilotReviewLabel(t *testing.T) {
	for _, tc := range []struct{ recommendation, want string }{
		{"Approval", "🟢"}, {"Changes", "🔴"}, {"Review", "🟡"}, {"Comments", "💬"}, {"Dismissed", "⚪"},
	} {
		if got := copilotReviewLabel(model.PullRequest{CopilotReview: &model.CopilotReview{Recommendation: tc.recommendation}}); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
	if got := copilotReviewLabel(model.PullRequest{}); got != "—" {
		t.Fatal(got)
	}
	p := model.PullRequest{CopilotReview: &model.CopilotReview{Recommendation: "Approval", Comments: 3}}
	if got := copilotReviewLabel(p); got != "🟢 3" {
		t.Fatal(got)
	}
	if got := reviewSummaryLabel(p); !strings.Contains(got, "Copilot: 🟢 3") {
		t.Fatal(got)
	}
}

func TestOpenCommentsInfo(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	for _, tc := range []struct {
		count     int
		status    model.Status
		autoMerge model.Status
		want      string
	}{
		{0, model.Mergeable, "", ""},
		{0, model.Conflicting, "", "CONFLICT"},
		{3, model.Mergeable, "", "3 åpne"},
		{12, model.Conflicting, "", "CONFLICT · 12 åpne"},
		{0, model.Mergeable, "ON", "auto"},
		{0, model.Mergeable, "OFF", ""},
		{0, model.Mergeable, model.Unknown, ""},
		{0, model.Conflicting, "ON", "CONFLICT · auto"},
		{3, model.Mergeable, "ON", "auto · 3 åpne"},
		{1, model.Conflicting, "ON", "CONFLICT · auto · 1 åpne"},
	} {
		p := model.PullRequest{OpenComments: &tc.count, Mergeability: tc.status, MergeQueue: "NONE", AutoMerge: tc.autoMerge}
		if got := prInfoLabel(p); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
		for _, width := range []int{100, 120, 140, 160, 200} {
			m.width = width
			row := ansi.Strip(m.row(p, false, false))
			if tc.count == 0 && strings.Contains(row, "0 åpne") {
				t.Fatalf("width %d: zero open comments shown: %q", width, row)
			}
			if !strings.Contains(row, ansi.Truncate(tc.want, 22, "…")) || ansi.StringWidth(row) > width {
				t.Fatalf("width %d: %q", width, row)
			}
		}
	}
}

func TestCopilotReviewPendingLabel(t *testing.T) {
	for _, login := range []string{"Copilot", "copilot-pull-request-reviewer", "copilot-pull-request-reviewer[bot]"} {
		for _, review := range []*model.CopilotReview{nil, {Recommendation: "Approval", Comments: 3}} {
			p := model.PullRequest{RequestedReviewers: []string{login}, CopilotReview: review}
			if got := copilotReviewLabel(p); got != "⏳" {
				t.Fatalf("%s: got %q", login, got)
			}
			if got := reviewSummaryLabel(p); !strings.Contains(got, "Copilot: ⏳") {
				t.Fatal(got)
			}
		}
	}
	if got := copilotReviewLabel(model.PullRequest{RequestedReviewers: []string{"alice"}}); got != "—" {
		t.Fatal(got)
	}
}

func TestReviewRequestAgeAndReviewerColors(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.User = "Alice" })
	m.mattermost = &mattermostState{sent: map[string]time.Time{}}
	p := model.PullRequest{URL: "https://github.com/acme/repo/pull/1", Review: model.Approved}
	for _, view := range []model.View{model.Reviews, model.TeamReviews} {
		m.active = view
		for _, tc := range []struct {
			name        string
			sent        bool
			reviewers   map[string]string
			want, color string
		}{
			{"not requested even if aggregate approved", false, nil, "", "179"},
			{"waiting for first review", true, nil, "2h", "179"},
			{"other approval", true, map[string]string{"bob": "APPROVED"}, "2h", "108"},
			{"other changes requested", true, map[string]string{"bob": "CHANGES_REQUESTED"}, "2h", "108"},
			{"own approval", true, map[string]string{"alice": "APPROVED"}, "APPROVED", "108"},
			{"own approval without channel request", false, map[string]string{"alice": "APPROVED"}, "APPROVED", "108"},
			{"own changes", true, map[string]string{"alice": "CHANGES_REQUESTED"}, "CHANGES", "174"},
		} {
			t.Run(view.String()+"/"+tc.name, func(t *testing.T) {
				delete(m.mattermost.sent, p.URL)
				if tc.sent {
					m.mattermost.sent[p.URL] = time.Now().Add(-2 * time.Hour)
				}
				p.ReviewSummary = &model.ReviewSummary{Reviewers: tc.reviewers}
				for _, state := range tc.reviewers {
					if state == "APPROVED" {
						p.ReviewSummary.Approved++
					} else {
						p.ReviewSummary.Changes++
					}
				}
				if got := m.reviewLabel(p); got != tc.want {
					t.Fatalf("label %q, want %q", got, tc.want)
				}
				if got := m.reviewColor(p); got != tc.color {
					t.Fatalf("color %q, want %q", got, tc.color)
				}
				for _, width := range []int{60, 100, 160} {
					m.width = width
					for _, selected := range []bool{false, true} {
						row := ansi.Strip(m.row(p, false, selected))
						if tc.want != "" && !strings.Contains(row, tc.want) {
							t.Fatal(row)
						}
						if ansi.StringWidth(row) > width {
							t.Fatal(row)
						}
					}
				}
			})
		}
	}
}

func TestLocalAIStatusAndLatestRelatedSession(t *testing.T) {
	m := unifiedModel(t)
	p := m.prs.Snapshot().Datasets[0].PRs[0]
	at := time.Now().Add(-90 * time.Minute).UTC().Format(time.RFC3339Nano)
	m.copilot.sessions = []model.CopilotSession{
		{ID: "branch", Repository: p.Repository, Branch: p.HeadRef, Status: "active", LastActivity: time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano)},
		{ID: "jira", Jira: "APP-1", Status: "waiting", LastActivity: at},
		{ID: "unrelated", Status: "failed", LastActivity: time.Now().Format(time.RFC3339Nano)},
	}
	if got := m.localAILabel(p); got != "🟡 1h" {
		t.Fatalf("latest related: %q", got)
	}
	for _, tc := range []struct{ status, icon string }{{"active", "🟢"}, {"waiting", "🟡"}, {"completed", "✅"}, {"failed", "🔴"}, {"inactive", "⚪"}, {"unknown", "⚪"}} {
		m.copilot.sessions[1].Status = tc.status
		if got := m.localAILabel(p); got != tc.icon+" 1h" {
			t.Errorf("%s: %q", tc.status, got)
		}
	}
	m.copilot.sessions[1].LastActivity = "invalid"
	if got := m.localAILabel(p); got != "🟢 2h" {
		t.Fatalf("valid timestamp must win: %q", got)
	}
	m.copilot.sessions = m.copilot.sessions[2:]
	if got := m.localAILabel(p); got != "" {
		t.Fatalf("unrelated: %q", got)
	}
	m.entities.data.Links = append(m.entities.data.Links, model.EntityLink{Left: prRef(p), Right: copilotRef(m.copilot.sessions[0])})
	if got := m.localAILabel(p); got != "🔴 nå" {
		t.Fatalf("manual link: %q", got)
	}
	m.copilot.sessions[0].LastActivity = ""
	if got := m.localAILabel(p); got != "🔴" {
		t.Fatalf("missing timestamp: %q", got)
	}
	m.copilot = nil
	if got := m.localAILabel(p); got != "" {
		t.Fatalf("no provider: %q", got)
	}
}

func TestLocalAIAge(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		elapsed time.Duration
		want    string
	}{
		{-time.Minute, "nå"}, {0, "nå"}, {59 * time.Second, "nå"}, {time.Minute, "1m"}, {59 * time.Minute, "59m"}, {time.Hour, "1h"}, {24 * time.Hour, "1d"}, {6 * 24 * time.Hour, "6d"}, {7 * 24 * time.Hour, "1u"}, {14 * 24 * time.Hour, "2u"},
	} {
		if got := localAIAge(now.Add(-tc.elapsed), now); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.elapsed, got, tc.want)
		}
	}
}

func TestGitHubAIColumnPlacementAndWidth(t *testing.T) {
	m := unifiedModel(t)
	p := m.prs.Snapshot().Datasets[0].PRs[0]
	m.copilot.sessions[0].Status = "waiting"
	m.copilot.sessions[0].LastActivity = time.Now().Add(-90 * time.Minute).Format(time.RFC3339Nano)
	for _, width := range []int{60, 100, 119, 120, 140, 160, 200} {
		m.width = width
		header, row := ansi.Strip(m.row(p, true, false)), ansi.Strip(m.row(p, false, false))
		if width >= 120 {
			if !strings.Contains(header, "COPILOT AI") || !strings.Contains(row, "🟡 1h") {
				t.Errorf("width %d: %s\n%s", width, header, row)
			}
		} else if strings.Contains(header, "COPILOT") || strings.Contains(row, "🟡") {
			t.Errorf("narrow width %d: %s", width, row)
		}
		if ansi.StringWidth(header) > width || ansi.StringWidth(row) > width {
			t.Errorf("overflow at %d: %d/%d", width, ansi.StringWidth(header), ansi.StringWidth(row))
		}
	}
}

func TestLocalAIRefreshContinuesOutsideAIView(t *testing.T) {
	m := unifiedModel(t)
	if m.page == pageCopilot {
		t.Fatal("expected GitHub view")
	}
	m.useSource(nil)
	if cmd := m.initProviders(); cmd == nil || !m.copilot.loading {
		t.Fatal("AI data not loaded at startup")
	}
	m.copilot.loading = false
	next, cmd := m.updateCopilot(copilotTick{})
	m = next.(Model)
	if cmd == nil || !m.copilot.loading {
		t.Fatal("background tick did not refresh AI data")
	}
	next, cmd = m.updateCopilot(copilotResult{sessions: m.copilot.sessions})
	m = next.(Model)
	if cmd == nil || !m.copilot.timerPending || m.copilot.loading {
		t.Fatal("background refresh did not schedule next tick")
	}
}
