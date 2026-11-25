-- Backups, decision D-2: restores to the original location
-- (restore-in-place) and archives to download (dump). SQLite does not
-- change a CHECK constraint: backup_runs is rebuilt to accept the two
-- operations, its rows kept.
CREATE TABLE backup_runs_v4 (
    id TEXT PRIMARY KEY,
    operation TEXT NOT NULL
        CHECK (operation IN ('init', 'backup', 'forget-prune', 'check', 'restore', 'snapshots', 'unlock', 'diff', 'copy',
            'restore-in-place', 'dump')),
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
INSERT INTO backup_runs_v4 SELECT id, operation, origin, requested_by, state, failure, message, warning, snapshot_id,
        files_new, files_changed, files_unmodified, dirs_new, dirs_changed, dirs_unmodified, data_added,
        total_files_processed, total_bytes_processed, duration_seconds, details, started_at, finished_at
    FROM backup_runs;
DROP TABLE backup_runs;
ALTER TABLE backup_runs_v4 RENAME TO backup_runs;
CREATE INDEX backup_runs_started_idx ON backup_runs (started_at DESC);
CREATE INDEX backup_runs_operation_state_idx ON backup_runs (operation, state, started_at DESC);
