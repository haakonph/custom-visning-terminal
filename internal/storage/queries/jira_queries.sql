-- name: ListJiraQueries :many
SELECT * FROM jira_queries ORDER BY id;

-- name: InsertJiraQueries :exec
INSERT INTO jira_queries (id, query, synced_at) VALUES (sqlc.arg(id), sqlc.arg(query), sqlc.arg(synced_at));

-- name: ClearJiraQueries :exec
DELETE FROM jira_queries;
