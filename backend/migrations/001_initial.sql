-- SQLite schema for the single-host Manager.
-- Timestamps are DATETIME text written in UTC by the driver
-- ("2006-01-02 15:04:05.999999999+00:00"), which keeps lexical order
-- equal to chronological order for range filters and retention purges.
-- UUIDs are stored as canonical text and JSON documents as text.

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    revoked_at DATETIME,
    ip_address TEXT,
    user_agent TEXT
);
CREATE INDEX sessions_user_expires_idx ON sessions (user_id, expires_at DESC);

CREATE TABLE audit_logs (
    id TEXT PRIMARY KEY,
    actor TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    action TEXT NOT NULL,
    request_id TEXT,
    outcome TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE host_infos (
    id TEXT PRIMARY KEY,
    singleton_key TEXT NOT NULL UNIQUE DEFAULT 'local',
    hostname TEXT NOT NULL,
    distribution TEXT,
    os_version TEXT,
    kernel_version TEXT,
    boot_time DATETIME,
    updated_at DATETIME NOT NULL
);

CREATE TABLE containers (
    id TEXT PRIMARY KEY,
    docker_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    image TEXT NOT NULL,
    state TEXT NOT NULL,
    status TEXT,
    created_at DATETIME,
    started_at DATETIME,
    first_seen_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    removed_at DATETIME,
    restart_count INTEGER NOT NULL DEFAULT 0,
    restart_policy TEXT,
    ports TEXT,
    networks TEXT,
    mounts TEXT,
    updated_at DATETIME NOT NULL
);
CREATE INDEX containers_state_name_idx ON containers (state, name);
CREATE INDEX containers_last_seen_idx ON containers (last_seen_at DESC);

CREATE TABLE system_metric_samples (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT,
    labels TEXT,
    recorded_at DATETIME NOT NULL
);
CREATE INDEX system_metric_name_time_idx ON system_metric_samples (name, recorded_at DESC);
CREATE INDEX system_metric_time_idx ON system_metric_samples (recorded_at);

CREATE TABLE container_metric_samples (
    id TEXT PRIMARY KEY,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT,
    labels TEXT,
    recorded_at DATETIME NOT NULL
);
CREATE INDEX container_metric_name_time_idx
    ON container_metric_samples (container_id, name, recorded_at DESC);
CREATE INDEX container_metric_time_idx ON container_metric_samples (recorded_at);

CREATE TABLE events (
    id TEXT PRIMARY KEY,
    container_id TEXT REFERENCES containers(id) ON DELETE SET NULL,
    type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info'
        CHECK (severity IN ('info', 'warning', 'error', 'critical')),
    message TEXT NOT NULL,
    metadata TEXT,
    occurred_at DATETIME NOT NULL
);
CREATE INDEX events_occurred_idx ON events (occurred_at DESC);
CREATE INDEX events_container_occurred_idx ON events (container_id, occurred_at DESC);
