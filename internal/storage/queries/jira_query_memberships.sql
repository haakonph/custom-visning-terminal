-- name: ListJiraQueryMemberships :many
SELECT * FROM jira_query_memberships ORDER BY query_id, position;

-- name: InsertJiraQueryMemberships :exec
INSERT INTO jira_query_memberships (query_id, position, issue_key) VALUES (sqlc.arg(query_id), sqlc.arg(position), sqlc.arg(issue_key));

-- name: ClearJiraQueryMemberships :exec
DELETE FROM jira_query_memberships;
