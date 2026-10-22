-- name: ListCopilotUsage :many
SELECT * FROM copilot_usage ORDER BY session_id;

-- name: InsertCopilotUsage :exec
INSERT INTO copilot_usage (session_id, event_key, input_tokens, output_tokens, duration_ms) VALUES (sqlc.arg(session_id), sqlc.arg(event_key), sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(duration_ms)) ON CONFLICT (session_id, event_key) DO UPDATE SET input_tokens = excluded.input_tokens, output_tokens = excluded.output_tokens, duration_ms = excluded.duration_ms;

-- name: ClearCopilotUsage :exec
DELETE FROM copilot_usage;

-- name: DeleteCopilotEventUsage :exec
DELETE FROM copilot_usage WHERE session_id = ? AND event_key = ?;
