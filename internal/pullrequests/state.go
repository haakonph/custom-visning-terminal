// Package pullrequests owns pull request identity, list membership and actions.
// The owner serializes state changes; background work receives immutable inputs.
package pullrequests

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/syncstate"
)

type Cache interface {
	Load(string) (model.Snapshot, error)
	Save(model.Snapshot) error
}

type State struct {
	parent                    context.Context
	ctx                       context.Context
	cancel                    context.CancelFunc
	session                   syncstate.Session
	source                    model.Source
	cache                     Cache
	rows                      map[string]model.PullRequest
	observed                  map[string]uint64
	members                   [model.ViewCount][]string
	synced                    [model.ViewCount]time.Time
	snapshot                  model.Snapshot // immutable read projection, rebuilt after a domain change
	autoMergeBusy, reviewBusy bool
}

func New(parent context.Context, source model.Source, cache Cache) *State {
	s := &State{parent: parent}
	s.Configure(source, cache)
	return s
}
func (s *State) Configure(source model.Source, cache Cache) {
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx, s.cancel = context.WithCancel(s.parent)
	s.session.Reset()
	s.source, s.cache = source, cache
	s.rows, s.observed = map[string]model.PullRequest{}, map[string]uint64{}
	s.members = [model.ViewCount][]string{}
	s.synced = [model.ViewCount]time.Time{}
	s.snapshot = model.Snapshot{Version: 1}
	s.autoMergeBusy, s.reviewBusy = false, false
}
func (s *State) Source() model.Source                  { return s.source }
func (s *State) Cache() Cache                          { return s.cache }
func (s *State) Generation() uint64                    { return s.session.Generation() }
func (s *State) Revision() uint64                      { return s.session.Revision() }
func (s *State) AutoMergeBusy() bool                   { return s.autoMergeBusy }
func (s *State) ReviewBusy() bool                      { return s.reviewBusy }
func (s *State) Begin(resource string) syncstate.Token { return s.session.Begin(resource) }
func (s *State) Accept(resource string, token syncstate.Token) bool {
	return s.session.Accept(resource, token)
}
func (s *State) Current(token syncstate.Token) bool { return s.session.Current(token) }

// Snapshot and Lookup return read-only projections. Copy with Export before I/O
// or handing data to code that may mutate slices or nested values.
func (s *State) Snapshot() model.Snapshot                   { return s.snapshot }
func (s *State) Lookup(id string) (model.PullRequest, bool) { p, ok := s.rows[id]; return p, ok }
func (s *State) Export() model.Snapshot {
	out := s.snapshot
	for v, d := range out.Datasets {
		out.Datasets[v].PRs = make([]model.PullRequest, len(d.PRs))
		for i, p := range d.PRs {
			out.Datasets[v].PRs[i] = clone(p)
		}
	}
	return out
}
func (s *State) Restore(snapshot model.Snapshot) {
	s.rows, s.observed = map[string]model.PullRequest{}, map[string]uint64{}
	s.snapshot = model.Snapshot{Version: 1, User: snapshot.User}
	for v, d := range snapshot.Datasets {
		s.members[v] = nil
		s.synced[v] = d.SyncedAt
		seen := map[string]bool{}
		for _, p := range d.PRs {
			id := p.Key()
			if old, ok := s.rows[id]; !ok || p.UpdatedAt.After(old.UpdatedAt) {
				s.rows[id] = clone(p)
			}
			if !seen[id] {
				s.members[v] = append(s.members[v], id)
				seen[id] = true
			}
		}
	}
	s.project()
}
func (s *State) Upsert(p model.PullRequest) { s.rows[p.Key()] = clone(p); s.project() }
func ListResource(view model.View) string   { return fmt.Sprintf("list:%d", view) }
func (s *State) ApplyList(view model.View, result model.Result, err error, at time.Time, token syncstate.Token) bool {
	if !s.Accept(ListResource(view), token) {
		return false
	}
	ids := append([]string(nil), s.members[view]...)
	if err == nil && result.Complete {
		ids = nil
		s.synced[view] = at
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, p := range result.PRs {
		id := p.Key()
		if token.Sequence >= s.observed[id] {
			s.rows[id] = clone(p)
			s.observed[id] = token.Sequence
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	s.members[view] = ids
	s.sortMemberships()
	s.project()
	return true
}
func (s *State) project() {
	snap := model.Snapshot{Version: 1, User: s.snapshot.User}
	for v, ids := range s.members {
		snap.Datasets[v].SyncedAt = s.synced[v]
		for _, id := range ids {
			if p, ok := s.rows[id]; ok {
				snap.Datasets[v].PRs = append(snap.Datasets[v].PRs, p)
			}
		}
	}
	s.snapshot = snap
}
func (s *State) Resolve(ids []string) ([]model.PullRequest, error) {
	seen := map[string]bool{}
	rows := make([]model.PullRequest, 0, len(ids))
	for _, id := range ids {
		p, ok := s.rows[id]
		if !ok {
			return nil, fmt.Errorf("ukjent PR: %s", id)
		}
		if !seen[id] {
			rows = append(rows, clone(p))
			seen[id] = true
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("velg minst én PR")
	}
	return rows, nil
}
func clone(p model.PullRequest) model.PullRequest {
	p.Labels = append([]string(nil), p.Labels...)
	p.RequestedReviewers = append([]string(nil), p.RequestedReviewers...)
	p.RequestedTeams = append([]string(nil), p.RequestedTeams...)
	if p.OpenComments != nil {
		v := *p.OpenComments
		p.OpenComments = &v
	}
	if p.CopilotReview != nil {
		v := *p.CopilotReview
		p.CopilotReview = &v
	}
	if p.ReviewSummary != nil {
		v := *p.ReviewSummary
		v.Reviewers = map[string]string{}
		for k, r := range p.ReviewSummary.Reviewers {
			v.Reviewers[k] = r
		}
		p.ReviewSummary = &v
	}
	return p
}
func ReviewPending(p model.PullRequest) bool {
	for _, r := range p.RequestedReviewers {
		if strings.EqualFold(r, "copilot-pull-request-reviewer[bot]") || strings.EqualFold(r, "copilot") {
			return true
		}
	}
	return false
}

func (s *State) SetCache(cache Cache) { s.cache = cache }
func (s *State) InvalidateReads()     { s.session.Mutate() }

func (s *State) Context() context.Context { return s.ctx }
func (s *State) Observe(p model.PullRequest, token syncstate.Token) bool {
	if !s.Current(token) || s.Revision() != token.Revision || token.Sequence < s.observed[p.Key()] {
		return false
	}
	s.rows[p.Key()] = clone(p)
	s.observed[p.Key()] = token.Sequence
	s.sortMemberships()
	s.project()
	return true
}

func (s *State) sortMemberships() {
	for v, ids := range s.members {
		ids = append([]string(nil), ids...)
		sort.SliceStable(ids, func(i, j int) bool {
			a, b := s.rows[ids[i]], s.rows[ids[j]]
			if a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.Key() < b.Key()
			}
			return a.UpdatedAt.After(b.UpdatedAt)
		})
		s.members[v] = ids
	}
}

func (s *State) Match(resource string, token syncstate.Token) bool {
	return s.session.Match(resource, token)
}
