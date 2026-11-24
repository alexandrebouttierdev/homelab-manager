-- Alert rules and alerts (SPEC_ADMINISTRATION.md § 6.1, § 6.2).
-- params is the JSON object of the rule parameters; default rules are
-- seeded below, can be changed and disabled, not deleted.
CREATE TABLE alert_rules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL
        CHECK (type IN ('host_metric', 'container_metric', 'container_state', 'unit_failed', 'collector')),
    severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    enabled BOOLEAN NOT NULL DEFAULT 1,
    is_default BOOLEAN NOT NULL DEFAULT 0,
    params TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

-- One row per firing of a rule on a target. rule_id is not a foreign key:
-- the history outlives a deleted rule, whose name and type are copied.
-- target is the JSON description of the target; value and threshold are
-- set for metric rules, observed for the others.
CREATE TABLE alerts (
    id TEXT PRIMARY KEY,
    rule_id TEXT NOT NULL,
    rule_name TEXT NOT NULL,
    rule_type TEXT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    target_key TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '{}',
    state TEXT NOT NULL CHECK (state IN ('firing', 'acknowledged', 'resolved')),
    value REAL,
    threshold REAL,
    observed TEXT NOT NULL DEFAULT '',
    fired_at DATETIME NOT NULL,
    acknowledged_at DATETIME,
    acknowledged_by TEXT,
    resolved_at DATETIME,
    resolution TEXT
);
-- At most one open alert per rule and target, even across restarts.
CREATE UNIQUE INDEX alerts_open_rule_target_idx ON alerts (rule_id, target_key) WHERE state <> 'resolved';
CREATE INDEX alerts_fired_idx ON alerts (fired_at DESC);
CREATE INDEX alerts_resolved_idx ON alerts (resolved_at) WHERE resolved_at IS NOT NULL;

-- Default rules (§ 6.1). The temperature threshold is fixed: the collection
-- does not record the thresholds of the sensors (decision Q-6).
INSERT INTO alert_rules (id, name, type, severity, enabled, is_default, params, created_at, updated_at) VALUES
    ('6f1c2a10-7a4e-4c4b-9a51-000000000001', 'Disque presque plein', 'host_metric', 'warning', 1, 1,
        '{"metric":"filesystem.percent","operator":">","threshold":85,"forSeconds":300}',
        '2026-11-24 00:00:00+00:00', '2026-11-24 00:00:00+00:00'),
    ('6f1c2a10-7a4e-4c4b-9a51-000000000002', 'Mémoire disponible faible', 'host_metric', 'warning', 1, 1,
        '{"metric":"memory.available_percent","operator":"<","threshold":10,"forSeconds":300}',
        '2026-11-24 00:00:00+00:00', '2026-11-24 00:00:00+00:00'),
    ('6f1c2a10-7a4e-4c4b-9a51-000000000003', 'Température élevée', 'host_metric', 'critical', 1, 1,
        '{"metric":"temperature.celsius","operator":">","threshold":85,"forSeconds":120}',
        '2026-11-24 00:00:00+00:00', '2026-11-24 00:00:00+00:00'),
    ('6f1c2a10-7a4e-4c4b-9a51-000000000004', 'Collecte système indisponible', 'collector', 'warning', 1, 1,
        '{"collector":"system","forSeconds":120}',
        '2026-11-24 00:00:00+00:00', '2026-11-24 00:00:00+00:00'),
    ('6f1c2a10-7a4e-4c4b-9a51-000000000005', 'Collecte Docker indisponible', 'collector', 'warning', 1, 1,
        '{"collector":"docker","forSeconds":120}',
        '2026-11-24 00:00:00+00:00', '2026-11-24 00:00:00+00:00'),
    ('6f1c2a10-7a4e-4c4b-9a51-000000000006', 'Unité systemd en échec', 'unit_failed', 'critical', 1, 1,
        '{"unitPattern":"*"}',
        '2026-11-24 00:00:00+00:00', '2026-11-24 00:00:00+00:00');
