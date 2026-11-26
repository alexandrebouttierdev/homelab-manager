-- Compact metric storage (METRICS_COLLECTION.md § 4, DATA_MODEL.md).
--
-- A series (host or container, name, unit, labels) is stored once; each
-- point keeps only its series, its time in Unix milliseconds (UTC) and its
-- value. Raw points are kept 24 hours; 5-minute rollups (sum, min, max,
-- count) cover the whole retention.
--
-- The former tables system_metric_samples and container_metric_samples are
-- not converted here: the Manager converts them in the background, in short
-- batches, then drops them, so that this migration stays instant whatever
-- their size.

CREATE TABLE metric_series (
    id INTEGER PRIMARY KEY,
    -- NULL for the host.
    container_id TEXT REFERENCES containers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    unit TEXT NOT NULL DEFAULT '',
    -- Canonical JSON object (sorted keys), '' without labels.
    labels TEXT NOT NULL DEFAULT ''
);
-- NULL values are distinct in a UNIQUE index: the host is indexed as ''.
CREATE UNIQUE INDEX metric_series_key_idx
    ON metric_series (COALESCE(container_id, ''), name, unit, labels);
CREATE INDEX metric_series_container_name_idx ON metric_series (container_id, name);

CREATE TABLE metric_points (
    series_id INTEGER NOT NULL REFERENCES metric_series(id) ON DELETE CASCADE,
    recorded_at INTEGER NOT NULL,
    value REAL NOT NULL,
    PRIMARY KEY (series_id, recorded_at)
) WITHOUT ROWID;
CREATE INDEX metric_points_time_idx ON metric_points (recorded_at);

CREATE TABLE metric_rollups (
    series_id INTEGER NOT NULL REFERENCES metric_series(id) ON DELETE CASCADE,
    -- Unix seconds, a multiple of 300.
    bucket_start INTEGER NOT NULL,
    value_sum REAL NOT NULL,
    value_min REAL NOT NULL,
    value_max REAL NOT NULL,
    sample_count INTEGER NOT NULL CHECK (sample_count > 0),
    PRIMARY KEY (series_id, bucket_start)
) WITHOUT ROWID;
CREATE INDEX metric_rollups_time_idx ON metric_rollups (bucket_start);

-- rolled_until (Unix seconds, a multiple of 300): every point recorded
-- before it is counted in metric_rollups; later points are not yet.
CREATE TABLE metric_rollup_state (
    singleton_key TEXT PRIMARY KEY CHECK (singleton_key = 'local'),
    rolled_until INTEGER NOT NULL
);
INSERT INTO metric_rollup_state (singleton_key, rolled_until) VALUES ('local', 0);
