CREATE TABLE github_review_summaries (
    pull_request_id TEXT PRIMARY KEY REFERENCES github_pull_requests(id) ON DELETE CASCADE,
    approved INTEGER NOT NULL,
    changes INTEGER NOT NULL,
    pending INTEGER NOT NULL,
    pending_teams INTEGER NOT NULL,
    complete INTEGER NOT NULL CHECK (complete IN (0, 1)),
    fallback INTEGER NOT NULL CHECK (fallback IN (0, 1))
);
