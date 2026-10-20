package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

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
		if width >= 100 && (!strings.Contains(header, "AUTO") || !strings.Contains(header, "INFO") || strings.Contains(header, "QUEUE") || !strings.Contains(row, "ON") || !strings.Contains(row, "#2 QUEUED")) {
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
			m.detail = &detailState{pr: p}
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
	m.snapshot.User = "alice"
	m.active = model.TeamReviews
	m.snapshot.Datasets[model.TeamReviews].PRs = []model.PullRequest{{Author: "Alice", URL: "https://github.com/acme/repo/pull/1"}, {Author: "bob", URL: "https://github.com/acme/repo/pull/2"}}
	if rows := m.filtered(); len(rows) != 1 || rows[0].Author != "bob" {
		t.Fatal(rows)
	}
	m.active = model.Open
	m.snapshot.Datasets[model.Open].PRs = m.snapshot.Datasets[model.TeamReviews].PRs
	if len(m.filtered()) != 2 {
		t.Fatal("own PR hidden outside Team Review")
	}
}

func TestReviewSummaryRendering(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.width = 180
	p := model.PullRequest{Title: "Stacked PR", Review: model.ChangesRequested, ReviewSummary: &model.ReviewSummary{Approved: 2, Changes: 1, Pending: 1, PendingTeams: 1, Complete: true, Fallback: true}}
	m.detail = &detailState{pr: p}
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
	if got := copilotReviewLabel(model.PullRequest{}); got != "—" {
		t.Fatal(got)
	}
	p := model.PullRequest{CopilotReview: &model.CopilotReview{Recommendation: "Approval", Comments: 3}}
	if got := copilotReviewLabel(p); got != "🟢 Approval · 3" {
		t.Fatal(got)
	}
	if got := reviewSummaryLabel(p); !strings.Contains(got, "Copilot: 🟢 Approval · 3") {
		t.Fatal(got)
	}
}

func TestOpenCommentsInfo(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	for _, tc := range []struct {
		count  int
		status model.Status
		want   string
	}{
		{0, model.Mergeable, "0 åpne"},
		{3, model.Mergeable, "3 åpne"},
		{12, model.Conflicting, "CONFLICT · 12 åpne"},
	} {
		p := model.PullRequest{OpenComments: &tc.count, Mergeability: tc.status, MergeQueue: "NONE"}
		if got := prInfoLabel(p); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
		for _, width := range []int{100, 120, 140, 160, 200} {
			m.width = width
			row := ansi.Strip(m.row(p, false, false))
			if !strings.Contains(row, tc.want) || ansi.StringWidth(row) > width {
				t.Fatalf("width %d: %q", width, row)
			}
		}
	}
}
