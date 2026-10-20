package github

import (
	"context"
	gh "github.com/google/go-github/v93/github"
	"sort"
	"time"
)

// Teams includes memberships across all accessible organizations, not a repo's teams.
func (c *Client) Teams(ctx context.Context) ([]string, error) {
	c.teamsMu.Lock()
	defer c.teamsMu.Unlock()
	if !c.teamsAt.IsZero() && time.Since(c.teamsAt) < 15*time.Minute {
		return append([]string(nil), c.teams...), nil
	}
	seen := map[string]bool{}
	for page := 1; page != 0; {
		teams, response, err := c.api.Teams.ListUserTeams(ctx, &gh.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return teamNames(seen), safeError(err)
		}
		for _, team := range teams {
			if org, slug := team.GetOrganization().GetLogin(), team.GetSlug(); org != "" && slug != "" {
				seen[org+"/"+slug] = true
			}
		}
		next := response.NextPage
		if next != 0 && next <= page {
			break
		}
		page = next
	}
	c.teams = teamNames(seen)
	c.teamsAt = time.Now()
	return append([]string(nil), c.teams...), nil
}
func teamNames(seen map[string]bool) []string {
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
