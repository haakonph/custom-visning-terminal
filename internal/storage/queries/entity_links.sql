-- name: ClearEntityLinks :exec
DELETE FROM entity_links;
-- name: InsertEntityLink :exec
INSERT INTO entity_links (source_provider, source_url, source_key, source_title, target_provider, target_url, target_key, target_title)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
-- name: ListEntityLinks :many
SELECT * FROM entity_links ORDER BY source_provider, source_url, target_provider, target_url;
-- name: ClearArchivedEntities :exec
DELETE FROM archived_entities;
-- name: InsertArchivedEntity :exec
INSERT INTO archived_entities (provider, url, reference_key, title) VALUES (?, ?, ?, ?);
-- name: ListArchivedEntities :many
SELECT * FROM archived_entities ORDER BY provider, url;
