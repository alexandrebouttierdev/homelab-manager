DROP TABLE IF EXISTS alerts;
DROP TABLE IF EXISTS operations;
DROP TABLE IF EXISTS agent_status;
DROP TABLE IF EXISTS agent_statuses;
DROP TABLE IF EXISTS agent_tokens;
DROP TABLE IF EXISTS metrics;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS events;

CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    ip_address VARCHAR(45),
    user_agent VARCHAR(512)
);
CREATE INDEX IF NOT EXISTS sessions_user_expires_idx
    ON sessions (user_id, expires_at DESC);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY,
    actor VARCHAR(255) NOT NULL,
    target_type VARCHAR(255) NOT NULL,
    target_id VARCHAR(255) NOT NULL,
    action VARCHAR(255) NOT NULL,
    request_id VARCHAR(255),
    outcome VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE host_infos (
    id UUID PRIMARY KEY,
    singleton_key VARCHAR(16) NOT NULL UNIQUE DEFAULT 'local',
    hostname VARCHAR(255) NOT NULL,
    distribution VARCHAR(255),
    os_version VARCHAR(255),
    kernel_version VARCHAR(255),
    boot_time TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE containers (
    id UUID PRIMARY KEY,
    docker_id VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    image TEXT NOT NULL,
    state VARCHAR(64) NOT NULL,
    status TEXT,
    created_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    removed_at TIMESTAMPTZ,
    restart_count BIGINT NOT NULL DEFAULT 0,
    restart_policy VARCHAR(255),
    ports JSONB,
    networks JSONB,
    mounts JSONB,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX containers_state_name_idx ON containers (state, name);
CREATE INDEX containers_last_seen_idx ON containers (last_seen_at DESC);

CREATE TABLE system_metric_samples (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    unit VARCHAR(32),
    labels JSONB,
    recorded_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX system_metric_name_time_idx ON system_metric_samples (name, recorded_at DESC);
CREATE INDEX system_metric_time_idx ON system_metric_samples (recorded_at);

CREATE TABLE container_metric_samples (
    id UUID PRIMARY KEY,
    container_id UUID NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    unit VARCHAR(32),
    labels JSONB,
    recorded_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX container_metric_name_time_idx
    ON container_metric_samples (container_id, name, recorded_at DESC);
CREATE INDEX container_metric_time_idx ON container_metric_samples (recorded_at);

CREATE TABLE events (
    id UUID PRIMARY KEY,
    container_id UUID REFERENCES containers(id) ON DELETE SET NULL,
    type VARCHAR(255) NOT NULL,
    severity VARCHAR(32) NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    metadata JSONB,
    occurred_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX events_occurred_idx ON events (occurred_at DESC);
CREATE INDEX events_container_occurred_idx ON events (container_id, occurred_at DESC);
