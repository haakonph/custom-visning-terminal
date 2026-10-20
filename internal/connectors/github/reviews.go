package github

import (
	"context"
	"errors"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type opinionatedReviews struct {
	PageInfo *struct {
		HasNextPage bool `json:"hasNextPage"`
	} `json:"pageInfo"`
	Nodes []*struct {
		State  string `json:"state"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
	} `json:"nodes"`
}

func applyReview(p *model.PullRequest, d *details) {
	p.Review, p.ReviewSummary = model.Unknown, nil
	if reviews := d.LatestOpinionatedReviews; reviews != nil {
		summary := &model.ReviewSummary{Fallback: d.ReviewDecision == ""}
		summary.Complete = reviews.PageInfo != nil && !reviews.PageInfo.HasNextPage && reviews.Nodes != nil
		opinions := map[string]string{}
		// GitHub returns the last opinionated review per author. Comments do not
		// replace an opinion, and dismissed reviews no longer count.
		for _, review := range reviews.Nodes {
			if review == nil {
				summary.Complete = false
				continue
			}
			if review.State == "COMMENTED" {
				continue
			}
			if review.Author == nil || review.Author.Login == "" {
				summary.Complete = false
				continue
			}
			author := strings.ToLower(review.Author.Login)
			switch review.State {
			case "APPROVED", "CHANGES_REQUESTED":
				opinions[author] = review.State
			case "DISMISSED":
				delete(opinions, author)
			default:
				summary.Complete = false
			}
		}
		for _, state := range opinions {
			if state == "APPROVED" {
				summary.Approved++
			} else {
				summary.Changes++
			}
		}
		users, teams := map[string]bool{}, map[string]bool{}
		requests := d.ReviewRequests
		if requests == nil || requests.PageInfo == nil || requests.PageInfo.HasNextPage || requests.Nodes == nil {
			summary.Complete = false
		}
		if requests != nil {
			for _, node := range requests.Nodes {
				if node == nil {
					summary.Complete = false
					continue
				}
				r := node.RequestedReviewer
				if r.Login != "" {
					users[strings.ToLower(r.Login)] = true
				} else if r.Slug != "" && r.Organization.Login != "" {
					teams[strings.ToLower(r.Organization.Login+"/"+r.Slug)] = true
				} else {
					summary.Complete = false
				}
			}
		}
		summary.Pending, summary.PendingTeams = len(users), len(teams)
		p.ReviewSummary = summary
		if summary.Complete && summary.Fallback {
			switch {
			case summary.Changes > 0:
				p.Review = model.ChangesRequested
			case summary.Approved > 0:
				p.Review = model.Approved
			case summary.Pending > 0 || summary.PendingTeams > 0:
				p.Review = model.Pending
			default:
				p.Review = model.NoReview
			}
		}
	}
	// Branch-protection decisions remain authoritative when supplied by GitHub.
	switch d.ReviewDecision {
	case "APPROVED":
		p.Review = model.Approved
	case "CHANGES_REQUESTED":
		p.Review = model.ChangesRequested
	case "REVIEW_REQUIRED":
		p.Review = model.Pending
	}
}

// Copilot submits COMMENTED reviews, so its recommendation must come from the
// summary body rather than GitHub's branch-protection review decision.
func applyCopilotReview(p *model.PullRequest, d *details) {
	p.CopilotReview = nil
	r := latestCopilotReview(d.CopilotReviews)
	if r == nil {
		return
	}
	recommendation := "Comments"
	if r.State == "DISMISSED" {
		recommendation = "Dismissed"
	} else {
		for _, line := range strings.Split(strings.ToLower(r.Body), "\n") {
			switch {
			case strings.Contains(line, "approval recommended"):
				recommendation = "Approval"
			case strings.Contains(line, "changes recommended"), strings.Contains(line, "changes requested"):
				recommendation = "Changes"
			case strings.Contains(line, "review recommended"), strings.Contains(line, "further review recommended"):
				recommendation = "Review"
			default:
				continue
			}
			break
		}
	}
	p.CopilotReview = &model.CopilotReview{Recommendation: recommendation, Comments: r.Comments.TotalCount}
}

type copilotReview struct {
	Author *struct {
		Login string `json:"login"`
		Type  string `json:"__typename"`
	} `json:"author"`
	Body     string `json:"body"`
	State    string `json:"state"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
}
type copilotReviews struct {
	PageInfo struct {
		HasPreviousPage bool   `json:"hasPreviousPage"`
		StartCursor     string `json:"startCursor"`
	} `json:"pageInfo"`
	Nodes []*copilotReview `json:"nodes"`
}

func latestCopilotReview(reviews copilotReviews) *copilotReview {
	for i := len(reviews.Nodes) - 1; i >= 0; i-- {
		r := reviews.Nodes[i]
		if r == nil || r.Author == nil || r.Author.Type != "Bot" {
			continue
		}
		login := strings.ToLower(strings.TrimSuffix(r.Author.Login, "[bot]"))
		if login == "copilot" || login == "copilot-pull-request-reviewer" {
			return r
		}
	}
	return nil
}

// Read backwards until the latest Copilot review is found, including when more
// than 100 human reviews were submitted after it.
func (c *Client) completeCopilotReviews(ctx context.Context, id string, d *details) error {
	seen := map[string]bool{}
	for latestCopilotReview(d.CopilotReviews) == nil && d.CopilotReviews.PageInfo.HasPreviousPage {
		cursor := d.CopilotReviews.PageInfo.StartCursor
		if cursor == "" || seen[cursor] {
			return errors.New("Copilot review pagination incomplete")
		}
		seen[cursor] = true
		req, err := c.api.NewRequest(ctx, "POST", "graphql", map[string]any{
			"query":     `query($id:ID!,$before:String!) { node(id:$id) { ... on PullRequest { copilotReviews: reviews(last:100,before:$before) { pageInfo { hasPreviousPage startCursor } nodes { author { login __typename } body state comments { totalCount } } } } } }`,
			"variables": map[string]any{"id": id, "before": cursor},
		})
		if err != nil {
			return errors.New("could not create Copilot review request")
		}
		var response struct {
			Data struct {
				Node *details `json:"node"`
			} `json:"data"`
			Errors []any `json:"errors"`
		}
		if _, err := c.api.Do(req, &response); err != nil {
			return safeError(err)
		}
		if len(response.Errors) > 0 || response.Data.Node == nil {
			return errors.New("could not read Copilot reviews")
		}
		d.CopilotReviews = response.Data.Node.CopilotReviews
	}
	return nil
}
