-- Scheduled maintenance tasks of a fixed catalog (decision D-1): the
-- Manager runs them through units shipped with the project; an
-- administrator only sets their schedule and their typed parameters.
CREATE TABLE maintenance_tasks (
    id TEXT PRIMARY KEY CHECK (id IN ('journal-vacuum')),
    enabled BOOLEAN NOT NULL DEFAULT 0,
    frequency TEXT NOT NULL DEFAULT 'weekly' CHECK (frequency IN ('daily', 'weekly')),
    weekday INTEGER NOT NULL DEFAULT 0 CHECK (weekday BETWEEN 0 AND 6),
    hour INTEGER NOT NULL DEFAULT 4 CHECK (hour BETWEEN 0 AND 23),
    minute INTEGER NOT NULL DEFAULT 0 CHECK (minute BETWEEN 0 AND 59),
    max_size_mib INTEGER NOT NULL DEFAULT 512 CHECK (max_size_mib IN (256, 512, 1024, 2048)),
    last_run_at DATETIME,
    last_succeeded BOOLEAN,
    last_message TEXT NOT NULL DEFAULT '',
    last_origin TEXT NOT NULL DEFAULT '',
    updated_at DATETIME,
    updated_by TEXT NOT NULL DEFAULT ''
);
