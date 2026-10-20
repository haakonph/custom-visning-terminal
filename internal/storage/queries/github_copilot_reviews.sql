-- name: ListGithubCopilotReviews :many
SELECT * FROM github_copilot_reviews ORDER BY pull_request_id;

-- name: InsertGithubCopilotReviews :exec
INSERT INTO github_copilot_reviews (pull_request_id, recommendation, comments) VALUES (sqlc.arg(pull_request_id), sqlc.arg(recommendation), sqlc.arg(comments));
