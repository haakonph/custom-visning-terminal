package tui

import (
	"context"
	"errors"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"strings"
	"testing"
)

type reviewRequestSource struct {
	fakeSource
	calls []string
	fail  string
}

func (s *reviewRequestSource) RequestCopilotReview(_ context.Context, p model.PullRequest) error {
	s.calls = append(s.calls, p.URL)
	if p.URL == s.fail {
		return errors.New("access denied")
	}
	return nil
}
func TestCopilotReviewSelectionAndDuplicate(t *testing.T) {
	m := ready(t)
	source := &reviewRequestSource{}
	m.source = source
	p := m.filtered()[0]
	m.detail = &detailState{pr: p}
	m, _ = update(m, key('a'))
	m.selected = 1
	m, cmd := update(m, key('r'))
	if cmd == nil || !m.reviewRequestBusy {
		t.Fatal("request not scheduled")
	}
	_, duplicate := m.startCopilotReview([]model.TaskReference{prRef(p)})
	if duplicate != nil {
		t.Fatal("duplicate request scheduled")
	}
	m, _ = update(m, cmd())
	if !copilotReviewPending(m.detail.pr) {
		t.Fatal("detail should show requested review immediately")
	}
	for _, dataset := range m.snapshot.Datasets {
		for _, pr := range dataset.PRs {
			if pr.URL == p.URL && !copilotReviewPending(pr) {
				t.Fatal("requested review missing from snapshot")
			}
		}
	}
	if len(source.calls) != 1 || source.calls[0] != p.URL || m.reviewRequestBusy || !strings.Contains(m.sequenceNotice, "1 PR-er") {
		t.Fatalf("calls=%v notice=%s", source.calls, m.sequenceNotice)
	}
}
func TestCopilotReviewPartialFailure(t *testing.T) {
	m := ready(t)
	rows := m.filtered()
	source := &reviewRequestSource{fail: rows[1].URL}
	m.source = source
	refs := []model.TaskReference{prRef(rows[0]), prRef(rows[0]), prRef(rows[1])}
	next, cmd := m.startCopilotReview(refs)
	m = next.(Model)
	m, _ = update(m, cmd())
	if len(source.calls) != 2 || m.reviewRequestBusy || !strings.Contains(m.sequenceNotice, "1 forespørsler sendt") {
		t.Fatal(source.calls, m.sequenceNotice)
	}
	for _, dataset := range m.snapshot.Datasets {
		for _, pr := range dataset.PRs {
			if pr.URL == rows[0].URL && !copilotReviewPending(pr) {
				t.Fatal("successful request should show pending")
			}
			if pr.URL == rows[1].URL && copilotReviewPending(pr) {
				t.Fatal("failed request should not show pending")
			}
		}
	}
	if _, cmd = m.startCopilotReview([]model.TaskReference{{Provider: "jira"}}); cmd != nil {
		t.Fatal("wrong provider allowed")
	}
}
