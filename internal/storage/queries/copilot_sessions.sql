-- name: ListCopilotSessions :many
SELECT * FROM copilot_sessions ORDER BY id;

-- name: InsertCopilotSessions :exec
INSERT INTO copilot_sessions (id, name, client, cwd, project_root, git_root, branch, repository, remote, extra_metadata, model, agent, jira, started_at, last_activity, status, status_source, input_tokens, output_tokens, duration_ms) VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(client), sqlc.arg(cwd), sqlc.arg(project_root), sqlc.arg(git_root), sqlc.arg(branch), sqlc.arg(repository), sqlc.arg(remote), sqlc.arg(extra_metadata), sqlc.arg(model), sqlc.arg(agent), sqlc.arg(jira), sqlc.arg(started_at), sqlc.arg(last_activity), sqlc.arg(status), sqlc.arg(status_source), sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(duration_ms)) ON CONFLICT (id) DO UPDATE SET name = excluded.name, client = excluded.client, cwd = excluded.cwd, project_root = excluded.project_root, git_root = excluded.git_root, branch = excluded.branch, repository = excluded.repository, remote = excluded.remote, extra_metadata = excluded.extra_metadata, model = excluded.model, agent = excluded.agent, jira = excluded.jira, started_at = excluded.started_at, last_activity = excluded.last_activity, status = excluded.status, status_source = excluded.status_source, input_tokens = excluded.input_tokens, output_tokens = excluded.output_tokens, duration_ms = excluded.duration_ms;

-- name: ClearCopilotSessions :exec
DELETE FROM copilot_sessions;

-- name: DeleteCopilotSession :exec
DELETE FROM copilot_sessions WHERE id = ?;
