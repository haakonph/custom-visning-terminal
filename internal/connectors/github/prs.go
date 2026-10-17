package github

import (
	"context"
	"errors"
	"fmt"
	"slices"

	gh "github.com/google/go-github/v93/github"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func query(view model.View, user string) string {
	switch view {
	case model.Open:
		return "is:open is:pr author:" + user + " draft:false"
	case model.Drafts:
		return "is:open is:pr author:" + user + " draft:true"
	case model.Reviews:
		return "is:open is:pr user-review-requested:" + user
	}
	return ""
}
func (c *Client) List(ctx context.Context, view model.View, user string) (model.Result, error) {
	result := model.Result{Complete: true, PRs: []model.PullRequest{}}
	queries := []string{query(view, user)}
	var teams []string
	if view == model.TeamReviews {
		var err error
		teams, err = c.Teams(ctx)
		if err != nil {
			result.Complete = false
			result.Warnings = append(result.Warnings, "Team membership incomplete: "+err.Error())
		}
		queries = nil
		for _, team := range teams {
			queries = append(queries, "is:open is:pr team-review-requested:"+team)
		}
	} else if view < model.Open || view >= model.ViewCount {
		return result, errors.New("invalid PR view")
	}
	found := map[string]model.PullRequest{}
	var failures []error
	searchLimited := false
	for qi, q := range queries {
		rows, complete, err := c.search(ctx, q)
		if !complete {
			result.Complete = false
			if err == nil {
				searchLimited = true
			}
		}
		if err != nil {
			failures = append(failures, err)
		}
		for _, p := range rows {
			if prior, ok := found[p.Key()]; ok {
				p.RequestedTeams = prior.RequestedTeams
			}
			if view == model.TeamReviews && !slices.Contains(p.RequestedTeams, teams[qi]) {
				p.RequestedTeams = append(p.RequestedTeams, teams[qi])
			}
			found[p.Key()] = p
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
	}
	for _, p := range found {
		result.PRs = append(result.PRs, p)
	}
	model.Sort(result.PRs)
	if searchLimited {
		result.Warnings = append(result.Warnings, "Partial discovery: GitHub search is limited to 1,000 results per query or team, and may return incomplete results")
	}
	if len(result.PRs) > 0 {
		if err := c.enrich(ctx, result.PRs); err != nil {
			result.Complete = false
			result.Warnings = append(result.Warnings, "PR details incomplete: "+err.Error())
		}
	}
	return result, errors.Join(failures...)
}
func (c *Client) search(ctx context.Context, q string) ([]model.PullRequest, bool, error) {
	rows := []model.PullRequest{}
	complete := true
	// GitHub's Search API exposes at most 1,000 hits, regardless of total_count.
	for page := 1; page <= 10; page++ {
		hits, response, err := c.api.Search.Issues(ctx, q, &gh.SearchOptions{Sort: "updated", Order: "desc", ListOptions: gh.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return rows, false, safeError(err)
		}
		if hits.GetIncompleteResults() || hits.GetTotal() > 1000 {
			complete = false
		}
		for _, issue := range hits.Issues {
			if issue.IsPullRequest() {
				rows = append(rows, mapIssue(issue))
			}
		}
		if response.NextPage == 0 || len(hits.Issues) == 0 {
			break
		}
		if page == 10 {
			complete = false
		}
	}
	return rows, complete, nil
}
func missingNode(id string) error { return fmt.Errorf("GitHub could not read PR node %s", id) }
