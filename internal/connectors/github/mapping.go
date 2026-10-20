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
  latestOpinionatedReviews(first:100) {
   pageInfo { hasNextPage }
   nodes { state author { login } }
  }
  copilotReviews: reviews(last:100) {
   pageInfo { hasPreviousPage startCursor }
   nodes { author { login __typename } body state comments { totalCount } }
  }
  reviewThreads(first:100) { pageInfo { hasNextPage endCursor } nodes { isResolved comments(first:1) { nodes { author { __typename } } } } }
  autoMergeRequest { enabledAt }
  mergeQueueEntry { position state }
  reviewRequests(first:100) {
   pageInfo { hasNextPage }
   nodes { requestedReviewer { ... on User { login } ... on Team { slug organization { login } } } }
  }
  commits(last:1) { nodes { commit { statusCheckRollup { state } } } }
 } }
}`

type details struct {
	CopilotReviews           copilotReviews      `json:"copilotReviews"`
	ReviewThreads            *reviewThreads      `json:"reviewThreads"`
	AutoMergeRequest         json.RawMessage     `json:"autoMergeRequest"`
	MergeQueueEntry          json.RawMessage     `json:"mergeQueueEntry"`
	HeadOID                  string              `json:"headRefOid"`
	Body                     string              `json:"body"`
	HeadRef                  string              `json:"headRefName"`
	ID                       string              `json:"id"`
	IsDraft                  bool                `json:"isDraft"`
	ReviewDecision           string              `json:"reviewDecision"`
	Mergeable                string              `json:"mergeable"`
	LatestOpinionatedReviews *opinionatedReviews `json:"latestOpinionatedReviews"`
	ReviewRequests           *struct {
		PageInfo *struct {
			HasNextPage bool `json:"hasNextPage"`
		} `json:"pageInfo"`
		Nodes []*struct {
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
				StatusCheckRollup *checkRollup `json:"statusCheckRollup"`
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
		query := strings.TrimSuffix(detailsQuery, "}") + ciFields(rows[start:end], nil) + "}"
		req, err := c.api.NewRequest(ctx, "POST", "graphql", map[string]any{"query": query, "variables": map[string]any{"ids": ids}})
		if err != nil {
			return errors.New("could not create GitHub details request")
		}
		var response struct {
			Data   map[string]json.RawMessage `json:"data"`
			Errors []json.RawMessage          `json:"errors"`
		}
		_, err = c.api.Do(req, &response)
		if err != nil {
			failures = append(failures, safeError(err))
			continue
		}
		if len(response.Errors) > 0 {
			failures = append(failures, errors.New("GitHub GraphQL returned partial data; check repository and organization access"))
		}
		var nodes []*details
		if err := json.Unmarshal(response.Data["nodes"], &nodes); err != nil {
			failures = append(failures, errors.New("could not read GitHub details"))
			continue
		}
		mapped := map[string]*details{}
		for _, d := range nodes {
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
			// The per-PR alias supplies isRequired with the correct PR ID even
			// when several pull requests share a commit.
			if raw := response.Data[ciAlias(i-start)]; len(raw) > 0 {
				var checks details
				if json.Unmarshal(raw, &checks) != nil || string(raw) == "null" {
					rows[i].CI = model.Unknown
					failures = append(failures, errors.New("could not read GitHub checks"))
					d.Commits.Nodes = nil
				} else {
					d.Commits = checks.Commits
					if err := c.completeChecks(ctx, rows[i], d); err != nil {
						failures = append(failures, err)
						rows[i].CI = model.Unknown
						d.Commits.Nodes = nil
					}
				}
			}
			if err := c.completeCopilotReviews(ctx, rows[i].ID, d); err != nil {
				failures = append(failures, err)
			}
			applyDetails(&rows[i], d)
			if err := c.completeOpenComments(ctx, &rows[i], d.ReviewThreads); err != nil {
				failures = append(failures, err)
			}
			if rows[i].ReviewSummary != nil && !rows[i].ReviewSummary.Complete {
				failures = append(failures, errors.New("review summary incomplete; at most 100 reviewers and requests are included"))
			}
			if d.ReviewRequests != nil && d.ReviewRequests.PageInfo != nil && d.ReviewRequests.PageInfo.HasNextPage {
				failures = append(failures, errors.New("reviewer list truncated at 100 requests"))
			}
		}
	}
	return errors.Join(failures...)
}
func applyDetails(p *model.PullRequest, d *details) {
	p.AutoMerge, p.MergeQueue, p.QueuePosition = model.Unknown, model.Unknown, 0
	if len(d.AutoMergeRequest) > 0 {
		p.AutoMerge = "ON"
		if string(d.AutoMergeRequest) == "null" {
			p.AutoMerge = "OFF"
		}
	}
	if len(d.MergeQueueEntry) > 0 {
		if string(d.MergeQueueEntry) == "null" {
			p.MergeQueue = "NONE"
		} else {
			var entry struct {
				Position int    `json:"position"`
				State    string `json:"state"`
			}
			if json.Unmarshal(d.MergeQueueEntry, &entry) == nil {
				p.MergeQueue = model.Status(entry.State)
				if p.MergeQueue == "" {
					p.MergeQueue = "QUEUED"
				}
				p.QueuePosition = entry.Position
			}
		}
	}
	p.Body = d.Body
	p.HeadRef = d.HeadRef
	p.HeadOID = d.HeadOID
	p.Draft = d.IsDraft

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
	if d.ReviewRequests != nil {
		for _, node := range d.ReviewRequests.Nodes {
			if node == nil {
				continue
			}
			r := node.RequestedReviewer
			if r.Login != "" {
				p.RequestedReviewers = append(p.RequestedReviewers, r.Login)
			}
			if r.Slug != "" && r.Organization.Login != "" {
				p.RequestedTeams = append(p.RequestedTeams, r.Organization.Login+"/"+r.Slug)
			}
		}
	}
	if d.ReviewRequests == nil || d.ReviewRequests.PageInfo == nil || d.ReviewRequests.PageInfo.HasNextPage {
		for _, team := range teams {
			if !contains(p.RequestedTeams, team) {
				p.RequestedTeams = append(p.RequestedTeams, team)
			}
		}
	}
	applyReview(p, d)
	applyCopilotReview(p, d)
	if len(d.Commits.Nodes) > 0 && d.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		p.CI = checkStatus(d.Commits.Nodes[0].Commit.StatusCheckRollup)
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
