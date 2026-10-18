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

// Exercise both providers through the shared Update/View dispatch after merging.
func TestDetailsFromMattermostPRRoundTrip(t *testing.T) {
	m := readerModel(t)
	m, _ = update(m, key('g'))
	m, _ = update(m, key('d'))
	if m.detail == nil || m.detail.pr.Number != 1 || !strings.Contains(m.View().Content, "Endrede filer") {
		t.Fatal("linked PR did not open GitHub details")
	}
	data := m.mmReader.data
	m, _ = update(m, mmResult{data: data, authenticated: true})
	if m.detail == nil || m.mmReader.active {
		t.Fatal("Mattermost background refresh displaced details")
	}
	m, _ = update(m, key('M'))
	if m.mmReader.active {
		t.Fatal("detail view leaked navigation keys")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = update(m, key('v'))
	if m.detail != nil || !m.mmReader.active || !m.mmReader.mentions {
		t.Fatal("could not return from GitHub details to Mattermost references")
	}
}
