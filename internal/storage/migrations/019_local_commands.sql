-- Position is presentation order; stable_id is the task identity.
UPDATE tasks SET stable_id = lower(hex(randomblob(16))) WHERE stable_id = '';
CREATE UNIQUE INDEX tasks_stable_id ON tasks(stable_id);
ALTER TABLE tasks ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;

-- Durable migration metadata belongs to the local store, never to a cache.
CREATE TABLE local_imports (
    kind TEXT PRIMARY KEY,
    source_path TEXT NOT NULL,
    fingerprint TEXT NOT NULL
);
