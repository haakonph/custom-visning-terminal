package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type checkRollup struct {
	State    string           `json:"state"`
	Contexts *checkConnection `json:"contexts"`
}

type checkConnection struct {
	Nodes    []checkContext `json:"nodes"`
	PageInfo checkPageInfo  `json:"pageInfo"`
}

type checkPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type checkContext struct {
	Name       string `json:"name"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
	IsRequired *bool  `json:"isRequired"`
}

type requiredChecks struct {
	BranchProtectionRule *struct {
		Contexts []string `json:"requiredStatusCheckContexts"`
	} `json:"branchProtectionRule"`
	Rules *struct {
		Nodes []struct {
			Parameters *struct {
				Checks []struct {
					Context string `json:"context"`
				} `json:"requiredStatusChecks"`
			} `json:"parameters"`
		} `json:"nodes"`
		PageInfo checkPageInfo `json:"pageInfo"`
	} `json:"rules"`
}

func (r *requiredChecks) contexts() []string {
	var contexts []string
	if r == nil {
		return contexts
	}
	if r.BranchProtectionRule != nil {
		contexts = append(contexts, r.BranchProtectionRule.Contexts...)
	}
	if r.Rules != nil {
		for _, rule := range r.Rules.Nodes {
			if rule.Parameters != nil {
				for _, check := range rule.Parameters.Checks {
					contexts = append(contexts, check.Context)
				}
			}
		}
	}
	return contexts
}

func requiredChecksFields(cursor string) string {
	after := ""
	if cursor != "" {
		quoted, _ := json.Marshal(cursor)
		after = ",after:" + string(quoted)
	}
	return `baseRef {
 branchProtectionRule { requiredStatusCheckContexts }
 rules(first:100` + after + `) {
  pageInfo { hasNextPage endCursor }
  nodes { parameters { ... on RequiredStatusChecksParameters { requiredStatusChecks { context } } } }
 }
}`
}

func ciAlias(i int) string { return fmt.Sprintf("ci%d", i) }

// isRequired is relative to a pull request, so each alias passes its own ID.
// JSON string quoting also safely quotes GraphQL IDs and pagination cursors.
func ciFields(rows []model.PullRequest, cursors map[int]string) string {
	var query strings.Builder
	for i, p := range rows {
		id, _ := json.Marshal(p.ID)
		after := ""
		if cursor := cursors[i]; cursor != "" {
			quoted, _ := json.Marshal(cursor)
			after = ",after:" + string(quoted)
		}
		fmt.Fprintf(&query, `%s:node(id:%s) { ... on PullRequest {
 %s
 commits(last:1) { nodes { commit { statusCheckRollup { state
  contexts(first:100%s) {
   pageInfo { hasNextPage endCursor }
   nodes {
    ... on CheckRun { name status conclusion isRequired(pullRequestId:%s) }
    ... on StatusContext { context state isRequired(pullRequestId:%s) }
   }
  }
 } } } }
} }
`, ciAlias(i), id, requiredChecksFields(""), after, id, id)
	}
	return query.String()
}

func (c *Client) completeChecks(ctx context.Context, p model.PullRequest, d *details) error {
	if len(d.Commits.Nodes) == 0 || d.Commits.Nodes[0].Commit.StatusCheckRollup == nil {
		return c.completeRequiredChecks(ctx, p, d)
	}
	rollup := d.Commits.Nodes[0].Commit.StatusCheckRollup
	seen := map[string]bool{}
	for rollup.Contexts != nil && rollup.Contexts.PageInfo.HasNextPage {
		cursor := rollup.Contexts.PageInfo.EndCursor
		if cursor == "" || seen[cursor] {
			return errors.New("GitHub checks pagination incomplete")
		}
		seen[cursor] = true
		query := "query {" + ciFields([]model.PullRequest{p}, map[int]string{0: cursor}) + "}"
		req, err := c.api.NewRequest(ctx, "POST", "graphql", map[string]any{"query": query})
		if err != nil {
			return errors.New("could not create GitHub checks request")
		}
		var response struct {
			Data   map[string]*details `json:"data"`
			Errors []json.RawMessage   `json:"errors"`
		}
		if _, err := c.api.Do(req, &response); err != nil {
			return safeError(err)
		}
		next := response.Data[ciAlias(0)]
		if len(response.Errors) > 0 || next == nil || len(next.Commits.Nodes) == 0 {
			return errors.New("GitHub checks pagination incomplete")
		}
		page := next.Commits.Nodes[0].Commit.StatusCheckRollup
		if page == nil || page.Contexts == nil {
			return errors.New("GitHub checks pagination incomplete")
		}
		rollup.Contexts.Nodes = append(rollup.Contexts.Nodes, page.Contexts.Nodes...)
		rollup.Contexts.PageInfo = page.Contexts.PageInfo
	}
	return c.completeRequiredChecks(ctx, p, d)
}

func (c *Client) completeRequiredChecks(ctx context.Context, p model.PullRequest, d *details) error {
	seen := map[string]bool{}
	for d.RequiredChecks != nil && d.RequiredChecks.Rules != nil && d.RequiredChecks.Rules.PageInfo.HasNextPage {
		cursor := d.RequiredChecks.Rules.PageInfo.EndCursor
		if cursor == "" || seen[cursor] {
			return errors.New("GitHub required checks pagination incomplete")
		}
		seen[cursor] = true
		id, _ := json.Marshal(p.ID)
		query := fmt.Sprintf("query { %s:node(id:%s) { ... on PullRequest { %s } } }", ciAlias(0), id, requiredChecksFields(cursor))
		req, err := c.api.NewRequest(ctx, "POST", "graphql", map[string]any{"query": query})
		if err != nil {
			return errors.New("could not create GitHub required checks request")
		}
		var response struct {
			Data   map[string]*details `json:"data"`
			Errors []json.RawMessage   `json:"errors"`
		}
		if _, err := c.api.Do(req, &response); err != nil {
			return safeError(err)
		}
		next := response.Data[ciAlias(0)]
		if len(response.Errors) > 0 || next == nil || next.RequiredChecks == nil || next.RequiredChecks.Rules == nil {
			return errors.New("GitHub required checks pagination incomplete")
		}
		d.RequiredChecks.Rules.Nodes = append(d.RequiredChecks.Rules.Nodes, next.RequiredChecks.Rules.Nodes...)
		d.RequiredChecks.Rules.PageInfo = next.RequiredChecks.Rules.PageInfo
	}
	return nil
}

func checkStatus(rollup *checkRollup, required ...string) model.Status {
	absent := make(map[string]bool, len(required))
	for _, name := range required {
		if name != "" {
			absent[name] = true
		}
	}
	if rollup == nil {
		if len(absent) > 0 {
			return model.Missing
		}
		return model.Unknown
	}
	// Older cached/partial responses may only have the aggregate state.
	if rollup.Contexts == nil {
		switch rollup.State {
		case "SUCCESS":
			return model.Pass
		case "FAILURE", "ERROR":
			return model.Fail
		case "PENDING":
			return model.Pending
		case "EXPECTED":
			return model.Missing
		default:
			return model.Unknown
		}
	}
	failed, missing, warning, texts, pending, unknown := false, false, false, false, false, false
	for _, check := range rollup.Contexts.Nodes {
		required := absent[check.Name] || absent[check.Context] || (check.IsRequired != nil && *check.IsRequired)
		delete(absent, check.Name)
		delete(absent, check.Context)
		state := check.State
		if state == "" {
			if check.Status != "COMPLETED" {
				switch check.Status {
				case "IN_PROGRESS":
					pending = true
				case "QUEUED", "WAITING", "REQUESTED", "PENDING":
					if required {
						missing = true
					} else {
						pending = true
					}
				default:
					unknown = true
				}
				continue
			}
			state = check.Conclusion
		}
		switch state {
		case "SUCCESS", "NEUTRAL":
		case "SKIPPED":
			if required {
				missing = true
			}
		case "PENDING":
			pending = true
		case "EXPECTED":
			if required {
				missing = true
			} else {
				pending = true
			}
		case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
			if strings.Contains(strings.ToLower(check.Name+" "+check.Context), "tekst-sync") {
				texts = true
			} else if required {
				failed = true
			} else if check.IsRequired == nil {
				unknown = true
			} else {
				warning = true
			}
		default:
			unknown = true
		}
	}
	switch {
	case failed:
		return model.Fail
	case unknown || rollup.Contexts.PageInfo.HasNextPage:
		return model.Unknown
	case missing || len(absent) > 0:
		return model.Missing
	case pending:
		return model.Pending
	case texts:
		return model.Texts
	case warning:
		return model.Warning
	case len(rollup.Contexts.Nodes) == 0:
		return model.Unknown
	default:
		return model.Pass
	}
}
