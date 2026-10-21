package github

import (
	"context"
	"fmt"
	gh "github.com/google/go-github/v93/github"
	"sort"
	"strings"
)

// WritableRepositories lists every page and trusts only explicit push permission.
func (c *Client) WritableRepositories(ctx context.Context) ([]string, error) {
	var result []string
	for page := 1; page != 0; {
		repos, response, err := c.api.Repositories.List(ctx, "", &gh.RepositoryListOptions{ListOptions: gh.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return nil, safeError(err)
		}
		for _, repo := range repos {
			if repo.GetPermissions().GetPush() && !repo.GetArchived() && !repo.GetDisabled() {
				result = append(result, repo.GetFullName())
			}
		}
		page = response.NextPage
	}
	sort.Strings(result)
	return result, nil
}

// CheckWritable revalidates permissions immediately before creating a checkout.
func (c *Client) CheckWritable(ctx context.Context, id string) error {
	owner, name, ok := strings.Cut(id, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("invalid repository identity")
	}
	repo, _, err := c.api.Repositories.Get(ctx, owner, name)
	if err != nil {
		return safeError(err)
	}
	if !repo.GetPermissions().GetPush() || repo.GetArchived() || repo.GetDisabled() {
		return fmt.Errorf("repository har ikke tilgjengelig skrivetilgang")
	}
	return nil
}
