package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestDetailTreeAndStaleResult(t *testing.T) {
	m := New(context.Background(), nil, nil)
	defer m.cancel()
	m.detail = &detailState{request: 2, pr: model.PullRequest{Title: "Example"}, data: model.PRDetails{
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
