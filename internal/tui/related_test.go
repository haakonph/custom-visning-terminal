package tui

import (
	"context"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestRelatedPRsDeduplicatesAcrossViews(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	p := model.PullRequest{URL: "https://github.com/a/b/pull/1", Title: "ABC-12"}
	m.snapshot.Datasets[model.Open].PRs = []model.PullRequest{p, {URL: "https://github.com/a/b/pull/2", Title: "ABC-123"}}
	m.snapshot.Datasets[model.Reviews].PRs = []model.PullRequest{p}
	rows := m.relatedPRs("ABC-12")
	if len(rows) != 1 || rows[0].URL != p.URL { t.Fatalf("unexpected related PRs: %v", rows) }
}

func TestRelatedWithoutJira(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	next, cmd := m.navigateRelated()
	if cmd != nil || next.(Model).jiraActive || next.(Model).notice == "" { t.Fatal("expected configuration notice") }
}
