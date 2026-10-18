CREATE TABLE accounts (
    id INTEGER PRIMARY KEY,
    provider TEXT NOT NULL,
    username TEXT NOT NULL
);

CREATE TABLE github_repositories (
    id INTEGER PRIMARY KEY,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    UNIQUE (account_id, name)
);

CREATE TABLE github_pull_requests (
    id TEXT PRIMARY KEY,
    repository_id INTEGER NOT NULL REFERENCES github_repositories(id) ON DELETE CASCADE,
    number INTEGER NOT NULL,
    provider_id TEXT NOT NULL,
    body TEXT NOT NULL,
    head_oid TEXT NOT NULL,
    head_ref TEXT NOT NULL,
    title TEXT NOT NULL,
    url TEXT NOT NULL,
    author TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    review TEXT NOT NULL,
    ci TEXT NOT NULL,
    mergeability TEXT NOT NULL,
    draft INTEGER NOT NULL CHECK (draft IN (0, 1))
);

CREATE TABLE github_labels (
    pull_request_id TEXT NOT NULL REFERENCES github_pull_requests(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    label TEXT NOT NULL,
    PRIMARY KEY (pull_request_id, position)
);

CREATE TABLE github_pull_request_reviewers (
    pull_request_id TEXT NOT NULL REFERENCES github_pull_requests(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('user', 'team')),
    position INTEGER NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY (pull_request_id, kind, position)
);

CREATE TABLE sync_state (
    view_id INTEGER PRIMARY KEY,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    synced_at TEXT NOT NULL
);

CREATE TABLE github_view_memberships (
    view_id INTEGER NOT NULL REFERENCES sync_state(view_id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    pull_request_id TEXT NOT NULL REFERENCES github_pull_requests(id) ON DELETE CASCADE,
    PRIMARY KEY (view_id, position)
);
