package github

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

type reviewThreads struct {
	PageInfo *struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []*struct {
		IsResolved *bool `json:"isResolved"`
		Comments   *struct {
			Nodes []*struct {
				Author *struct {
					Type string `json:"__typename"`
				} `json:"author"`
			} `json:"nodes"`
		} `json:"comments"`
	} `json:"nodes"`
}

// Count threads rather than replies: resolving a conversation closes all its comments.
func (c *Client) completeOpenComments(ctx context.Context, p *model.PullRequest, page *reviewThreads) error {
	p.OpenComments = nil
	if page == nil {
		return nil
	}
	count := 0
	seen := map[string]bool{}
	for {
		if page.PageInfo == nil || page.Nodes == nil {
			return errors.New("incomplete GitHub review threads")
		}
		for _, thread := range page.Nodes {
			if thread == nil || thread.IsResolved == nil {
				return errors.New("incomplete GitHub review thread")
			}
			if !*thread.IsResolved {
				if thread.Comments == nil || len(thread.Comments.Nodes) == 0 || thread.Comments.Nodes[0] == nil {
					return errors.New("missing GitHub review thread author")
				}
				author := thread.Comments.Nodes[0].Author
				// Deleted users still have conversations requiring a response; bot threads
				// are covered by the separate Copilot review column.
				if author == nil || author.Type != "Bot" {
					count++
				}
			}
		}
		if !page.PageInfo.HasNextPage {
			p.OpenComments = &count
			return nil
		}
		cursor := page.PageInfo.EndCursor
		if cursor == "" || seen[cursor] {
			return errors.New("invalid GitHub review thread cursor")
		}
		seen[cursor] = true
		var data struct {
			Node *struct {
				ReviewThreads *reviewThreads `json:"reviewThreads"`
			} `json:"node"`
		}
		err := c.reviewThreadsGraphQL(ctx, `query($id:ID!,$after:String!){node(id:$id){... on PullRequest{reviewThreads(first:100,after:$after){pageInfo{hasNextPage endCursor} nodes{isResolved comments(first:1){nodes{author{__typename}}}}}}}}`, map[string]any{"id": p.ID, "after": cursor}, &data)
		if err != nil {
			return err
		}
		if data.Node == nil || data.Node.ReviewThreads == nil {
			return errors.New("missing GitHub review threads")
		}
		page = data.Node.ReviewThreads
	}
}

func (c *Client) reviewThreadsGraphQL(ctx context.Context, query string, variables map[string]any, data any) error {
	req, err := c.api.NewRequest(ctx, "POST", "graphql", map[string]any{"query": query, "variables": variables})
	if err != nil {
		return errors.New("could not create GitHub review thread request")
	}
	var response struct {
		Data   json.RawMessage
		Errors []json.RawMessage
	}
	if _, err := c.api.Do(req, &response); err != nil {
		return safeError(err)
	}
	if len(response.Errors) > 0 {
		return errors.New("GitHub returned incomplete review threads")
	}
	if len(response.Data) == 0 || string(response.Data) == "null" || json.Unmarshal(response.Data, data) != nil {
		return errors.New("could not read GitHub review threads")
	}
	return nil
}
