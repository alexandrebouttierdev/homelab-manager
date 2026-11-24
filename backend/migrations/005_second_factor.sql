-- Second factor (SPEC_ADMINISTRATION.md § 3.3, 3.6). The TOTP secret is
-- sealed by the secrets box (AES-256-GCM, § 2.5); recovery codes are stored
-- as SHA-256 hashes and used once.
CREATE TABLE totp_secrets (
    id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    secret_sealed TEXT NOT NULL,
    confirmed_at DATETIME,
    last_used_step INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL
);

CREATE TABLE recovery_codes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL UNIQUE,
    used_at DATETIME,
    created_at DATETIME NOT NULL
);
CREATE INDEX recovery_codes_user_idx ON recovery_codes (user_id);

ALTER TABLE security_policies ADD COLUMN require_admin_second_factor BOOLEAN NOT NULL DEFAULT 0;
