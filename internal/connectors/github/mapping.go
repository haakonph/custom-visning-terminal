package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	gh "github.com/google/go-github/v93/github"
	"github.com/haakonph/custom-visning-terminal/internal/model"
)

func mapIssue(i *gh.Issue) model.PullRequest {
	repository := strings.TrimPrefix(i.GetRepositoryURL(), "https://api.github.com/repos/")
	p := model.PullRequest{ID: i.GetNodeID(), Repository: repository, Number: i.GetNumber(), Title: i.GetTitle(), Body: i.GetBody(), URL: i.GetHTMLURL(), Author: i.GetUser().GetLogin(), Draft: i.GetDraft(), CreatedAt: i.GetCreatedAt().Time, UpdatedAt: i.GetUpdatedAt().Time, Review: model.Unknown, CI: model.Unknown, Mergeability: model.Unknown}
	for _, label := range i.Labels {
		p.Labels = append(p.Labels, label.GetName())
	}
	return p
}

const detailsQuery = `query($ids:[ID!]!) {
 nodes(ids:$ids) { ... on PullRequest {
  id body headRefName headRefOid isDraft reviewDecision mergeable
  reviewRequests(first:100) {
   pageInfo { hasNextPage }
   nodes { requestedReviewer { ... on User { login } ... on Team { slug organization { login } } } }
  }
  commits(last:1) { nodes { commit { statusCheckRollup { state } } } }
 } }
}`

type details struct {
	HeadOID        string `json:"headRefOid"`
	Body           string `json:"body"`
	HeadRef        string `json:"headRefName"`
	ID             string `json:"id"`
	IsDraft        bool   `json:"isDraft"`
	ReviewDecision string `json:"reviewDecision"`
	Mergeable      string `json:"mergeable"`
	ReviewRequests struct {
		PageInfo struct {
			HasNextPage bool `json:"hasNextPage"`
		} `json:"pageInfo"`
		Nodes []struct {
			RequestedReviewer struct {
				Login        string `json:"login"`
				Slug         string `json:"slug"`
				Organization struct {
					Login string `json:"login"`
				} `json:"organization"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

func (c *Client) enrich(ctx context.Context, rows []model.PullRequest) error {
	var failures []error
	for start := 0; start < len(rows); start += 40 {
		end := min(start+40, len(rows))
		ids := make([]string, 0, end-start)
		for _, p := range rows[start:end] {
			ids = append(ids, p.ID)
		}
		req, err := c.api.NewRequest(ctx, "POST", "graphql", map[string]any{"query": detailsQuery, "variables": map[string]any{"ids": ids}})
		if err != nil {
			return errors.New("could not create GitHub details request")
		}
		var response struct {
			Data struct {
				Nodes []*details `json:"nodes"`
			} `json:"data"`
			Errors []json.RawMessage `json:"errors"`
		}
		_, err = c.api.Do(req, &response)
		if err != nil {
			failures = append(failures, safeError(err))
			continue
		}
		if len(response.Errors) > 0 {
			failures = append(failures, errors.New("GitHub GraphQL returned partial data; check repository and organization access"))
		}
		mapped := map[string]*details{}
		for _, d := range response.Data.Nodes {
			if d != nil {
				mapped[d.ID] = d
			}
		}
		for i := start; i < end; i++ {
			d := mapped[rows[i].ID]
			if d == nil {
				failures = append(failures, missingNode(rows[i].ID))
				continue
			}
			applyDetails(&rows[i], d)
			if d.ReviewRequests.PageInfo.HasNextPage {
				failures = append(failures, errors.New("reviewer list truncated at 100 requests"))
			}
		}
	}
	return errors.Join(failures...)
}
func applyDetails(p *model.PullRequest, d *details) {
	p.Body = d.Body
	p.HeadRef = d.HeadRef
	p.HeadOID = d.HeadOID
	p.Draft = d.IsDraft
	switch d.ReviewDecision {
	case "APPROVED":
		p.Review = model.Approved
	case "CHANGES_REQUESTED":
		p.Review = model.ChangesRequested
	case "REVIEW_REQUIRED":
		p.Review = model.Pending
	}
	switch d.Mergeable {
	case "MERGEABLE":
		p.Mergeability = model.Mergeable
	case "CONFLICTING":
		p.Mergeability = model.Conflicting
	}
	// Keep team matches discovered by search if the reviewer connection is truncated.
	teams := p.RequestedTeams
	p.RequestedReviewers = nil
	p.RequestedTeams = nil
	for _, node := range d.ReviewRequests.Nodes {
		r := node.RequestedReviewer
		if r.Login != "" {
			p.RequestedReviewers = append(p.RequestedReviewers, r.Login)
		}
		if r.Slug != "" && r.Organization.Login != "" {
			p.RequestedTeams = append(p.RequestedTeams, r.Organization.Login+"/"+r.Slug)
		}
	}
	if d.ReviewRequests.PageInfo.HasNextPage {
		for _, team := range teams {
			if !contains(p.RequestedTeams, team) {
				p.RequestedTeams = append(p.RequestedTeams, team)
			}
		}
	}
	if len(d.Commits.Nodes) > 0 && d.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		switch d.Commits.Nodes[0].Commit.StatusCheckRollup.State {
		case "SUCCESS":
			p.CI = model.Pass
		case "FAILURE", "ERROR":
			p.CI = model.Fail
		case "PENDING", "EXPECTED":
			p.CI = model.Pending
		}
	}
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
