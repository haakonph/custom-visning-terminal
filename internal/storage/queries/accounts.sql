-- name: ListAccounts :many
SELECT * FROM accounts ORDER BY id;

-- name: InsertAccounts :exec
INSERT INTO accounts (id, provider, username) VALUES (sqlc.arg(id), sqlc.arg(provider), sqlc.arg(username));

-- name: ClearAccounts :exec
DELETE FROM accounts;
