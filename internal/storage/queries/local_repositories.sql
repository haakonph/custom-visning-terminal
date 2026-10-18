-- name: ListLocalRepositories :many
SELECT * FROM local_repositories ORDER BY identity;

-- name: InsertLocalRepositories :exec
INSERT INTO local_repositories (identity, path) VALUES (sqlc.arg(identity), sqlc.arg(path));

-- name: ClearLocalRepositories :exec
DELETE FROM local_repositories;
