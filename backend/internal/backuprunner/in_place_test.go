package backuprunner

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"homelab-manager/backend/internal/backupjob"
	"homelab-manager/backend/internal/domain"
)

func inPlaceJob(dryRun bool, include ...string) backupjob.Job {
	return backupjob.Job{
		Version: domain.BackupJobVersion, Operation: domain.BackupOpRestoreInPlace,
		RepositoryType: domain.RepositoryLocal, Repository: "/mnt/backup",
		Sources: []string{}, Excludes: []string{}, SQLiteDatabases: []string{},
		Restore: &backupjob.Restore{SnapshotID: "3a253ad8", Include: include, DryRun: dryRun, Target: "/"},
	}
}

const restoreSummary = `{"message_type":"summary","total_files":2,"files_restored":1,"total_bytes":20,"bytes_restored":10}`

func TestInPlaceRestoreBacksUpWhatItReplaces(t *testing.T) {
	f := newFixture(t)
	f.config.InPlaceRoot = t.TempDir()
	if err := os.MkdirAll(filepath.Join(f.config.InPlaceRoot, "srv", "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	f.file("backup.out", summaryLine+"\n")
	f.file("restore.out", restoreSummary+"\n")
	f.file("snapshots.out", snapshotsJSON)
	f.job(inPlaceJob(false, "/srv/app", "/srv/gone"), domain.BackupCredentials{Password: "p"})
	status, result, stderr := f.run("restore-in-place")
	if status != 0 || !result.Success || !result.Details.InPlace || result.Details.SafetySnapshotID != "abcdef0123456789" || result.Details.FilesRestored != 1 {
		t.Fatalf("status %d, result %+v, stderr %s", status, result, stderr)
	}
	backupArgs := strings.TrimSpace(f.read("args.backup"))
	if backupArgs != "backup\n--json\n--tag\nhomelab-manager\n--tag\nbefore-restore\n--\n"+filepath.Join(f.config.InPlaceRoot, "srv", "app") {
		t.Fatalf("safety backup args = %q (an absent path is not backed up)", backupArgs)
	}
	want := "restore\n--json\n--target\n" + f.config.InPlaceRoot + "\n--overwrite\nif-changed\n--include\n/srv/app\n--include\n/srv/gone\n--\n3a253ad8"
	if got := strings.TrimSpace(f.read("args.restore")); got != want {
		t.Fatalf("restore args = %q, want %q", got, want)
	}
	if strings.Contains(f.read("args.restore"), "--delete") {
		t.Fatal("an in-place restore never deletes")
	}
	if len(result.Snapshots) != 2 {
		t.Fatalf("the snapshots are not listed after the restore: %+v", result.Snapshots)
	}
}

func TestInPlaceRestoreStopsWithoutItsSafetyBackup(t *testing.T) {
	f := newFixture(t)
	f.config.InPlaceRoot = t.TempDir()
	if err := os.MkdirAll(filepath.Join(f.config.InPlaceRoot, "srv"), 0o700); err != nil {
		t.Fatal(err)
	}
	f.file("backup.err", "Fatal: repository is locked\n")
	f.file("backup.code", "1")
	f.job(inPlaceJob(false, "/srv"), domain.BackupCredentials{Password: "p"})
	status, result, _ := f.run("restore-in-place")
	if status != 1 || result.Success || !strings.Contains(result.Message, "nothing was restored") {
		t.Fatalf("status %d, result %+v", status, result)
	}
	if _, err := os.Stat(filepath.Join(f.bin, "args.restore")); !os.IsNotExist(err) {
		t.Fatal("restic restored without a safety backup")
	}
}

func TestInPlaceSimulationChangesNothing(t *testing.T) {
	f := newFixture(t)
	f.config.InPlaceRoot = t.TempDir()
	f.file("restore.out", restoreSummary+"\n")
	f.job(inPlaceJob(true, "/srv/app"), domain.BackupCredentials{Password: "p"})
	if status, result, _ := f.run("restore-in-place"); status != 0 || !result.Details.DryRun {
		t.Fatalf("simulation = %d %+v", status, result)
	}
	if !strings.Contains(f.read("args.restore"), "--dry-run") {
		t.Fatal("dry run flag missing")
	}
	if _, err := os.Stat(filepath.Join(f.bin, "args.backup")); !os.IsNotExist(err) {
		t.Fatal("a simulation made a safety backup")
	}
}

func TestInPlaceRestoreRefusesProtectedPaths(t *testing.T) {
	for _, include := range []string{"/", "/etc/nginx", "/usr", "/var/lib/homelab-manager", "/var"} {
		f := newFixture(t)
		f.config.InPlaceRoot = t.TempDir()
		f.job(inPlaceJob(false, include), domain.BackupCredentials{Password: "p"})
		if status, _, _ := f.run("restore-in-place"); status != 1 {
			t.Fatalf("%s accepted", include)
		}
		if _, err := os.Stat(filepath.Join(f.bin, "args.restore")); !os.IsNotExist(err) {
			t.Fatalf("restic ran for %s", include)
		}
	}
}

const archiveRunID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"

func dumpJob(path string) backupjob.Job {
	return backupjob.Job{
		Version: domain.BackupJobVersion, Operation: domain.BackupOpDump,
		RepositoryType: domain.RepositoryLocal, Repository: "/mnt/backup",
		Sources: []string{}, Excludes: []string{}, SQLiteDatabases: []string{},
		Dump: &backupjob.Dump{SnapshotID: "3a253ad8", Path: path, Target: domain.BackupArchiveRoot + "/" + archiveRunID + ".tar.gz"},
	}
}

func TestDumpWritesACompressedArchive(t *testing.T) {
	f := newFixture(t)
	f.config.ArchiveRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(f.config.ArchiveRoot, "old.tar.gz"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	var tarball strings.Builder
	writer := tar.NewWriter(&tarball)
	_ = writer.WriteHeader(&tar.Header{Name: "srv/app/notes.txt", Mode: 0o600, Size: 5})
	_, _ = writer.Write([]byte("hello"))
	_ = writer.Close()
	f.file("dump.out", tarball.String())
	f.job(dumpJob("/srv/app"), domain.BackupCredentials{Password: "p"})
	status, result, stderr := f.run("dump")
	if status != 0 || !result.Success || result.Details.ArchivePath != "/srv/app" || result.Details.ArchiveBytes == 0 {
		t.Fatalf("status %d, result %+v, stderr %s", status, result, stderr)
	}
	if got := strings.TrimSpace(f.read("args.dump")); got != "dump\n--archive\ntar\n--\n3a253ad8\n/srv/app" {
		t.Fatalf("args = %q", got)
	}
	archive := filepath.Join(f.config.ArchiveRoot, archiveRunID+".tar.gz")
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, _ := file.Stat()
	if info.Mode().Perm() != archiveMode || info.Size() != result.Details.ArchiveBytes {
		t.Fatalf("archive = %v, %d bytes", info.Mode(), info.Size())
	}
	unzipped, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(unzipped)
	header, err := reader.Next()
	content, _ := io.ReadAll(reader)
	if err != nil || header.Name != "srv/app/notes.txt" || string(content) != "hello" {
		t.Fatalf("archive content = %v %v %q", header, err, content)
	}
	if _, err := os.Stat(filepath.Join(f.config.ArchiveRoot, "old.tar.gz")); !os.IsNotExist(err) {
		t.Fatal("an old archive was kept")
	}
}

func TestDumpRemovesAFailedArchive(t *testing.T) {
	f := newFixture(t)
	f.config.ArchiveRoot = t.TempDir()
	f.file("dump.err", "Fatal: path /srv/none not found in snapshot\n")
	f.file("dump.code", "1")
	f.job(dumpJob("/srv/none"), domain.BackupCredentials{Password: "p"})
	if status, result, _ := f.run("dump"); status != 1 || !strings.Contains(result.Message, "not found") {
		t.Fatalf("status %d, result %+v", status, result)
	}
	if entries, _ := os.ReadDir(f.config.ArchiveRoot); len(entries) != 0 {
		t.Fatalf("a failed archive was kept: %v", entries)
	}
}

func TestLimitedWriterStopsAtItsBound(t *testing.T) {
	var sink strings.Builder
	writer := &limitedWriter{writer: &sink, room: 4}
	if _, err := writer.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("e")); err == nil || !writer.exceeded {
		t.Fatal("the bound was not enforced")
	}
}
