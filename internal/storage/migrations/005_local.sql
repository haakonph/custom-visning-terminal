CREATE TABLE tasks (
    position INTEGER PRIMARY KEY,
    stable_id TEXT NOT NULL,
    title TEXT NOT NULL
);

CREATE TABLE task_references (
    task_position INTEGER NOT NULL REFERENCES tasks(position) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    provider TEXT NOT NULL,
    url TEXT NOT NULL,
    reference_key TEXT NOT NULL,
    title TEXT NOT NULL,
    role TEXT NOT NULL,
    PRIMARY KEY (task_position, position)
);

CREATE TABLE local_repositories (
    identity TEXT NOT NULL,
    path TEXT NOT NULL,
    PRIMARY KEY (identity)
);

CREATE TABLE local_worktrees (
    identity TEXT NOT NULL,
    path TEXT NOT NULL,
    head TEXT NOT NULL,
    PRIMARY KEY (identity)
);

CREATE TABLE snoozes (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    url TEXT NOT NULL,
    until_at TEXT NOT NULL
);
