-- name: ListGithubViewMemberships :many
SELECT * FROM github_view_memberships ORDER BY view_id, position;

-- name: InsertGithubViewMemberships :exec
INSERT INTO github_view_memberships (view_id, position, pull_request_id) VALUES (sqlc.arg(view_id), sqlc.arg(position), sqlc.arg(pull_request_id));

-- name: ClearGithubViewMemberships :exec
DELETE FROM github_view_memberships;
