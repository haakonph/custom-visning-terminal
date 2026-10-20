package github

import (
	"context"
	"errors"
	gh "github.com/google/go-github/v93/github"
	"github.com/haakonph/custom-visning-terminal/internal/model"
	"strings"
)

// RequestCopilotReview also re-requests review after an earlier Copilot review.
// GitHub enforces access, Copilot availability and the PR's current state.
func (c *Client) RequestCopilotReview(ctx context.Context, p model.PullRequest) error {
	parts := strings.Split(p.Repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || p.Number <= 0 {
		return errors.New("Ugyldig repository eller PR-nummer")
	}
	_, _, err := c.api.PullRequests.RequestReviewers(ctx, parts[0], parts[1], p.Number, gh.ReviewersRequest{Reviewers: []string{"copilot-pull-request-reviewer"}})
	if err != nil {
		return safeError(err)
	}
	return nil
}
