ALTER TABLE tasks ADD COLUMN archived INTEGER NOT NULL DEFAULT 0;

CREATE TABLE entity_links (
    source_provider TEXT NOT NULL,
    source_url TEXT NOT NULL,
    source_key TEXT NOT NULL,
    source_title TEXT NOT NULL,
    target_provider TEXT NOT NULL,
    target_url TEXT NOT NULL,
    target_key TEXT NOT NULL,
    target_title TEXT NOT NULL,
    PRIMARY KEY (source_provider, source_url, target_provider, target_url)
);

CREATE TABLE archived_entities (
    provider TEXT NOT NULL,
    url TEXT NOT NULL,
    reference_key TEXT NOT NULL,
    title TEXT NOT NULL,
    PRIMARY KEY (provider, url)
);
