package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestDetailTreeAndStaleResult(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.prs.Upsert(model.PullRequest{Title: "Example"})
	m.detail = &detailState{request: 2, id: "", data: model.PRDetails{
		Files:    []model.ChangedFile{{Path: "src/app/a.go", Status: "modified"}, {Path: "src/app/b.go", Status: "added"}},
		Comments: []model.PRComment{{Author: "alice", Body: "line one\nline two"}},
		Checks:   []model.ActionCheck{{Name: "test", Status: "completed", Conclusion: "success"}},
	}}
	lines := strings.Join(m.detailLines(), "\n")
	for _, want := range []string{"src/", "  app/", "    a.go", "line one\nline two", "test: completed · success"} {
		if !strings.Contains(lines, want) {
			t.Errorf("missing %q in %s", want, lines)
		}
	}
	if strings.Count(lines, "  app/") != 1 {
		t.Fatal("directory repeated")
	}
	updated, _ := m.Update(detailMsg{request: 1, data: model.PRDetails{Warnings: []string{"stale"}}})
	if len(updated.(Model).detail.data.Warnings) != 0 {
		t.Fatal("stale response replaced current details")
	}
	m.width = 1
	m.height = 1
	_ = m.detailView()
}

// Exercise both providers through the shared Update/View dispatch after merging.
func TestDetailsFromMattermostPRRoundTrip(t *testing.T) {
	m := readerModel(t)
	m, _ = sequence(m, "rg")
	m, _ = update(m, key('d'))
	if m.detail == nil || m.detailPR().Number != 1 || !strings.Contains(m.View().Content, "Endrede filer") {
		t.Fatal("linked PR did not open GitHub details")
	}
	data := m.mmReader.data
	m, _ = update(m, mmResult{data: data, authenticated: true})
	if m.detail == nil || (m.page == pageChat) {
		t.Fatal("Mattermost background refresh displaced details")
	}
	m, _ = update(m, key('M'))
	if m.page == pageChat {
		t.Fatal("detail view leaked navigation keys")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = sequence(m, "rc")
	if m.detail != nil || m.dialog == nil || len(m.dialog.rows) != 2 {
		t.Fatal("could not return from GitHub details to Mattermost references")
	}
}

func TestDetailRefreshCompletesCopilotReview(t *testing.T) {
	m := ready(t)
	p := m.filtered()[0]
	p.RequestedReviewers = []string{"copilot-pull-request-reviewer[bot]"}
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[model.Open].PRs = []model.PullRequest{p} })
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[model.Reviews].PRs = []model.PullRequest{p} })
	m.prs.Upsert(p)
	m.detail = &detailState{request: 2, id: p.Key()}
	fresh := p
	fresh.RequestedReviewers = nil
	fresh.CopilotReview = &model.CopilotReview{ID: "completed", Recommendation: "Approval", Comments: 2}
	m, _ = update(m, detailMsg{request: 1, data: model.PRDetails{PullRequest: &fresh}})
	if !copilotReviewPending(m.detailPR()) {
		t.Fatal("stale response changed status")
	}
	m, _ = update(m, detailMsg{request: 2, data: model.PRDetails{PullRequest: &fresh}})
	for _, pr := range []model.PullRequest{m.detailPR(), m.prs.Snapshot().Datasets[model.Open].PRs[0], m.prs.Snapshot().Datasets[model.Reviews].PRs[0]} {
		if got := copilotReviewLabel(pr); got != "🟢 2" {
			t.Fatalf("completed review still shows %q", got)
		}
	}
	m, _ = update(m, detailMsg{request: 2, data: model.PRDetails{Warnings: []string{"Status: offline"}}})
	if got := copilotReviewLabel(m.detailPR()); got != "🟢 2" {
		t.Fatalf("status lost after failure: %q", got)
	}
}

func TestDetailStatusDoesNotUndoAutoMerge(t *testing.T) {
	m := ready(t)
	p := m.filtered()[0]
	p.AutoMerge = "ON"
	m.prs.InvalidateReads()
	m.prs.Upsert(p)
	m.detail = &detailState{request: 2, id: p.Key()}
	stale := p
	stale.AutoMerge = "OFF"
	m, _ = update(m, detailMsg{request: 2, data: model.PRDetails{PullRequest: &stale}})
	if m.detailPR().AutoMerge != "ON" {
		t.Fatal("stale detail status undid automerge")
	}
}
