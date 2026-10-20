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
`, ciAlias(i), id, after, id, id)
	}
	return query.String()
}

func (c *Client) completeChecks(ctx context.Context, p model.PullRequest, d *details) error {
	if len(d.Commits.Nodes) == 0 || d.Commits.Nodes[0].Commit.StatusCheckRollup == nil {
		return nil
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
	return nil
}

func checkStatus(rollup *checkRollup) model.Status {
	// Older cached/partial responses may only have the aggregate state.
	if rollup.Contexts == nil {
		switch rollup.State {
		case "SUCCESS":
			return model.Pass
		case "FAILURE", "ERROR":
			return model.Fail
		case "PENDING", "EXPECTED":
			return model.Pending
		default:
			return model.Unknown
		}
	}
	failed, warning, texts, pending, unknown := false, false, false, false, false
	for _, check := range rollup.Contexts.Nodes {
		state := check.State
		if state == "" {
			if check.Status != "COMPLETED" {
				pending = true
				continue
			}
			state = check.Conclusion
		}
		switch state {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
		case "PENDING", "EXPECTED":
			pending = true
		case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
			if strings.Contains(strings.ToLower(check.Name+" "+check.Context), "tekst-sync") {
				texts = true
			} else if check.IsRequired == nil {
				unknown = true
			} else if *check.IsRequired {
				failed = true
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
	case warning:
		return model.Warning
	case texts:
		return model.Texts
	case pending:
		return model.Pending
	case len(rollup.Contexts.Nodes) == 0:
		return model.Unknown
	default:
		return model.Pass
	}
}
