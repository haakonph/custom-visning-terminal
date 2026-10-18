CREATE TABLE jira_projects (
    project_key TEXT PRIMARY KEY
);

CREATE TABLE jira_issues (
    issue_key TEXT PRIMARY KEY,
    project_key TEXT NOT NULL REFERENCES jira_projects(project_key),
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL,
    priority TEXT NOT NULL,
    assignee TEXT NOT NULL,
    issue_type TEXT NOT NULL,
    description TEXT NOT NULL,
    due TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE jira_labels (
    issue_key TEXT NOT NULL REFERENCES jira_issues(issue_key) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    label TEXT NOT NULL,
    PRIMARY KEY (issue_key, position)
);

CREATE TABLE jira_queries (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    query TEXT NOT NULL,
    synced_at TEXT NOT NULL
);

CREATE TABLE jira_query_memberships (
    query_id INTEGER NOT NULL REFERENCES jira_queries(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    issue_key TEXT NOT NULL REFERENCES jira_issues(issue_key) ON DELETE CASCADE,
    PRIMARY KEY (query_id, position)
);
