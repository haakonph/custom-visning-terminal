-- Read-only projection for SQL inspection. The terminal views remain unchanged.
CREATE VIEW overview_items AS
SELECT 'github' AS provider, p.id AS item_id, p.title, p.url, p.updated_at AS last_activity,
       r.name AS repository, p.head_ref AS branch, p.review AS status
FROM github_pull_requests p JOIN github_repositories r ON r.id = p.repository_id
UNION ALL
SELECT 'jira', issue_key, title, url, updated_at, '', '', status FROM jira_issues
UNION ALL
SELECT 'copilot', id, name, '', last_activity, repository, branch, status FROM copilot_sessions;
CREATE INDEX github_pr_updated ON github_pull_requests(updated_at);
CREATE INDEX jira_issue_updated ON jira_issues(updated_at);
CREATE INDEX copilot_event_time ON copilot_events(session_id, time);
CREATE INDEX snooze_deadline ON snoozes(until_at);
