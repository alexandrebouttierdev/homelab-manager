-- Backups, SFTP repositories (SPEC_ADMINISTRATION.md § 7.2, decision D-13).
-- An SFTP repository pins the host key of its server; the Manager holds
-- one SSH key for its backups, its private part sealed by the secrets box.
-- SQLite does not change a CHECK constraint: backup_configs is rebuilt to
-- accept the type sftp, its row kept.
CREATE TABLE backup_configs_v2 (
    id TEXT PRIMARY KEY,
    singleton_key TEXT NOT NULL UNIQUE DEFAULT 'local',
    repository_type TEXT CHECK (repository_type IN ('local', 'rest', 's3', 'sftp')),
    repository_location TEXT,
    repository_host_key TEXT NOT NULL DEFAULT '',
    sealed_credentials TEXT NOT NULL DEFAULT '',
    initialized_at DATETIME,
    sources TEXT NOT NULL DEFAULT '[]',
    excludes TEXT NOT NULL DEFAULT '[]',
    exclude_caches BOOLEAN NOT NULL DEFAULT 1,
    exclude_if_present TEXT NOT NULL DEFAULT '',
    schedule_enabled BOOLEAN NOT NULL DEFAULT 0,
    schedule_frequency TEXT NOT NULL DEFAULT 'daily' CHECK (schedule_frequency IN ('daily', 'weekly')),
    schedule_weekday INTEGER NOT NULL DEFAULT 0 CHECK (schedule_weekday BETWEEN 0 AND 6),
    schedule_hour INTEGER NOT NULL DEFAULT 3 CHECK (schedule_hour BETWEEN 0 AND 23),
    schedule_minute INTEGER NOT NULL DEFAULT 0 CHECK (schedule_minute BETWEEN 0 AND 59),
    retention_enabled BOOLEAN NOT NULL DEFAULT 0,
    keep_daily INTEGER NOT NULL DEFAULT 7 CHECK (keep_daily BETWEEN 0 AND 1000),
    keep_weekly INTEGER NOT NULL DEFAULT 4 CHECK (keep_weekly BETWEEN 0 AND 1000),
    keep_monthly INTEGER NOT NULL DEFAULT 6 CHECK (keep_monthly BETWEEN 0 AND 1000),
    keep_yearly INTEGER NOT NULL DEFAULT 0 CHECK (keep_yearly BETWEEN 0 AND 1000),
    check_enabled BOOLEAN NOT NULL DEFAULT 1,
    check_subset_percent INTEGER NOT NULL DEFAULT 5 CHECK (check_subset_percent BETWEEN 1 AND 100),
    copy_repository_type TEXT CHECK (copy_repository_type IN ('local', 'rest', 's3', 'sftp')),
    copy_repository_location TEXT,
    copy_repository_host_key TEXT NOT NULL DEFAULT '',
    copy_sealed_credentials TEXT NOT NULL DEFAULT '',
    copy_initialized_at DATETIME,
    copy_after_backup BOOLEAN NOT NULL DEFAULT 0,
    ssh_public_key TEXT NOT NULL DEFAULT '',
    ssh_sealed_private_key TEXT NOT NULL DEFAULT '',
    ssh_key_created_at DATETIME,
    updated_at DATETIME NOT NULL,
    updated_by TEXT NOT NULL DEFAULT ''
);
INSERT INTO backup_configs_v2 (id, singleton_key, repository_type, repository_location, sealed_credentials,
        initialized_at, sources, excludes, exclude_caches, exclude_if_present, schedule_enabled, schedule_frequency,
        schedule_weekday, schedule_hour, schedule_minute, retention_enabled, keep_daily, keep_weekly, keep_monthly,
        keep_yearly, check_enabled, check_subset_percent, copy_repository_type, copy_repository_location,
        copy_sealed_credentials, copy_initialized_at, copy_after_backup, updated_at, updated_by)
    SELECT id, singleton_key, repository_type, repository_location, sealed_credentials,
        initialized_at, sources, excludes, exclude_caches, exclude_if_present, schedule_enabled, schedule_frequency,
        schedule_weekday, schedule_hour, schedule_minute, retention_enabled, keep_daily, keep_weekly, keep_monthly,
        keep_yearly, check_enabled, check_subset_percent, copy_repository_type, copy_repository_location,
        copy_sealed_credentials, copy_initialized_at, copy_after_backup, updated_at, updated_by
    FROM backup_configs;
DROP TABLE backup_configs;
ALTER TABLE backup_configs_v2 RENAME TO backup_configs;
