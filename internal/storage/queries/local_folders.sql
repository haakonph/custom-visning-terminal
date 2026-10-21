-- name: ListLocalFolders :many
SELECT * FROM local_folders ORDER BY path;

-- name: SaveLocalFolder :exec
INSERT INTO local_folders (path, repository, issue_key, title, favorite, hidden)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET repository=excluded.repository,
    issue_key=excluded.issue_key, title=excluded.title,
    favorite=excluded.favorite, hidden=excluded.hidden;
