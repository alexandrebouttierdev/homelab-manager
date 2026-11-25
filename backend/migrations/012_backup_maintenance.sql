-- Backups, increment 6B (SPEC_ADMINISTRATION.md § 7.3 and § 7.4):
-- retention, check, restore, connection of an existing repository, unlock.

ALTER TABLE backup_configs ADD COLUMN retention_enabled BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE backup_configs ADD COLUMN keep_daily INTEGER NOT NULL DEFAULT 7 CHECK (keep_daily BETWEEN 0 AND 1000);
ALTER TABLE backup_configs ADD COLUMN keep_weekly INTEGER NOT NULL DEFAULT 4 CHECK (keep_weekly BETWEEN 0 AND 1000);
ALTER TABLE backup_configs ADD COLUMN keep_monthly INTEGER NOT NULL DEFAULT 6 CHECK (keep_monthly BETWEEN 0 AND 1000);
ALTER TABLE backup_configs ADD COLUMN keep_yearly INTEGER NOT NULL DEFAULT 0 CHECK (keep_yearly BETWEEN 0 AND 1000);
ALTER TABLE backup_configs ADD COLUMN check_enabled BOOLEAN NOT NULL DEFAULT 1;
ALTER TABLE backup_configs ADD COLUMN check_subset_percent INTEGER NOT NULL DEFAULT 5 CHECK (check_subset_percent BETWEEN 1 AND 100);

-- SQLite does not change a CHECK constraint: backup_runs is rebuilt to
-- accept the new operations, its rows kept. details holds the figures of a
-- restore, a forget or a check (JSON, NULL otherwise).
CREATE TABLE backup_runs_v2 (
    id TEXT PRIMARY KEY,
    operation TEXT NOT NULL
        CHECK (operation IN ('init', 'backup', 'forget-prune', 'check', 'restore', 'snapshots', 'unlock')),
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'schedule', 'catchup')),
    requested_by TEXT,
    state TEXT NOT NULL CHECK (state IN ('running', 'succeeded', 'failed')),
    failure TEXT,
    message TEXT NOT NULL DEFAULT '',
    warning BOOLEAN NOT NULL DEFAULT 0,
    snapshot_id TEXT NOT NULL DEFAULT '',
    files_new INTEGER,
    files_changed INTEGER,
    files_unmodified INTEGER,
    dirs_new INTEGER,
    dirs_changed INTEGER,
    dirs_unmodified INTEGER,
    data_added INTEGER,
    total_files_processed INTEGER,
    total_bytes_processed INTEGER,
    duration_seconds REAL,
    details TEXT,
    started_at DATETIME NOT NULL,
    finished_at DATETIME
);
INSERT INTO backup_runs_v2 (id, operation, origin, requested_by, state, failure, message, warning, snapshot_id,
        files_new, files_changed, files_unmodified, dirs_new, dirs_changed, dirs_unmodified, data_added,
        total_files_processed, total_bytes_processed, duration_seconds, started_at, finished_at)
    SELECT id, operation, origin, requested_by, state, failure, message, warning, snapshot_id,
        files_new, files_changed, files_unmodified, dirs_new, dirs_changed, dirs_unmodified, data_added,
        total_files_processed, total_bytes_processed, duration_seconds, started_at, finished_at
    FROM backup_runs;
DROP TABLE backup_runs;
ALTER TABLE backup_runs_v2 RENAME TO backup_runs;
CREATE INDEX backup_runs_started_idx ON backup_runs (started_at DESC);
CREATE INDEX backup_runs_operation_state_idx ON backup_runs (operation, state, started_at DESC);
