CREATE TABLE usage_metrics (
    day TEXT NOT NULL,
    category TEXT NOT NULL,
    name TEXT NOT NULL,
    source TEXT NOT NULL,
    target TEXT NOT NULL,
    count INTEGER NOT NULL CHECK (count > 0),
    PRIMARY KEY (day, category, name, source, target)
);
