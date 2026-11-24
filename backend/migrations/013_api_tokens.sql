-- API tokens (SPEC_ADMINISTRATION.md § 3.5, phase 8A): read-only scopes,
-- stored as the SHA-256 of the token, shown once. Deleting an account
-- deletes its tokens.
CREATE TABLE api_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    prefix TEXT NOT NULL,
    scopes TEXT NOT NULL DEFAULT '[]',
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    last_used_at DATETIME
);
CREATE INDEX api_tokens_user_idx ON api_tokens (user_id);
