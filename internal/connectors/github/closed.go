package github

import (
	"context"
	"errors"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

// Closed verifies the PR directly: disappearing from a search or review queue
// does not mean that it was closed. Merged PRs also have state "closed".
func (c *Client) Closed(ctx context.Context, p model.PullRequest) (bool, error) {
	owner, repo, ok := strings.Cut(p.Repository, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") || p.Number <= 0 {
		return false, errors.New("invalid PR identity")
	}
	pr, _, err := c.api.PullRequests.Get(ctx, owner, repo, p.Number)
	if err != nil {
		return false, safeError(err)
	}
	switch pr.GetState() {
	case "closed":
		return true, nil
	case "open":
		return false, nil
	default:
		return false, errors.New("GitHub did not return a PR state")
	}
}
