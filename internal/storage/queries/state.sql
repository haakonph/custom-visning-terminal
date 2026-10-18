-- name: GetState :one
SELECT kind FROM storage_state WHERE kind = ?;

-- name: SetState :exec
INSERT INTO storage_state(kind) VALUES (?) ON CONFLICT(kind) DO NOTHING;

-- name: LoadLegacyDocument :one
SELECT payload FROM documents WHERE id = 1;

-- name: DeleteLegacyDocument :exec
DELETE FROM documents WHERE id = 1;
