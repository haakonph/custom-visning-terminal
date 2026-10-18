-- name: ListGithubRepositories :many
SELECT * FROM github_repositories ORDER BY id;

-- name: InsertGithubRepositories :exec
INSERT INTO github_repositories (id, account_id, name) VALUES (sqlc.arg(id), sqlc.arg(account_id), sqlc.arg(name));

-- name: ClearGithubRepositories :exec
DELETE FROM github_repositories;
