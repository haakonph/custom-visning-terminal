CREATE TABLE local_folders (
    path TEXT PRIMARY KEY,
    repository TEXT NOT NULL DEFAULT '',
    issue_key TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    favorite INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1)),
    hidden INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1))
);
