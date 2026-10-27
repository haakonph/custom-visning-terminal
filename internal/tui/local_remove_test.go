package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haakonph/custom-visning-terminal/internal/local"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func TestRemoveWorktreeConfirmation(t *testing.T) {
	m := localModel(t)
	p := m.filtered()[0]
	m, cmd := sequence(m, "aw")
	if cmd != nil || len(m.local.removePending) != 1 || m.local.removePending[0].Key() != p.Key() {
		t.Fatal("missing confirmation")
	}
	if !strings.Contains(m.View().Content, "y slett") {
		t.Fatal("confirmation hidden")
	}
	m, cmd = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.local.removing {
		t.Fatal("enter should not delete")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(m.local.removePending) != 0 {
		t.Fatal("cancel failed")
	}
	m, _ = sequence(m, "aw")
	m, cmd = update(m, key('y'))
	if cmd == nil || !m.local.removing || !m.local.preparing[p.Key()] {
		t.Fatal("deletion not queued")
	}
	m, cmd = sequence(m, "ee")
	if cmd != nil {
		t.Fatal("open overlaps removal")
	}
	m.local.statuses[p.Key()] = local.Status{Path: "/old"}
	m, _ = update(m, localRemoveResult{rows: []model.PullRequest{p}, errors: []error{nil}})
	if m.local.removing || m.local.preparing[p.Key()] || m.local.statuses[p.Key()].Path != "" {
		t.Fatal("stale state remains")
	}
}

func TestRemoveSelectionAndFailure(t *testing.T) {
	m := localModel(t)
	p, q := m.filtered()[0], m.filtered()[1]
	m.chosen[p.Key()], m.chosen[q.Key()] = struct{}{}, struct{}{}
	m, _ = sequence(m, "aw")
	if len(m.local.removePending) != 2 {
		t.Fatal("selection ignored")
	}
	m, _ = update(m, key('y'))
	m, _ = update(m, localRemoveResult{rows: []model.PullRequest{p, q}, errors: []error{errors.New("local changes"), nil}})
	if m.local.removing || len(m.local.preparing) != 0 || !strings.Contains(strings.Join(m.local.results, "\n"), "local changes") {
		t.Fatal("partial failure not reported")
	}
}
