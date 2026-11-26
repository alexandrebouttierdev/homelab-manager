package backuprunner

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monoops/backend/internal/domain"
)

// openWAL opens a database in WAL mode, as most applications do.
func openWAL(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+(&url.URL{Path: path}).EscapedPath()+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TABLE notes (body TEXT); INSERT INTO notes VALUES ('committed')"); err != nil {
		t.Fatal(err)
	}
	return db
}

func countRows(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+(&url.URL{Path: path}).EscapedPath()+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM notes").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// TestDumpCopiesOneTransaction: the copy holds what was committed, not a
// transaction in progress, while the application keeps its database open.
func TestDumpCopiesOneTransaction(t *testing.T) {
	dir := t.TempDir()
	database := filepath.Join(dir, "app data", "app.db")
	if err := os.MkdirAll(filepath.Dir(database), 0o700); err != nil {
		t.Fatal(err)
	}
	app := openWAL(t, database)
	tx, err := app.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO notes VALUES ('in progress')"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	root := filepath.Join(dir, "dumps")
	if err := os.MkdirAll(filepath.Join(root, "stale"), 0o700); err != nil {
		t.Fatal(err)
	}
	made, failures := dumpSQLite(context.Background(), root, []string{database})
	if !made || len(failures) != 0 {
		t.Fatalf("dump = %t, %v", made, failures)
	}
	copyPath := domain.BackupDumpPath(root, database)
	if got := countRows(t, copyPath); got != 1 {
		t.Fatalf("the copy holds %d rows, want the committed one", got)
	}
	info, err := os.Stat(copyPath)
	if err != nil || info.Mode().Perm() != dumpMode {
		t.Fatalf("copy mode = %v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(root, "stale")); !os.IsNotExist(err) {
		t.Fatal("an old copy was kept")
	}
}

func TestDumpReportsWhatItCannotCopy(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.db")
	openWAL(t, good)
	text := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(text, []byte("not a database at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.db")
	if err := os.Symlink(good, link); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "dumps")
	made, failures := dumpSQLite(context.Background(), root, []string{good, text, link, filepath.Join(dir, "missing.db")})
	if !made || len(failures) != 3 {
		t.Fatalf("dump = %t, %v", made, failures)
	}
	joined := strings.Join(failures, "\n")
	for _, want := range []string{"notes.txt: not a SQLite database", "link.db: not a regular file", "missing.db: cannot read the file"} {
		if !strings.Contains(joined, want) {
			t.Errorf("failures lack %q: %s", want, joined)
		}
	}
}

func TestBackupAddsTheCopiesAndWarnsOnFailures(t *testing.T) {
	f := newFixture(t)
	f.config.DumpRoot = filepath.Join(t.TempDir(), "dumps")
	database := filepath.Join(t.TempDir(), "app.db")
	openWAL(t, database)
	f.file("backup.out", summaryLine+"\n")
	f.file("snapshots.out", "[]")
	job := backupJob()
	job.RepositoryType, job.Repository = domain.RepositoryLocal, "/mnt/backup"
	job.SQLiteDatabases = []string{database, "/nonexistent/app.db"}
	f.job(job, domain.BackupCredentials{Password: "p"})
	status, result, stderr := f.run("backup")
	if status != 0 || !result.Success || !result.Warning || !strings.Contains(result.Message, "SQLite copy failed for /nonexistent/app.db") {
		t.Fatalf("status %d, result %+v, stderr %s", status, result, stderr)
	}
	args := strings.Split(strings.TrimSpace(f.read("args.backup")), "\n")
	if args[len(args)-1] != f.config.DumpRoot || args[len(args)-2] != "/srv/data" {
		t.Fatalf("args = %q", args)
	}
	entries, err := os.ReadDir(f.config.DumpRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("copies left on the disk: %v, %v", entries, err)
	}
}
