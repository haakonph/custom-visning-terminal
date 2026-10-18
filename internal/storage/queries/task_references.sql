-- name: ListTaskReferences :many
SELECT * FROM task_references ORDER BY task_position, position;

-- name: InsertTaskReferences :exec
INSERT INTO task_references (task_position, position, provider, url, reference_key, title, role) VALUES (sqlc.arg(task_position), sqlc.arg(position), sqlc.arg(provider), sqlc.arg(url), sqlc.arg(reference_key), sqlc.arg(title), sqlc.arg(role));

-- name: ClearTaskReferences :exec
DELETE FROM task_references;
