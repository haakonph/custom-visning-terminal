package tui

import (
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/connectors/jira"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestJiraPRAggregatePriority(t *testing.T) {
	ready := model.PullRequest{Review: model.Approved, CI: model.Pass, Mergeability: model.Mergeable}
	tests := []struct {
		name string
		pr   model.PullRequest
		want string
	}{
		{"ready", ready, "Klar til merge"},
		{"conflict", model.PullRequest{Mergeability: model.Conflicting}, "Blokkert"},
		{"failed draft", model.PullRequest{Draft: true, CI: model.Fail}, "Blokkert"},
		{"changes", model.PullRequest{Review: model.ChangesRequested}, "Endringer ønsket"},
		{"review", model.PullRequest{Review: model.NoReview}, "Venter på review"},
		{"draft", model.PullRequest{Draft: true, Review: model.NoReview}, "Under arbeid"},
		{"checks", model.PullRequest{Review: model.Approved, CI: model.Pending}, "Sjekker pågår"},
		{"unknown", model.PullRequest{Review: model.Approved}, "Ukjent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := jiraModel(t)
			defer m.cancel()
			ready.URL = "https://github.example/ready"
			ready.Title = "APP-1"
			tt.pr.URL = "https://github.example/other"
			tt.pr.HeadRef = "feature/APP-1"
			m.snapshot.Datasets[model.Open].PRs = []model.PullRequest{ready, tt.pr}
			m.snapshot.Datasets[model.Reviews].PRs = []model.PullRequest{tt.pr}
			got := m.jiraPRSummary("APP-1")
			if got.count != 2 || got.countLabel() != "2" || got.status != tt.want {
				t.Fatalf("got %+v", got)
			}
			if !strings.Contains(got.explanation, "GitHub-visningene") {
				t.Fatal(got.explanation)
			}
		})
	}
}

func TestJiraPRSummaryFollowsSelectionAndFits(t *testing.T) {
	m := jiraModel(t)
	defer m.cancel()
	m.source = &fakeSource{}
	m.jira.rows = []jira.Issue{{Key: "APP-1", Title: "First"}, {Key: "APP-2", Title: "Second"}}
	m.snapshot.Datasets[model.Open].PRs = []model.PullRequest{{URL: "https://github.example/1", Title: "APP-1", CI: model.Fail}}
	for _, width := range []int{40, 80, 120} {
		m.width = width
		m.height = 24
		m.jira.selected = 0
		content := m.jiraView().Content
		if strings.Index(content, "TITTEL") > strings.Index(content, " PR ") {
			t.Fatalf("PR column should follow title at width %d: %s", width, content)
		}
		if !strings.Contains(content, "Blokkert") || !strings.Contains(content, "feilede sjekker") {
			t.Fatalf("width %d: %s", width, content)
		}
		if got := m.jiraPRSummary("APP-2"); got.countLabel() != "" || got.status != "" || got.detail() != "" {
			t.Fatalf("empty PR summary should be blank: %+v", got)
		}
		m.jira.selected = 1
		content = m.jiraView().Content
		if strings.Contains(content, "Ingen åpne PR-er") || strings.Contains(content, "PR-er:  åpne") {
			t.Fatal(content)
		}
		issue := m.jira.rows[0]
		m.jira.detail = &issue
		if !strings.Contains(strings.Join(m.jiraDetailLines(), " "), "Blokkert") {
			t.Fatal("missing detail summary")
		}
	}
	m.source = nil
	if got := m.jiraPRSummary("APP-9"); got.status != "Ukjent" || got.countLabel() != "?" {
		t.Fatal(got)
	}
}
