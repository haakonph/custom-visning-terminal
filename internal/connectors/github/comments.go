package github

import (
	"context"
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
				count++
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
		err := c.mergeGraphQL(ctx, `query($id:ID!,$after:String!){node(id:$id){... on PullRequest{reviewThreads(first:100,after:$after){pageInfo{hasNextPage endCursor} nodes{isResolved}}}}}`, map[string]any{"id": p.ID, "after": cursor}, &data)
		if err != nil {
			return err
		}
		if data.Node == nil || data.Node.ReviewThreads == nil {
			return errors.New("missing GitHub review threads")
		}
		page = data.Node.ReviewThreads
	}
}
