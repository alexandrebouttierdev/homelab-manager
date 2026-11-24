-- Package vulnerabilities found by Grype (SPEC_ADMINISTRATION.md § 8 ter.4).

-- One row per scan. origin tells what started it; the totals count the
-- findings present after a successful scan, new_count and resolved_count
-- what it changed.
CREATE TABLE vulnerability_scans (
    id TEXT PRIMARY KEY,
    origin TEXT NOT NULL CHECK (origin IN ('startup', 'schedule', 'retry', 'manual')),
    requested_by TEXT,
    state TEXT NOT NULL CHECK (state IN ('running', 'succeeded', 'failed')),
    failure TEXT,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    grype_version TEXT NOT NULL DEFAULT '',
    db_built_at DATETIME,
    distro_id TEXT NOT NULL DEFAULT '',
    distro_version TEXT NOT NULL DEFAULT '',
    distro_name TEXT NOT NULL DEFAULT '',
    package_count INTEGER NOT NULL DEFAULT 0,
    critical_count INTEGER NOT NULL DEFAULT 0,
    high_count INTEGER NOT NULL DEFAULT 0,
    medium_count INTEGER NOT NULL DEFAULT 0,
    low_count INTEGER NOT NULL DEFAULT 0,
    negligible_count INTEGER NOT NULL DEFAULT 0,
    unknown_count INTEGER NOT NULL DEFAULT 0,
    new_count INTEGER NOT NULL DEFAULT 0,
    resolved_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX vulnerability_scans_started_idx ON vulnerability_scans (started_at DESC);

-- One row per vulnerability of a package version while it is present;
-- resolved_at is set when a successful scan no longer reports it. A
-- vulnerability found again later opens a new row. severity_rank (0 for
-- critical to 5 for unknown) orders the findings; fixed_versions and urls
-- are JSON arrays.
CREATE TABLE vulnerability_findings (
    id TEXT PRIMARY KEY,
    vulnerability_id TEXT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low', 'negligible', 'unknown')),
    severity_rank INTEGER NOT NULL,
    package TEXT NOT NULL,
    version TEXT NOT NULL,
    package_type TEXT NOT NULL DEFAULT '',
    fix_state TEXT NOT NULL CHECK (fix_state IN ('fixed', 'not-fixed', 'wont-fix', 'unknown')),
    fixed_versions TEXT NOT NULL DEFAULT '[]',
    description TEXT NOT NULL DEFAULT '',
    urls TEXT NOT NULL DEFAULT '[]',
    data_source TEXT NOT NULL DEFAULT '',
    first_seen_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    resolved_at DATETIME,
    first_scan_id TEXT NOT NULL,
    last_scan_id TEXT NOT NULL
);
CREATE UNIQUE INDEX vulnerability_findings_present_idx
    ON vulnerability_findings (vulnerability_id, package, version) WHERE resolved_at IS NULL;
CREATE INDEX vulnerability_findings_order_idx ON vulnerability_findings (severity_rank, first_seen_at DESC);
CREATE INDEX vulnerability_findings_resolved_idx ON vulnerability_findings (resolved_at) WHERE resolved_at IS NOT NULL;

-- The alert rules accept the vulnerabilities type: SQLite cannot change a
-- CHECK constraint, the table is rebuilt with its rows. The alerts are
-- untouched (rule_id is not a foreign key).
CREATE TABLE alert_rules_v2 (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL
        CHECK (type IN ('host_metric', 'container_metric', 'container_state', 'unit_failed', 'collector', 'vulnerabilities')),
    severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    enabled BOOLEAN NOT NULL DEFAULT 1,
    is_default BOOLEAN NOT NULL DEFAULT 0,
    params TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
INSERT INTO alert_rules_v2 (id, name, type, severity, enabled, is_default, params, created_at, updated_at)
    SELECT id, name, type, severity, enabled, is_default, params, created_at, updated_at FROM alert_rules;
DROP TABLE alert_rules;
ALTER TABLE alert_rules_v2 RENAME TO alert_rules;

-- Interval of the automatic vulnerability scans chosen in the interface;
-- NULL keeps vulnerability_scan_interval_hours of the configuration.
ALTER TABLE app_settings ADD COLUMN vulnerability_scan_interval_hours INTEGER
    CHECK (vulnerability_scan_interval_hours IS NULL OR vulnerability_scan_interval_hours BETWEEN 6 AND 168);
