-- Units recorded in the failed state by the system services collection
-- (SPEC_ADMINISTRATION.md § 5.1). The id is the unit name. Kept apart from
-- the events, which the retention purges: a failure is reported once, even
-- after a restart of the Manager or the purge of its event.
CREATE TABLE failed_units (
    id TEXT PRIMARY KEY,
    failed_at DATETIME NOT NULL
);
