-- Decision D-14: an administrator may allow accounts to sign in with a
-- passkey alone, without their password. Off by default.
ALTER TABLE security_policies ADD COLUMN allow_passkey_login BOOLEAN NOT NULL DEFAULT 0;
