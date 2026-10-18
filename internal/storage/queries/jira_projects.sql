-- name: ListJiraProjects :many
SELECT * FROM jira_projects ORDER BY project_key;

-- name: InsertJiraProjects :exec
INSERT INTO jira_projects (project_key) VALUES (sqlc.arg(project_key));

-- name: ClearJiraProjects :exec
DELETE FROM jira_projects;
