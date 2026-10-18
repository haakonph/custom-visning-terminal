-- name: ListGithubLabels :many
SELECT * FROM github_labels ORDER BY pull_request_id, position;

-- name: InsertGithubLabels :exec
INSERT INTO github_labels (pull_request_id, position, label) VALUES (sqlc.arg(pull_request_id), sqlc.arg(position), sqlc.arg(label));

-- name: ClearGithubLabels :exec
DELETE FROM github_labels;
