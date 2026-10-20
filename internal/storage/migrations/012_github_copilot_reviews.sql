CREATE TABLE github_copilot_reviews (
    pull_request_id TEXT PRIMARY KEY REFERENCES github_pull_requests(id) ON DELETE CASCADE,
    recommendation TEXT NOT NULL,
    comments INTEGER NOT NULL
);
