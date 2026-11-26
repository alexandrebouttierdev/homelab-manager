package backuprunner

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"monoops/backend/internal/domain"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver registered as "sqlite".
)

const (
	// dumpTimeout bounds the copy of one database.
	dumpTimeout = 15 * time.Minute
	// dumpBusyTimeout lets a copy wait for a writer holding a lock.
	dumpBusyTimeout = 30 * time.Second
	dumpMode        = 0o600
)

// sqliteHeader starts every SQLite database file.
var sqliteHeader = []byte("SQLite format 3\x00")

// dumpSQLite copies each database consistently into root, at its own path
// below it (decision D-3): VACUUM INTO reads one transaction of the
// database, whatever its journal mode, while the applications keep
// writing. Each copy is then checked. It returns whether any copy was made
// and a message per database that could not be copied; a failure does not
// stop the backup, which then ends with a warning.
func dumpSQLite(ctx context.Context, root string, databases []string) (bool, []string) {
	if err := clearDumps(root); err != nil {
		return false, []string{fmt.Sprintf("%s: %v", root, err)}
	}
	made := false
	var failures []string
	for _, database := range databases {
		if err := dumpOne(ctx, root, database); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", database, err))
			continue
		}
		made = true
	}
	return made, failures
}

// clearDumps empties root, keeping the directory itself.
func clearDumps(root string) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(root, 0o700)
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func dumpOne(ctx context.Context, root, database string) error {
	info, err := os.Lstat(database)
	if err != nil {
		return fmt.Errorf("cannot read the file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file (a symbolic link is not followed)")
	}
	if err := checkHeader(database); err != nil {
		return err
	}
	target := domain.BackupDumpPath(root, database)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create the copy directory: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, dumpTimeout)
	defer cancel()
	// Read-only: the copy never writes to the database itself.
	source := "file:" + (&url.URL{Path: database}).EscapedPath() + "?mode=ro" +
		fmt.Sprintf("&_pragma=busy_timeout(%d)", dumpBusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", source)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", target); err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("copy: %s", sqliteMessage(err))
	}
	if err := os.Chmod(target, dumpMode); err != nil {
		return fmt.Errorf("protect the copy: %w", err)
	}
	return checkCopy(ctx, target)
}

func checkHeader(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read the file: %w", err)
	}
	defer func() { _ = file.Close() }()
	header := make([]byte, len(sqliteHeader))
	if _, err := io.ReadFull(file, header); err != nil || !bytes.Equal(header, sqliteHeader) {
		return errors.New("not a SQLite database")
	}
	return nil
}

// checkCopy runs quick_check on the copy, which nothing else opens.
func checkCopy(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+(&url.URL{Path: path}).EscapedPath()+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open the copy: %w", err)
	}
	defer func() { _ = db.Close() }()
	var verdict string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&verdict); err != nil {
		return fmt.Errorf("check the copy: %s", sqliteMessage(err))
	}
	if verdict != "ok" {
		_ = os.Remove(path)
		return fmt.Errorf("the copy fails quick_check: %s", firstLine(verdict))
	}
	return nil
}

// sqliteMessage keeps the first line of a driver error, bounded.
func sqliteMessage(err error) string {
	return firstLine(err.Error())
}

func firstLine(text string) string {
	text, _, _ = strings.Cut(text, "\n")
	if len(text) > 300 {
		text = text[:300]
	}
	return text
}
