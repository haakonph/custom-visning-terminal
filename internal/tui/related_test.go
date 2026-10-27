package tui

import (
	"context"
	"regexp"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestRelatedPRsDeduplicatesAcrossViews(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	p := model.PullRequest{URL: "https://github.com/a/b/pull/1", Title: "ABC-12"}
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[model.Open].PRs = []model.PullRequest{p, {URL: "https://github.com/a/b/pull/2", Title: "ABC-123"}}
	})
	m.editPRs(func(snapshot *model.Snapshot) { snapshot.Datasets[model.Reviews].PRs = []model.PullRequest{p} })
	rows := m.relatedPRs("ABC-12")
	if len(rows) != 1 || rows[0].URL != p.URL {
		t.Fatalf("unexpected related PRs: %v", rows)
	}
}

func TestRelatedWithoutJira(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	next, cmd := m.navigateRelated()
	if cmd != nil || (next.(Model).page == pageJira) || next.(Model).notice == "" {
		t.Fatal("expected configuration notice")
	}
}

func TestRelatedPRsUsesCustomPattern(t *testing.T) {
	m := New(context.Background(), nil, nil).WithJiraPattern(regexp.MustCompile(`KAKE-[0-9]{3,5}`))
	defer m.cancel()
	m.editPRs(func(snapshot *model.Snapshot) {
		snapshot.Datasets[model.Open].PRs = []model.PullRequest{
			{URL: "https://github.com/a/b/pull/1", Title: "KAKE-123"},
			{URL: "https://github.com/a/b/pull/2", Title: "KAKE-12"},
		}
	})
	if got := m.relatedPRs("KAKE-123"); len(got) != 1 {
		t.Fatalf("expected one related PR, got %v", got)
	}
	if got := m.relatedPRs("KAKE-12"); len(got) != 0 {
		t.Fatalf("matched key outside pattern: %v", got)
	}
}
