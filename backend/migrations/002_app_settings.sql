-- Settings changed from the interface (singleton row). NULL keeps the value
-- of the service configuration.
CREATE TABLE app_settings (
    id TEXT PRIMARY KEY,
    singleton_key TEXT NOT NULL UNIQUE DEFAULT 'local',
    metrics_retention_days INTEGER CHECK (metrics_retention_days BETWEEN 1 AND 365),
    updated_at DATETIME NOT NULL
);
