package pullrequests

import (
	"context"
	"errors"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"testing"
	"time"
)

type source struct {
	calls    []string
	fail     string
	started  chan struct{}
	canceled chan struct{}
}

func (*source) User(context.Context) (string, error) { return "alice", nil }
func (*source) List(context.Context, model.View, string) (model.Result, error) {
	return model.Result{}, nil
}
func (s *source) SetAutoMerge(ctx context.Context, p model.PullRequest, enabled bool) (model.PullRequest, error) {
	if s.started != nil {
		close(s.started)
		<-ctx.Done()
		close(s.canceled)
		return p, ctx.Err()
	}
	s.calls = append(s.calls, p.Key())
	if p.Key() == s.fail {
		return p, errors.New("denied")
	}
	if enabled {
		p.AutoMerge = "ON"
	} else {
		p.AutoMerge = "OFF"
	}
	return p, nil
}
func (*source) RequestCopilotReview(context.Context, model.PullRequest) error { return nil }
func fixture(t *testing.T, client *source) *State {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := New(ctx, client, nil)
	snap := model.Snapshot{Version: 1, User: "alice"}
	p := model.PullRequest{URL: "https://github.com/o/r/pull/1", Repository: "o/r", Number: 1, Title: "one"}
	snap.Datasets[model.Open].PRs = []model.PullRequest{p, {URL: "https://github.com/o/r/pull/2", Repository: "o/r", Number: 2, Title: "two"}}
	snap.Datasets[model.Reviews].PRs = []model.PullRequest{p}
	s.Restore(snap)
	return s
}
func TestActionDeduplicatesAndUpdatesEveryProjection(t *testing.T) {
	client := &source{}
	s := fixture(t, client)
	p := s.Snapshot().Datasets[0].PRs[0]
	before := s.Export()
	run, err := s.SetAutoMerge(context.Background(), []string{p.Key(), p.Key()}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetAutoMerge(context.Background(), []string{p.Key()}, true); err == nil {
		t.Fatal("duplicate mutation allowed")
	}
	if !s.ApplyAutoMerge(run()) {
		t.Fatal("result rejected")
	}
	if len(client.calls) != 1 {
		t.Fatal(client.calls)
	}
	for _, v := range []model.View{model.Open, model.Reviews} {
		if s.Snapshot().Datasets[v].PRs[0].AutoMerge != "ON" {
			t.Fatal("projection disagrees")
		}
	}
	if before.Datasets[0].PRs[0].AutoMerge == "ON" {
		t.Fatal("export changed during background persistence")
	}
}
func TestPartialActionStopsAndReportsUnattemptedTargets(t *testing.T) {
	client := &source{fail: "https://github.com/o/r/pull/2"}
	s := fixture(t, client)
	ids := []string{"https://github.com/o/r/pull/1", client.fail, "https://github.com/o/r/pull/3"}
	s.Upsert(model.PullRequest{URL: ids[2]})
	run, err := s.SetAutoMerge(context.Background(), ids, true)
	if err != nil {
		t.Fatal(err)
	}
	result := run()
	if !s.ApplyAutoMerge(result) || result.Err == nil || len(client.calls) != 2 || !result.Outcomes[0].Updated || result.Outcomes[1].Err == nil || result.Outcomes[2].Updated || result.Outcomes[2].Err != nil {
		t.Fatalf("%+v", result)
	}
	if p, _ := s.Lookup(ids[0]); p.AutoMerge != "ON" {
		t.Fatal(p)
	}
}
func TestRefreshCannotUndoMutationsInEitherCompletionOrder(t *testing.T) {
	for _, refreshFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "mutation-first", true: "refresh-first"}[refreshFirst], func(t *testing.T) {
			s := fixture(t, &source{})
			old := s.Snapshot().Datasets[0].PRs[0]
			token := s.Begin(ListResource(model.Open))
			run, _ := s.SetAutoMerge(context.Background(), []string{old.Key()}, true)
			apply := func() {
				if s.ApplyList(model.Open, model.Result{PRs: []model.PullRequest{old}, Complete: true}, nil, time.Now(), token) {
					t.Fatal("stale read accepted")
				}
			}
			if refreshFirst {
				apply()
			}
			s.ApplyAutoMerge(run())
			if !refreshFirst {
				apply()
			}
			if p, _ := s.Lookup(old.Key()); p.AutoMerge != "ON" {
				t.Fatal(p)
			}
		})
	}
}
func TestIndependentMutationsCanFinishInAnyOrder(t *testing.T) {
	s := fixture(t, &source{})
	id := s.Snapshot().Datasets[0].PRs[0].Key()
	auto, _ := s.SetAutoMerge(context.Background(), []string{id}, true)
	review, _ := s.RequestReview(context.Background(), []string{id})
	if !s.ApplyReview(review()) || !s.ApplyAutoMerge(auto()) || s.AutoMergeBusy() || s.ReviewBusy() {
		t.Fatal("unrelated completion invalidated an action")
	}
	p, _ := s.Lookup(id)
	if p.AutoMerge != "ON" || !ReviewPending(p) {
		t.Fatal(p)
	}
}
func TestAccountSwitchCancelsWorkAndRejectsOldResults(t *testing.T) {
	client := &source{started: make(chan struct{}), canceled: make(chan struct{})}
	s := fixture(t, client)
	id := s.Snapshot().Datasets[0].PRs[0].Key()
	run, _ := s.SetAutoMerge(context.Background(), []string{id}, true)
	result := make(chan AutoMergeResult, 1)
	go func() { result <- run() }()
	<-client.started
	s.Configure(&source{}, nil)
	select {
	case <-client.canceled:
	case <-time.After(time.Second):
		t.Fatal("old session not canceled")
	}
	if s.ApplyAutoMerge(<-result) || s.AutoMergeBusy() || len(s.Snapshot().Datasets[0].PRs) != 0 {
		t.Fatal("old result affected new account")
	}
}
func TestPartialListsRetainMembershipAndNewerObservationsWin(t *testing.T) {
	s := fixture(t, &source{})
	old := s.Snapshot().Datasets[0].PRs[0]
	earlier := s.Begin(ListResource(model.Open))
	later := s.Begin(ListResource(model.Reviews))
	fresh := old
	fresh.Title = "newest"
	s.ApplyList(model.Reviews, model.Result{PRs: []model.PullRequest{fresh}, Complete: true}, nil, time.Now(), later)
	s.ApplyList(model.Open, model.Result{PRs: []model.PullRequest{old}}, errors.New("page failed"), time.Now(), earlier)
	if p, _ := s.Lookup(old.Key()); p.Title != "newest" {
		t.Fatal(p)
	}
	if len(s.Snapshot().Datasets[0].PRs) != 2 {
		t.Fatal("partial result removed membership")
	}
}
