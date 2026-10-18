-- name: ListTasks :many
SELECT * FROM tasks ORDER BY position;

-- name: InsertTasks :exec
INSERT INTO tasks (position, stable_id, title) VALUES (sqlc.arg(position), sqlc.arg(stable_id), sqlc.arg(title));

-- name: ClearTasks :exec
DELETE FROM tasks;
