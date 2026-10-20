-- name: ListGithubOpenComments :many
SELECT pull_request_id, count FROM github_open_comments ORDER BY pull_request_id;

-- name: InsertGithubOpenComments :exec
INSERT INTO github_open_comments (pull_request_id, count) VALUES (sqlc.arg(pull_request_id), sqlc.arg(count));
