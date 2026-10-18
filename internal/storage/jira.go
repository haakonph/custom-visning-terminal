package storage

import (
	"context"
	"errors"

	"github.com/haakonph/custom-visning-terminal/internal/model"
	"github.com/haakonph/custom-visning-terminal/internal/storage/dbgen"
)

func writeJira(ctx context.Context, q *dbgen.Queries, data model.JiraCache) error {
	if err := q.ClearJiraQueries(ctx); err != nil {
		return err
	}
	if err := q.ClearJiraIssues(ctx); err != nil {
		return err
	}
	if err := q.ClearJiraProjects(ctx); err != nil {
		return err
	}
	if err := q.InsertJiraQueries(ctx, dbgen.InsertJiraQueriesParams{ID: 1, Query: data.Query, SyncedAt: timestamp(data.SyncedAt)}); err != nil {
		return err
	}
	projects := map[string]bool{}
	issues := map[string]model.JiraIssue{}
	for _, i := range data.Issues {
		if old, ok := issues[i.Key]; !ok || !old.Updated.After(i.Updated) {
			issues[i.Key] = i
		}
	}
	for _, i := range issues {
		if !projects[i.Project] {
			if err := q.InsertJiraProjects(ctx, i.Project); err != nil {
				return err
			}
			projects[i.Project] = true
		}
		if err := q.InsertJiraIssues(ctx, dbgen.InsertJiraIssuesParams{IssueKey: i.Key, ProjectKey: i.Project, Url: i.URL, Title: i.Title, Status: i.Status, Priority: i.Priority, Assignee: i.Assignee, IssueType: i.Type, Description: i.Description, Due: i.Due, UpdatedAt: timestamp(i.Updated)}); err != nil {
			return err
		}
		for pos, label := range i.Labels {
			if err := q.InsertJiraLabels(ctx, dbgen.InsertJiraLabelsParams{IssueKey: i.Key, Position: int64(pos), Label: label}); err != nil {
				return err
			}
		}
	}
	for pos, i := range data.Issues {
		if err := q.InsertJiraQueryMemberships(ctx, dbgen.InsertJiraQueryMembershipsParams{QueryID: 1, Position: int64(pos), IssueKey: i.Key}); err != nil {
			return err
		}
	}
	return nil
}
func readJira(ctx context.Context, q *dbgen.Queries) (model.JiraCache, error) {
	out := model.JiraCache{}
	queries, err := q.ListJiraQueries(ctx)
	if err != nil {
		return out, err
	}
	if len(queries) != 1 {
		return out, errors.New("invalid Jira query data")
	}
	out.Query = queries[0].Query
	out.SyncedAt, err = parseTime(queries[0].SyncedAt)
	if err != nil {
		return out, err
	}
	rows, err := q.ListJiraIssues(ctx)
	if err != nil {
		return out, err
	}
	issues := map[string]*model.JiraIssue{}
	for _, r := range rows {
		at, err := parseTime(r.UpdatedAt)
		if err != nil {
			return out, err
		}
		issues[r.IssueKey] = &model.JiraIssue{Key: r.IssueKey, Project: r.ProjectKey, URL: r.Url, Title: r.Title, Status: r.Status, Priority: r.Priority, Assignee: r.Assignee, Type: r.IssueType, Description: r.Description, Due: r.Due, Updated: at}
	}
	labels, err := q.ListJiraLabels(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range labels {
		if issues[r.IssueKey] == nil {
			return out, errors.New("missing Jira issue")
		}
		issues[r.IssueKey].Labels = append(issues[r.IssueKey].Labels, r.Label)
	}
	members, err := q.ListJiraQueryMemberships(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range members {
		if issues[r.IssueKey] == nil {
			return out, errors.New("missing Jira issue")
		}
		out.Issues = append(out.Issues, *issues[r.IssueKey])
	}
	return out, nil
}
