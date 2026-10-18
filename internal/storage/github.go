package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
)

func timestamp(t time.Time) string          { return t.Format(time.RFC3339Nano) }
func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }
func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
func prKey(p model.PullRequest) string {
	if p.URL != "" {
		return p.URL
	}
	if p.Repository != "" || p.Number != 0 {
		return fmt.Sprintf("%s#%d", p.Repository, p.Number)
	}
	return p.ID
}
func writeGitHub(ctx context.Context, q *dbgen.Queries, s model.Snapshot) error {
	if s.Version != 1 || s.User == "" {
		return errors.New("unsupported cache version or account")
	}
	if err := q.ClearAccounts(ctx); err != nil {
		return err
	}
	if err := q.InsertAccounts(ctx, dbgen.InsertAccountsParams{ID: 1, Provider: "github", Username: s.User}); err != nil {
		return err
	}
	prs := map[string]model.PullRequest{}
	observed := map[string]time.Time{}
	for _, d := range s.Datasets {
		for _, p := range d.PRs {
			key := prKey(p)
			old, ok := prs[key]
			if !ok || p.UpdatedAt.After(old.UpdatedAt) || (p.UpdatedAt.Equal(old.UpdatedAt) && !observed[key].After(d.SyncedAt)) {
				prs[key] = p
				observed[key] = d.SyncedAt
			}
		}
	}
	repos := map[string]int64{}
	for key, p := range prs {
		repoID, ok := repos[p.Repository]
		if !ok {
			repoID = int64(len(repos) + 1)
			repos[p.Repository] = repoID
			if err := q.InsertGithubRepositories(ctx, dbgen.InsertGithubRepositoriesParams{ID: repoID, AccountID: 1, Name: p.Repository}); err != nil {
				return err
			}
		}
		row := dbgen.InsertGithubPullRequestsParams{ID: key, RepositoryID: repoID, Number: int64(p.Number), ProviderID: p.ID, Body: p.Body, HeadOid: p.HeadOID, HeadRef: p.HeadRef, Title: p.Title, Url: p.URL, Author: p.Author, CreatedAt: timestamp(p.CreatedAt), UpdatedAt: timestamp(p.UpdatedAt), Review: string(p.Review), Ci: string(p.CI), Mergeability: string(p.Mergeability), Draft: boolInt(p.Draft)}
		if err := q.InsertGithubPullRequests(ctx, row); err != nil {
			return err
		}
		for pos, label := range p.Labels {
			if err := q.InsertGithubLabels(ctx, dbgen.InsertGithubLabelsParams{PullRequestID: key, Position: int64(pos), Label: label}); err != nil {
				return err
			}
		}
		for _, group := range []struct {
			kind  string
			names []string
		}{{"user", p.RequestedReviewers}, {"team", p.RequestedTeams}} {
			for pos, name := range group.names {
				if err := q.InsertGithubPullRequestReviewers(ctx, dbgen.InsertGithubPullRequestReviewersParams{PullRequestID: key, Kind: group.kind, Position: int64(pos), Name: name}); err != nil {
					return err
				}
			}
		}
	}
	for view, d := range s.Datasets {
		if err := q.InsertSyncState(ctx, dbgen.InsertSyncStateParams{ViewID: int64(view), AccountID: 1, SyncedAt: timestamp(d.SyncedAt)}); err != nil {
			return err
		}
		for pos, p := range d.PRs {
			if err := q.InsertGithubViewMemberships(ctx, dbgen.InsertGithubViewMembershipsParams{ViewID: int64(view), Position: int64(pos), PullRequestID: prKey(p)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func readGitHub(ctx context.Context, q *dbgen.Queries) (model.Snapshot, error) {
	out := model.Snapshot{Version: 1}
	accounts, err := q.ListAccounts(ctx)
	if err != nil {
		return out, err
	}
	if len(accounts) != 1 {
		return out, errors.New("invalid GitHub account data")
	}
	out.User = accounts[0].Username
	repos, err := q.ListGithubRepositories(ctx)
	if err != nil {
		return out, err
	}
	repoNames := map[int64]string{}
	for _, r := range repos {
		repoNames[r.ID] = r.Name
	}
	rows, err := q.ListGithubPullRequests(ctx)
	if err != nil {
		return out, err
	}
	prs := map[string]*model.PullRequest{}
	for _, r := range rows {
		created, err := parseTime(r.CreatedAt)
		if err != nil {
			return out, err
		}
		updated, err := parseTime(r.UpdatedAt)
		if err != nil {
			return out, err
		}
		if _, ok := repoNames[r.RepositoryID]; !ok {
			return out, errors.New("missing GitHub repository")
		}
		prs[r.ID] = &model.PullRequest{ID: r.ProviderID, Repository: repoNames[r.RepositoryID], Number: int(r.Number), Body: r.Body, HeadOID: r.HeadOid, HeadRef: r.HeadRef, Title: r.Title, URL: r.Url, Author: r.Author, Draft: r.Draft != 0, CreatedAt: created, UpdatedAt: updated, Review: model.Status(r.Review), CI: model.Status(r.Ci), Mergeability: model.Status(r.Mergeability)}
	}
	labels, err := q.ListGithubLabels(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range labels {
		if prs[r.PullRequestID] == nil {
			return out, errors.New("missing GitHub pull request")
		}
		prs[r.PullRequestID].Labels = append(prs[r.PullRequestID].Labels, r.Label)
	}
	reviewers, err := q.ListGithubPullRequestReviewers(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range reviewers {
		p := prs[r.PullRequestID]
		if p == nil {
			return out, errors.New("missing GitHub pull request")
		}
		if r.Kind == "user" {
			p.RequestedReviewers = append(p.RequestedReviewers, r.Name)
		} else {
			p.RequestedTeams = append(p.RequestedTeams, r.Name)
		}
	}
	syncs, err := q.ListSyncState(ctx)
	if err != nil {
		return out, err
	}
	if len(syncs) != int(model.ViewCount) {
		return out, errors.New("incomplete GitHub sync state")
	}
	for _, r := range syncs {
		if r.ViewID < 0 || r.ViewID >= int64(model.ViewCount) {
			return out, errors.New("invalid GitHub view")
		}
		at, err := parseTime(r.SyncedAt)
		if err != nil {
			return out, err
		}
		out.Datasets[r.ViewID].SyncedAt = at
	}
	memberships, err := q.ListGithubViewMemberships(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range memberships {
		if r.ViewID < 0 || r.ViewID >= int64(model.ViewCount) {
			return out, errors.New("invalid GitHub view")
		}
		if prs[r.PullRequestID] == nil {
			return out, errors.New("missing GitHub pull request")
		}
		out.Datasets[r.ViewID].PRs = append(out.Datasets[r.ViewID].PRs, *prs[r.PullRequestID])
	}
	return out, nil
}
