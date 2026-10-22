package github

import (
	"context"
	"fmt"
	"sort"
	"strings"

	gh "github.com/google/go-github/v93/github"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

// Details reads the current head, follows pagination, and preserves sections when
// another endpoint is unavailable (for example checks with a restricted token).
func (c *Client) Details(ctx context.Context, p model.PullRequest) (model.PRDetails, error) {
	var d model.PRDetails
	parts := strings.Split(p.Repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || p.Number <= 0 {
		return d, fmt.Errorf("invalid PR identity")
	}
	owner, repo := parts[0], parts[1]
	pr, _, err := c.api.PullRequests.Get(ctx, owner, repo, p.Number)
	if err != nil {
		return d, safeError(err)
	}
	warn := func(section string, err error) { d.Warnings = append(d.Warnings, section+": "+safeError(err).Error()) }
	status := p
	status.ID = pr.GetNodeID()
	if status.ID == "" {
		warn("Status", fmt.Errorf("missing PR node ID"))
	} else {
		rows := []model.PullRequest{status}
		if err := c.enrich(ctx, rows); err != nil {
			warn("Status", err)
		} else {
			d.PullRequest = &rows[0]
		}
	}
	for page := 1; page != 0; {
		rows, response, err := c.api.PullRequests.ListFiles(ctx, owner, repo, p.Number, &gh.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			warn("Filer", err)
			break
		}
		for _, f := range rows {
			d.Files = append(d.Files, model.ChangedFile{Path: f.GetFilename(), PreviousPath: f.GetPreviousFilename(), Status: f.GetStatus(), Additions: f.GetAdditions(), Deletions: f.GetDeletions()})
		}
		page = response.NextPage
	}
	sort.Slice(d.Files, func(i, j int) bool { return d.Files[i].Path < d.Files[j].Path })
	for page := 1; page != 0; {
		rows, response, err := c.api.Issues.ListComments(ctx, owner, repo, p.Number, &gh.IssueListCommentsOptions{ListOptions: gh.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			warn("Kommentarer", err)
			break
		}
		for _, v := range rows {
			d.Comments = append(d.Comments, model.PRComment{Author: v.GetUser().GetLogin(), Body: v.GetBody()})
		}
		page = response.NextPage
	}
	for page := 1; page != 0; {
		rows, response, err := c.api.PullRequests.ListComments(ctx, owner, repo, p.Number, &gh.PullRequestListCommentsOptions{ListOptions: gh.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			warn("Kodekommentarer", err)
			break
		}
		for _, v := range rows {
			d.Comments = append(d.Comments, model.PRComment{Author: v.GetUser().GetLogin(), Body: v.GetBody(), Path: v.GetPath(), Line: v.GetLine()})
		}
		page = response.NextPage
	}
	for page := 1; page != 0; {
		rows, response, err := c.api.PullRequests.ListReviews(ctx, owner, repo, p.Number, &gh.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			warn("Reviews", err)
			break
		}
		for _, v := range rows {
			if v.GetBody() != "" {
				d.Comments = append(d.Comments, model.PRComment{Author: v.GetUser().GetLogin() + " (" + v.GetState() + ")", Body: v.GetBody()})
			}
		}
		page = response.NextPage
	}
	for page := 1; page != 0; {
		rows, response, err := c.api.Checks.ListCheckRunsForRef(ctx, owner, repo, pr.GetHead().GetSHA(), &gh.ListCheckRunsOptions{ListOptions: gh.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			warn("Actions", err)
			break
		}
		for _, v := range rows.CheckRuns {
			d.Checks = append(d.Checks, model.ActionCheck{Name: v.GetName(), Status: v.GetStatus(), Conclusion: v.GetConclusion()})
		}
		page = response.NextPage
	}
	return d, nil
}
