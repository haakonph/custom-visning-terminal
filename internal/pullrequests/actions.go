package pullrequests

import (
	"context"
	"fmt"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/syncstate"
	"time"
)

type Outcome struct {
	ID      string
	Updated bool
	Err     error
}
type AutoMergeResult struct {
	Token    syncstate.Token
	Updated  []model.PullRequest
	Outcomes []Outcome
	Err      error
}

func (s *State) SetAutoMerge(parent context.Context, ids []string, enabled bool) (func() AutoMergeResult, error) {
	client, ok := s.source.(model.AutoMergeSource)
	if !ok {
		return nil, fmt.Errorf("automerge støttes ikke")
	}
	if s.autoMergeBusy {
		return nil, fmt.Errorf("Automerge oppdateres allerede…")
	}
	sessionContext := s.ctx
	rows, err := s.Resolve(ids)
	if err != nil {
		return nil, err
	}
	s.autoMergeBusy = true
	s.session.Mutate()
	token := s.Begin("automerge")
	return func() AutoMergeResult {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		stop := context.AfterFunc(sessionContext, cancel)
		if sessionContext.Err() != nil {
			cancel()
		}
		defer stop()
		defer cancel()
		result := AutoMergeResult{Token: token, Outcomes: make([]Outcome, len(rows))}
		for i, p := range rows {
			result.Outcomes[i].ID = p.Key()
		}
		for i, p := range rows {
			if err := ctx.Err(); err != nil {
				result.Err = err
				result.Outcomes[i].Err = err
				break
			}
			updated, err := client.SetAutoMerge(ctx, p, enabled)
			if err != nil {
				result.Outcomes[i].Err = err
				result.Err = fmt.Errorf("%s #%d: %w (%d PR-er oppdatert; resten stoppet)", p.Repository, p.Number, err, len(result.Updated))
				break
			}
			result.Updated = append(result.Updated, updated)
			result.Outcomes[i].Updated = true
		}
		return result
	}, nil
}
func (s *State) ApplyAutoMerge(result AutoMergeResult) bool {
	if !s.autoMergeBusy || !s.session.Match("automerge", result.Token) {
		return false
	}
	s.autoMergeBusy = false
	s.session.Mutate()
	for _, p := range result.Updated {
		if row, ok := s.rows[p.Key()]; ok {
			row.AutoMerge, row.MergeQueue, row.QueuePosition = p.AutoMerge, p.MergeQueue, p.QueuePosition
			s.rows[p.Key()] = row
		}
	}
	s.project()
	return true
}

type ReviewResult struct {
	Token syncstate.Token
	URLs  []string
	Err   error
}

func (s *State) RequestReview(parent context.Context, ids []string) (func() ReviewResult, error) {
	client, ok := s.source.(model.CopilotReviewSource)
	if !ok {
		return nil, fmt.Errorf("Copilot-review støttes ikke")
	}
	if s.reviewBusy {
		return nil, fmt.Errorf("Copilot-review forespørres allerede…")
	}
	sessionContext := s.ctx
	rows, err := s.Resolve(ids)
	if err != nil {
		return nil, err
	}
	s.reviewBusy = true
	// Action identity is independent of read revision: other mutations may finish
	// while this one runs. Generation + resource sequence identify its completion.
	token := s.Begin("review")
	return func() ReviewResult {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		stop := context.AfterFunc(sessionContext, cancel)
		if sessionContext.Err() != nil {
			cancel()
		}
		defer stop()
		defer cancel()
		result := ReviewResult{Token: token}
		for _, p := range rows {
			if err := ctx.Err(); err != nil {
				result.Err = err
				break
			}
			if err := client.RequestCopilotReview(ctx, p); err != nil {
				result.Err = fmt.Errorf("%s #%d: %w (%d forespørsler sendt; resten stoppet)", p.Repository, p.Number, err, len(result.URLs))
				break
			}
			result.URLs = append(result.URLs, p.URL)
		}
		return result
	}, nil
}
func (s *State) ApplyReview(result ReviewResult) bool {
	if !s.session.Match("review", result.Token) || !s.reviewBusy {
		return false
	}
	s.reviewBusy = false
	s.session.Mutate()
	for _, id := range result.URLs {
		if p, ok := s.rows[id]; ok && !ReviewPending(p) {
			p = clone(p)
			p.RequestedReviewers = append(p.RequestedReviewers, "copilot-pull-request-reviewer[bot]")
			s.rows[id] = p
		}
	}
	s.project()
	return true
}
