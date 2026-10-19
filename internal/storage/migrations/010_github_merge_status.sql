ALTER TABLE github_pull_requests ADD COLUMN auto_merge TEXT NOT NULL DEFAULT 'UNKNOWN';
ALTER TABLE github_pull_requests ADD COLUMN merge_queue TEXT NOT NULL DEFAULT 'UNKNOWN';
ALTER TABLE github_pull_requests ADD COLUMN queue_position INTEGER NOT NULL DEFAULT 0;
