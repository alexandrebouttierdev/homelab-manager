-- Backups with restic, increment 6A (SPEC_ADMINISTRATION.md § 7).

-- The singleton configuration of the module, set by an administrator.
-- sealed_credentials holds the repository password and its access keys,
-- sealed by the secrets box; sources and excludes are JSON arrays.
CREATE TABLE backup_configs (
    id TEXT PRIMARY KEY,
    singleton_key TEXT NOT NULL UNIQUE DEFAULT 'local',
    repository_type TEXT CHECK (repository_type IN ('local', 'rest', 's3')),
    repository_location TEXT,
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
    updated_at DATETIME NOT NULL,
    updated_by TEXT NOT NULL DEFAULT ''
);

-- One row per run of homelab-backup@<operation>.service.
CREATE TABLE backup_runs (
    id TEXT PRIMARY KEY,
    operation TEXT NOT NULL CHECK (operation IN ('init', 'backup')),
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
    started_at DATETIME NOT NULL,
    finished_at DATETIME
);
CREATE INDEX backup_runs_started_idx ON backup_runs (started_at DESC);
CREATE INDEX backup_runs_operation_state_idx ON backup_runs (operation, state, started_at DESC);

-- The snapshots of the repository made by the Manager, replaced after each
-- successful backup by the listing restic gives; paths and tags are JSON
-- arrays.
CREATE TABLE backup_snapshots (
    id TEXT PRIMARY KEY,
    short_id TEXT NOT NULL,
    taken_at DATETIME NOT NULL,
    hostname TEXT NOT NULL DEFAULT '',
    paths TEXT NOT NULL DEFAULT '[]',
    tags TEXT NOT NULL DEFAULT '[]',
    total_files INTEGER NOT NULL DEFAULT 0,
    total_bytes INTEGER NOT NULL DEFAULT 0,
    data_added INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX backup_snapshots_taken_idx ON backup_snapshots (taken_at DESC);
