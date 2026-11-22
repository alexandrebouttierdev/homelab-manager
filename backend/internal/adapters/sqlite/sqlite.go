// Package sqlite is the persistence adapter of the Manager. It stores users,
// sessions, the host inventory, metrics and events in a single local SQLite
// file accessed through Ent.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"homelab-manager/backend/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite" // Pure-Go SQLite driver registered as "sqlite".
)

const (
	driverName  = "sqlite"
	pingTimeout = 2 * time.Second
	// busyTimeout bounds how long a statement waits for a lock held by
	// another process (for example an operator running a backup).
	busyTimeout = 5 * time.Second
)

// Database owns the SQLite connection pool and the Ent client built on it.
type Database struct {
	db     *sql.DB
	client *ent.Client
}

// Open creates the database file if needed (mode 0600, parent directory 0750),
// verifies the connection and applies pending migrations before returning an
// Ent client. The caller must call Close.
func Open(ctx context.Context, path string) (*Database, error) {
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if err := prepareFile(path); err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	// A single connection serialises access inside the Manager: writes are
	// small and periodic, and it rules out SQLITE_BUSY between collectors,
	// purge and HTTP readers. Transactions must therefore only use their tx.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	err = db.PingContext(pingCtx)
	cancel()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to SQLite database: %w", err)
	}
	if err := applyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	return &Database{db: db, client: client}, nil
}

// Client returns the Ent client for the repositories of this package.
func (d *Database) Client() *ent.Client { return d.client }

// Close releases the Ent client and the underlying connection.
func (d *Database) Close() error { return d.client.Close() }

// dsn builds the modernc.org/sqlite connection string. Times are written in
// UTC with a sortable layout so that string comparisons in SQL are
// chronological; foreign keys are enforced; WAL keeps readers unblocked by
// the writer; immediate transactions avoid lock upgrades mid-transaction.
func dsn(path string) string {
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "synchronous(NORMAL)")
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeout.Milliseconds()))
	query.Set("_time_format", "sqlite")
	query.Set("_timezone", "UTC")
	query.Set("_txlock", "immediate")
	return (&url.URL{Scheme: "file", OmitHost: true, Path: path, RawQuery: query.Encode()}).String()
}

// prepareFile creates the parent directory and an empty database file with
// restrictive permissions, so SQLite (which reuses the main file's mode for
// its -wal and -shm files) never exposes password or session hashes.
func prepareFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("create database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close database file: %w", err)
	}
	return nil
}
