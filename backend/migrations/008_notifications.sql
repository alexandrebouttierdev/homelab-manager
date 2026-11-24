-- Notifications of the alerts (SPEC_ADMINISTRATION.md § 6.2, § 6.3).

-- Last message sent about an alert, on any channel: spaces the reminders
-- across a restart of the Manager.
ALTER TABLE alerts ADD COLUMN last_notified_at DATETIME;

-- A silence suspends the notifications of the alerts it covers; the alerts
-- stay open. rule_id is not a foreign key: a silence outlives its rule
-- until it ends. lifted_at is set when a user ends it early.
CREATE TABLE alert_silences (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL CHECK (scope IN ('rule', 'target', 'all')),
    rule_id TEXT,
    target_key TEXT,
    reason TEXT NOT NULL,
    created_by TEXT NOT NULL,
    starts_at DATETIME NOT NULL,
    ends_at DATETIME NOT NULL,
    lifted_at DATETIME,
    lifted_by TEXT,
    CHECK ((scope = 'all' AND rule_id IS NULL AND target_key IS NULL)
        OR (scope = 'rule' AND rule_id IS NOT NULL AND target_key IS NULL)
        OR (scope = 'target' AND rule_id IS NOT NULL AND target_key IS NOT NULL)),
    CHECK (ends_at > starts_at)
);
CREATE INDEX alert_silences_ends_idx ON alert_silences (ends_at);

-- Notification channels. settings is the JSON object of the settings that
-- are not secret; secrets is the JSON object of the secrets sealed by the
-- secrets box with "notification-channel:<id>" as associated data, '' when
-- the channel has none; secret_names lists the secrets it holds.
CREATE TABLE notification_channels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('telegram', 'discord', 'gotify', 'ntfy', 'smtp', 'webhook')),
    enabled BOOLEAN NOT NULL DEFAULT 1,
    min_severity TEXT NOT NULL CHECK (min_severity IN ('info', 'warning', 'critical')),
    renotify_minutes INTEGER NOT NULL DEFAULT 30
        CHECK (renotify_minutes = 0 OR renotify_minutes BETWEEN 5 AND 1440),
    settings TEXT NOT NULL DEFAULT '{}',
    secrets TEXT NOT NULL DEFAULT '',
    secret_names TEXT NOT NULL DEFAULT '[]',
    last_sent_at DATETIME,
    last_error_at DATETIME,
    last_error TEXT,
    last_error_status INTEGER,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
