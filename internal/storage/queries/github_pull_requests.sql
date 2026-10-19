-- name: ListGithubPullRequests :many
SELECT * FROM github_pull_requests ORDER BY id;

-- name: InsertGithubPullRequests :exec
INSERT INTO github_pull_requests (id, repository_id, number, provider_id, body, head_oid, head_ref, title, url, author, created_at, updated_at, review, ci, mergeability, draft, auto_merge, merge_queue, queue_position) VALUES (sqlc.arg(id), sqlc.arg(repository_id), sqlc.arg(number), sqlc.arg(provider_id), sqlc.arg(body), sqlc.arg(head_oid), sqlc.arg(head_ref), sqlc.arg(title), sqlc.arg(url), sqlc.arg(author), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(review), sqlc.arg(ci), sqlc.arg(mergeability), sqlc.arg(draft), sqlc.arg(auto_merge), sqlc.arg(merge_queue), sqlc.arg(queue_position));

-- name: ClearGithubPullRequests :exec
DELETE FROM github_pull_requests;
