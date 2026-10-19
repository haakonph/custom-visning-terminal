-- name: ListSnoozes :many
SELECT * FROM snoozes ORDER BY id;

-- name: InsertSnoozes :exec
INSERT INTO snoozes (id, kind, title, url, until_at) VALUES (sqlc.arg(id), sqlc.arg(kind), sqlc.arg(title), sqlc.arg(url), sqlc.arg(until_at));

-- name: ClearSnoozes :exec
DELETE FROM snoozes;

-- name: ListSnoozeReferences :many
SELECT * FROM snooze_references ORDER BY snooze_id, reference_role, position;

-- name: InsertSnoozeReference :exec
INSERT INTO snooze_references (snooze_id, reference_role, position, provider, url, reference_key, title)
VALUES (sqlc.arg(snooze_id), sqlc.arg(reference_role), sqlc.arg(position), sqlc.arg(provider), sqlc.arg(url), sqlc.arg(reference_key), sqlc.arg(title));
