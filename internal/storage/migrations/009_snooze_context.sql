CREATE TABLE snooze_references (
    snooze_id TEXT NOT NULL REFERENCES snoozes(id) ON DELETE CASCADE,
    reference_role TEXT NOT NULL CHECK (reference_role IN ('root', 'member')),
    position INTEGER NOT NULL,
    provider TEXT NOT NULL,
    url TEXT NOT NULL,
    reference_key TEXT NOT NULL,
    title TEXT NOT NULL,
    PRIMARY KEY (snooze_id, reference_role, position)
);
