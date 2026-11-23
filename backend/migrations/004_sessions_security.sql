-- Sessions, lockout and security policy (SPEC_ADMINISTRATION.md § 3.4-3.6).
-- last_seen_at is written at most once a minute per session.
ALTER TABLE sessions ADD COLUMN last_seen_at DATETIME;
ALTER TABLE sessions ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'password';
UPDATE sessions SET last_seen_at = created_at;

-- Singleton row of the policy set by an administrator. allowed_networks is
-- a JSON array of CIDR prefixes; an empty array allows every address.
CREATE TABLE security_policies (
    id TEXT PRIMARY KEY,
    singleton_key TEXT NOT NULL UNIQUE DEFAULT 'local',
    idle_timeout_minutes INTEGER NOT NULL DEFAULT 60
        CHECK (idle_timeout_minutes BETWEEN 5 AND 720),
    allowed_networks TEXT NOT NULL DEFAULT '[]',
    updated_at DATETIME NOT NULL
);

-- Consecutive failed passwords of an account; id is the account UUID.
CREATE TABLE login_failures (
    id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    last_failure_at DATETIME NOT NULL,
    locked_until DATETIME
);

CREATE INDEX audit_logs_created_idx ON audit_logs (created_at DESC);
