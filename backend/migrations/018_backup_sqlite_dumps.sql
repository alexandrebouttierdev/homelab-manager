-- Backups, consistent copies of SQLite databases (decision D-3): the files
-- monoops-backup-runner copies with VACUUM INTO before each backup, a JSON
-- array of absolute paths.
ALTER TABLE backup_configs ADD COLUMN sqlite_databases TEXT NOT NULL DEFAULT '[]';
