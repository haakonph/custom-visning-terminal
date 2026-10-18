-- name: ListLocalWorktrees :many
SELECT * FROM local_worktrees ORDER BY identity;

-- name: InsertLocalWorktrees :exec
INSERT INTO local_worktrees (identity, path, head) VALUES (sqlc.arg(identity), sqlc.arg(path), sqlc.arg(head));

-- name: ClearLocalWorktrees :exec
DELETE FROM local_worktrees;
