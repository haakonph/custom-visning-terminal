-- name: ListJiraIssues :many
SELECT * FROM jira_issues ORDER BY issue_key;

-- name: InsertJiraIssues :exec
INSERT INTO jira_issues (issue_key, project_key, url, title, status, priority, assignee, issue_type, description, due, updated_at) VALUES (sqlc.arg(issue_key), sqlc.arg(project_key), sqlc.arg(url), sqlc.arg(title), sqlc.arg(status), sqlc.arg(priority), sqlc.arg(assignee), sqlc.arg(issue_type), sqlc.arg(description), sqlc.arg(due), sqlc.arg(updated_at));

-- name: ClearJiraIssues :exec
DELETE FROM jira_issues;
