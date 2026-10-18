CREATE TABLE copilot_sessions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    client TEXT NOT NULL,
    cwd TEXT NOT NULL,
    project_root TEXT NOT NULL,
    git_root TEXT NOT NULL,
    branch TEXT NOT NULL,
    repository TEXT NOT NULL,
    remote TEXT NOT NULL,
    extra_metadata TEXT NOT NULL,
    model TEXT NOT NULL,
    agent TEXT NOT NULL,
    jira TEXT NOT NULL,
    started_at TEXT NOT NULL,
    last_activity TEXT NOT NULL,
    status TEXT NOT NULL,
    status_source TEXT NOT NULL,
    input_tokens INTEGER,
    output_tokens INTEGER,
    duration_ms REAL
);

CREATE TABLE copilot_events (
    session_id TEXT NOT NULL REFERENCES copilot_sessions(id) ON DELETE CASCADE,
    event_key TEXT NOT NULL,
    source_key TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_session_id TEXT NOT NULL,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    start_time TEXT NOT NULL,
    state TEXT NOT NULL,
    metric_series TEXT NOT NULL,
    time TEXT NOT NULL,
    event_type TEXT NOT NULL,
    content TEXT NOT NULL,
    role TEXT NOT NULL,
    tool TEXT NOT NULL,
    call_id TEXT NOT NULL,
    args TEXT NOT NULL,
    result TEXT NOT NULL,
    error TEXT NOT NULL,
    warning TEXT NOT NULL,
    agent TEXT NOT NULL,
    parent TEXT NOT NULL,
    model TEXT NOT NULL,
    metadata TEXT NOT NULL,
    raw TEXT NOT NULL,
    temporality INTEGER NOT NULL,
    PRIMARY KEY (session_id, event_key)
);

CREATE TABLE copilot_traces (
    trace_id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES copilot_sessions(id) ON DELETE CASCADE
);

CREATE TABLE copilot_usage (
    session_id TEXT NOT NULL,
    event_key TEXT NOT NULL,
    input_tokens INTEGER,
    output_tokens INTEGER,
    duration_ms REAL,
    PRIMARY KEY (session_id, event_key),
    FOREIGN KEY (session_id, event_key) REFERENCES copilot_events(session_id, event_key) ON DELETE CASCADE
);
