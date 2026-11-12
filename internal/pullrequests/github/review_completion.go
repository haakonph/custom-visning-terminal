package github

import (
	"context"
	"errors"
	"strings"
	"time"

	gh "github.com/google/go-github/v93/github"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func (c *Client) LatestReviewSince(ctx context.Context, p model.PullRequest, user string, since time.Time) (model.ReviewOutcome, error) {
	owner, repo, ok := strings.Cut(p.Repository, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") || p.Number <= 0 || user == "" || since.IsZero() {
		return "", errors.New("invalid PR review identity")
	}
	var latest *gh.PullRequestReview
	for page := 1; ; {
		reviews, response, err := c.api.PullRequests.ListReviews(ctx, owner, repo, p.Number, &gh.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return "", safeError(err)
		}
		for _, review := range reviews {
			if review == nil || !strings.EqualFold(review.GetUser().GetLogin(), user) || review.SubmittedAt == nil || review.SubmittedAt.Before(since) || review.GetState() == "PENDING" {
				continue
			}
			if latest == nil || review.SubmittedAt.After(latest.SubmittedAt.Time) || review.SubmittedAt.Equal(*latest.SubmittedAt) && review.GetID() > latest.GetID() {
				latest = review
			}
		}
		if response.NextPage == 0 {
			break
		}
		if response.NextPage <= page {
			return "", errors.New("GitHub review pagination incomplete")
		}
		page = response.NextPage
	}
	// Later comments, change requests or dismissals must never reuse an older
	// approval. Only the latest submitted review determines the reaction.
	switch latest.GetState() {
	case "APPROVED":
		return model.ReviewApproval, nil
	case "COMMENTED":
		return model.ReviewComment, nil
	default:
		return "", nil
	}
}
