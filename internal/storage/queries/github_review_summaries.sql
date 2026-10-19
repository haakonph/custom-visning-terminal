-- name: ListGithubReviewSummaries :many
SELECT * FROM github_review_summaries ORDER BY pull_request_id;

-- name: InsertGithubReviewSummaries :exec
INSERT INTO github_review_summaries (pull_request_id, approved, changes, pending, pending_teams, complete, fallback) VALUES (sqlc.arg(pull_request_id), sqlc.arg(approved), sqlc.arg(changes), sqlc.arg(pending), sqlc.arg(pending_teams), sqlc.arg(complete), sqlc.arg(fallback));
