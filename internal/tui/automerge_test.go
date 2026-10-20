package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type autoMergeSource struct {
	fakeSource
	calls []string
	fail  string
}

func (s *autoMergeSource) SetAutoMerge(_ context.Context, p model.PullRequest, enabled bool) (model.PullRequest, error) {
	s.calls = append(s.calls, p.URL)
	if p.URL == s.fail {
		return p, errors.New("access denied")
	}
	p.AutoMerge = "OFF"
	if enabled {
		p.AutoMerge = "ON"
	}
	p.MergeQueue = "NONE"
	return p, nil
}

func TestAutoMergeSelectionAndStatus(t *testing.T) {
	m := ready(t)
	source := &autoMergeSource{}
	m.source = source
	p := m.filtered()[0]
	m.snapshot.Datasets[model.Reviews].PRs = []model.PullRequest{p}
	m.detail = &detailState{pr: p}
	m, _ = update(m, key('a'))
	m.selected = 1 // The action uses the selection captured at sequence start.
	m, cmd := update(m, key('+'))
	if cmd == nil || !m.autoMergeBusy {
		t.Fatal("action not scheduled")
	}
	_, duplicate := m.startAutoMerge([]model.TaskReference{prRef(p)}, true)
	if duplicate != nil {
		t.Fatal("duplicate write scheduled")
	}
	m, _ = update(m, cmd())
	if len(source.calls) != 1 || source.calls[0] != p.URL || m.autoMergeBusy {
		t.Fatalf("calls=%v", source.calls)
	}
	if m.snapshot.Datasets[model.Open].PRs[0].AutoMerge != "ON" || m.snapshot.Datasets[model.Reviews].PRs[0].AutoMerge != "ON" || m.detail.pr.AutoMerge != "ON" {
		t.Fatal("status not propagated")
	}
	if !strings.Contains(strings.Join(m.detailLines(), "\n"), "Automerge: ON") {
		t.Fatal("status not visible")
	}
	m, _ = update(m, key('a'))
	m, cmd = update(m, key('-'))
	m, _ = update(m, cmd())
	if m.detail.pr.AutoMerge != "OFF" {
		t.Fatal("disable failed")
	}
	// A refresh started before the mutation must not restore the old status.
	m, cmd = update(m, refreshMsg{view: model.Open, result: model.Result{Complete: true, PRs: []model.PullRequest{p}}})
	if cmd == nil || m.snapshot.Datasets[model.Open].PRs[0].AutoMerge != "OFF" {
		t.Fatal("stale refresh applied")
	}
}

func TestAutoMergePartialFailure(t *testing.T) {
	m := ready(t)
	rows := m.filtered()
	source := &autoMergeSource{fail: rows[1].URL}
	m.source = source
	for _, p := range rows {
		m.chosen[p.Key()] = p
	}
	m, _ = update(m, key('a'))
	m, cmd := update(m, key('+'))
	m, _ = update(m, cmd())
	if m.snapshot.Datasets[model.Open].PRs[0].AutoMerge != "ON" || m.snapshot.Datasets[model.Open].PRs[1].AutoMerge == "ON" || !strings.Contains(m.sequenceNotice, "access denied") {
		t.Fatal("partial failure not preserved", m.sequenceNotice)
	}
}
