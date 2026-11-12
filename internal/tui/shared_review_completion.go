package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	mm "github.com/haakonph/custom-visning-terminal/internal/conversations"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type sharedReviewCompletion struct {
	pr         model.PullRequest
	postID     string
	sharedAt   time.Time
	generation uint64
}

func (m *Model) completeSharedReviews(before []model.PullRequest) tea.Cmd {
	s := m.mmReader
	if s == nil {
		return nil
	}
	if len(before) == 0 {
		return m.reactToCompletedSharedReviews()
	}
	remaining := map[string]bool{}
	for _, pr := range m.githubDataset(model.SharedReviews).PRs {
		remaining[strings.ToLower(pr.URL)] = true
	}
	posts := mattermostReviewPosts(s.data)
	for _, pr := range before {
		url := strings.ToLower(pr.URL)
		if post, ok := posts[url]; ok && post.ID != "" && !remaining[url] {
			s.reviewCompletions = append(s.reviewCompletions, sharedReviewCompletion{
				pr: pr, postID: post.ID, sharedAt: time.UnixMilli(post.CreatedAt), generation: m.prs.Generation(),
			})
		}
	}
	return m.reactToCompletedSharedReviews()
}

func (m *Model) reactToCompletedSharedReviews() tea.Cmd {
	s := m.mmReader
	if s == nil || s.busy || !s.authenticated || s.service == nil || len(s.reviewCompletions) == 0 {
		return nil
	}
	source, ok := m.prs.Source().(model.ReviewCompletionSource)
	if !ok {
		return nil
	}
	remaining := map[string]bool{}
	for _, pr := range m.githubDataset(model.SharedReviews).PRs {
		remaining[strings.ToLower(pr.URL)] = true
	}
	var pending []sharedReviewCompletion
	seen := map[string]bool{}
	for _, request := range s.reviewCompletions {
		key := request.postID + "\x00" + strings.ToLower(request.pr.URL)
		if request.generation == m.prs.Generation() && !remaining[strings.ToLower(request.pr.URL)] && !seen[key] {
			pending = append(pending, request)
			seen[key] = true
		}
	}
	s.reviewCompletions = nil
	if len(pending) == 0 {
		return nil
	}
	s.busy = true
	token := m.chatSession.Begin("operation")
	parent, githubContext := m.chatSession.Context(m.ctx), m.prs.Context()
	user, service := m.prs.Snapshot().User, s.service
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		stop := context.AfterFunc(githubContext, cancel)
		defer stop()
		defer cancel()
		if githubContext.Err() != nil {
			cancel()
		}
		var retries []sharedReviewCompletion
		var failures []error
		reacted := map[string]bool{}
		for _, request := range pending {
			if reacted[request.postID] {
				continue
			}
			completed, err := source.ReviewedSince(ctx, request.pr, user, request.sharedAt)
			if err == nil && completed {
				err = ctx.Err()
				if err == nil {
					err = service.CompleteReview(ctx, request.postID, request.pr.URL)
				}
				if err == nil {
					reacted[request.postID] = true
				}
			}
			if err != nil {
				retries = append(retries, request)
				failures = append(failures, fmt.Errorf("Kunne ikke sette ✅ på reviewmeldingen: %w", err))
			}
		}
		err := errors.Join(failures...)
		authenticated := !errors.Is(err, mm.ErrSession) && !errors.Is(err, mm.ErrAccount)
		return indexedMMResult(mmResult{token: token, data: service.Snapshot(), err: err, authenticated: authenticated, retryReviews: retries})
	}
}
