-- name: ListGithubPullRequestReviewers :many
SELECT * FROM github_pull_request_reviewers ORDER BY pull_request_id, kind, position;

-- name: InsertGithubPullRequestReviewers :exec
INSERT INTO github_pull_request_reviewers (pull_request_id, kind, position, name) VALUES (sqlc.arg(pull_request_id), sqlc.arg(kind), sqlc.arg(position), sqlc.arg(name));

-- name: ClearGithubPullRequestReviewers :exec
DELETE FROM github_pull_request_reviewers;
