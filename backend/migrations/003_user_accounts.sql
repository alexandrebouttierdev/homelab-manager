-- Several Manager accounts with roles (SPEC_ADMINISTRATION.md § 3.1-3.2).
-- These are MonoOps accounts only, never Linux accounts.
-- Before this migration the only account was the administrator, so every
-- existing row becomes an administrator.
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'viewer'
    CHECK (role IN ('viewer', 'operator', 'admin'));
ALTER TABLE users ADD COLUMN disabled_at DATETIME;
ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN last_login_at DATETIME;
ALTER TABLE users ADD COLUMN last_login_ip TEXT;
UPDATE users SET role = 'admin', display_name = username;
