-- name: ListSyncState :many
SELECT * FROM sync_state ORDER BY view_id;

-- name: InsertSyncState :exec
INSERT INTO sync_state (view_id, account_id, synced_at) VALUES (sqlc.arg(view_id), sqlc.arg(account_id), sqlc.arg(synced_at));

-- name: ClearSyncState :exec
DELETE FROM sync_state;
