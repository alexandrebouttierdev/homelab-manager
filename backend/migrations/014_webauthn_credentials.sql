-- Passkeys and FIDO2 security keys (SPEC_ADMINISTRATION.md § 3.3, phase
-- 8B). data holds the credential as the WebAuthn library stores it: public
-- key, sign count and flags, no secret. Deleting an account deletes its
-- keys.
CREATE TABLE webauthn_credentials (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    credential_id BLOB NOT NULL UNIQUE,
    data BLOB NOT NULL,
    created_at DATETIME NOT NULL,
    last_used_at DATETIME
);
CREATE INDEX webauthn_credentials_user_idx ON webauthn_credentials (user_id);
