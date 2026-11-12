package github

import (
	"context"
	"errors"
	"strings"
	"time"

	gh "github.com/google/go-github/v93/github"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func (c *Client) ReviewedSince(ctx context.Context, p model.PullRequest, user string, since time.Time) (bool, error) {
	owner, repo, ok := strings.Cut(p.Repository, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") || p.Number <= 0 || user == "" || since.IsZero() {
		return false, errors.New("invalid PR review identity")
	}
	for page := 1; ; {
		reviews, response, err := c.api.PullRequests.ListReviews(ctx, owner, repo, p.Number, &gh.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return false, safeError(err)
		}
		for _, review := range reviews {
			if review == nil || !strings.EqualFold(review.GetUser().GetLogin(), user) || review.SubmittedAt == nil || review.SubmittedAt.Before(since) {
				continue
			}
			switch review.GetState() {
			case "APPROVED", "CHANGES_REQUESTED", "COMMENTED":
				return true, nil
			}
		}
		if response.NextPage == 0 {
			return false, nil
		}
		if response.NextPage <= page {
			return false, errors.New("GitHub review pagination incomplete")
		}
		page = response.NextPage
	}
}
