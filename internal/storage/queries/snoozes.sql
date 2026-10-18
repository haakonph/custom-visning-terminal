-- name: ListSnoozes :many
SELECT * FROM snoozes ORDER BY id;

-- name: InsertSnoozes :exec
INSERT INTO snoozes (id, kind, title, url, until_at) VALUES (sqlc.arg(id), sqlc.arg(kind), sqlc.arg(title), sqlc.arg(url), sqlc.arg(until_at));

-- name: ClearSnoozes :exec
DELETE FROM snoozes;
