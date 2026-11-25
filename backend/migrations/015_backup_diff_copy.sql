-- Backups, increment 6C (SPEC_ADMINISTRATION.md § 7.2 and § 7.4): diff
-- between two snapshots and copies to a second repository (restic copy).
-- The copy repository has its own sealed credentials; copy_after_backup
-- copies after each successful scheduled backup.

ALTER TABLE backup_configs ADD COLUMN copy_repository_type TEXT CHECK (copy_repository_type IN ('local', 'rest', 's3'));
ALTER TABLE backup_configs ADD COLUMN copy_repository_location TEXT;
ALTER TABLE backup_configs ADD COLUMN copy_sealed_credentials TEXT NOT NULL DEFAULT '';
ALTER TABLE backup_configs ADD COLUMN copy_initialized_at DATETIME;
ALTER TABLE backup_configs ADD COLUMN copy_after_backup BOOLEAN NOT NULL DEFAULT 0;

-- SQLite does not change a CHECK constraint: backup_runs is rebuilt to
-- accept the operations diff and copy, its rows kept.
CREATE TABLE backup_runs_v3 (
    id TEXT PRIMARY KEY,
    operation TEXT NOT NULL
        CHECK (operation IN ('init', 'backup', 'forget-prune', 'check', 'restore', 'snapshots', 'unlock', 'diff', 'copy')),
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
INSERT INTO backup_runs_v3 SELECT id, operation, origin, requested_by, state, failure, message, warning, snapshot_id,
        files_new, files_changed, files_unmodified, dirs_new, dirs_changed, dirs_unmodified, data_added,
        total_files_processed, total_bytes_processed, duration_seconds, details, started_at, finished_at
    FROM backup_runs;
DROP TABLE backup_runs;
ALTER TABLE backup_runs_v3 RENAME TO backup_runs;
CREATE INDEX backup_runs_started_idx ON backup_runs (started_at DESC);
CREATE INDEX backup_runs_operation_state_idx ON backup_runs (operation, state, started_at DESC);
