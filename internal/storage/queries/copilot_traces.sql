-- name: ListCopilotTraces :many
SELECT * FROM copilot_traces ORDER BY trace_id;

-- name: InsertCopilotTraces :exec
INSERT INTO copilot_traces (trace_id, session_id) VALUES (sqlc.arg(trace_id), sqlc.arg(session_id));

-- name: ClearCopilotTraces :exec
DELETE FROM copilot_traces;
