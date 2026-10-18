-- name: ListJiraLabels :many
SELECT * FROM jira_labels ORDER BY issue_key, position;

-- name: InsertJiraLabels :exec
INSERT INTO jira_labels (issue_key, position, label) VALUES (sqlc.arg(issue_key), sqlc.arg(position), sqlc.arg(label));

-- name: ClearJiraLabels :exec
DELETE FROM jira_labels;
