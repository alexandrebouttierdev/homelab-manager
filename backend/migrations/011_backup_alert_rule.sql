-- Backup alert rule (SPEC_ADMINISTRATION.md § 6.1, phase 6). SQLite does not
-- change a CHECK constraint: alert_rules is rebuilt to accept the type
-- backup, its rows kept; alerts are untouched (rule_id is not a foreign
-- key).
CREATE TABLE alert_rules_v3 (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL
        CHECK (type IN ('host_metric', 'container_metric', 'container_state', 'unit_failed', 'collector', 'vulnerabilities', 'backup')),
    severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    enabled BOOLEAN NOT NULL DEFAULT 1,
    is_default BOOLEAN NOT NULL DEFAULT 0,
    params TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
INSERT INTO alert_rules_v3 (id, name, type, severity, enabled, is_default, params, created_at, updated_at)
    SELECT id, name, type, severity, enabled, is_default, params, created_at, updated_at FROM alert_rules;
DROP TABLE alert_rules;
ALTER TABLE alert_rules_v3 RENAME TO alert_rules;

-- Default rule: no successful backup for 26 hours, or a failed backup. It
-- stays frozen while no repository is initialized.
INSERT INTO alert_rules (id, name, type, severity, enabled, is_default, params, created_at, updated_at) VALUES
    ('6f1c2a10-7a4e-4c4b-9a51-000000000007', 'Sauvegarde en retard ou en échec', 'backup', 'warning', 1, 1,
        '{"maxAgeHours":26}', '2026-11-24 00:00:00+00:00', '2026-11-24 00:00:00+00:00');
