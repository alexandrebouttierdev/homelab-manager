CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    hostname VARCHAR(255) NOT NULL UNIQUE,
    agent_version VARCHAR(255),
    os VARCHAR(255),
    architecture VARCHAR(255),
    capabilities JSONB,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS agent_tokens (
    id UUID PRIMARY KEY,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_statuses (
    id UUID PRIMARY KEY,
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    details JSONB
);
CREATE INDEX IF NOT EXISTS agent_statuses_agent_observed_idx
    ON agent_statuses (agent_id, observed_at DESC);

CREATE TABLE IF NOT EXISTS metrics (
    id UUID PRIMARY KEY,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    unit VARCHAR(32),
    labels JSONB,
    recorded_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS metrics_agent_name_time_idx
    ON metrics (agent_id, name, recorded_at DESC);
