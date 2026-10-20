CREATE TABLE github_open_comments (
 pull_request_id TEXT PRIMARY KEY REFERENCES github_pull_requests(id) ON DELETE CASCADE,
 count INTEGER NOT NULL CHECK(count >= 0)
);
