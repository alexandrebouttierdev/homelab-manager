package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"monoops/backend/ent"
	"monoops/backend/ent/auditlog"
	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"
	"monoops/backend/migrations"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

func TestBackupConfigRoundTrip(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewBackupRepository(database.Client())

	empty, err := repository.BackupConfig(ctx)
	if err != nil || empty.Repository != nil || empty.Schedule != domain.DefaultBackupSchedule || !empty.Selection.ExcludeCaches ||
		empty.Retention != domain.DefaultBackupRetention || empty.Check != domain.DefaultBackupCheck {
		t.Fatalf("default = %+v, %v", empty, err)
	}
	updated := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	config := domain.BackupConfig{
		Repository:        &domain.BackupRepository{Type: domain.RepositoryS3, Location: "https://s3.example.com/bucket"},
		SealedCredentials: "sealed",
		Selection: domain.BackupSelection{
			Sources: []string{"/etc", "/srv"}, Excludes: []string{"*.tmp"}, ExcludeCaches: false, ExcludeIfPresent: ".nobackup",
		},
		Schedule:  domain.BackupSchedule{Enabled: true, Frequency: domain.BackupWeekly, Weekday: time.Sunday, Hour: 4, Minute: 15},
		Retention: domain.BackupRetention{Enabled: true, KeepDaily: 14, KeepYearly: 2},
		Check:     domain.BackupCheck{Enabled: false, SubsetPercent: 10},
		UpdatedAt: &updated, UpdatedBy: "alice",
	}
	audit := domain.AuditEntry{Actor: "alice", TargetType: "backup_config", TargetID: "local", Action: "update", Outcome: domain.AuditSuccess}
	if err := repository.SaveBackupConfig(ctx, config, audit); err != nil {
		t.Fatal(err)
	}
	got, err := repository.BackupConfig(ctx)
	if err != nil || *got.Repository != *config.Repository || got.SealedCredentials != "sealed" || got.Schedule != config.Schedule ||
		len(got.Selection.Sources) != 2 || got.Selection.ExcludeIfPresent != ".nobackup" || got.Selection.ExcludeCaches ||
		got.InitializedAt != nil || got.UpdatedBy != "alice" || !got.UpdatedAt.Equal(updated) ||
		got.Retention != config.Retention || got.Check != config.Check {
		t.Fatalf("stored = %+v, %v", got, err)
	}
	// A second save updates the singleton and clears the repository.
	config.Repository, config.SealedCredentials = nil, ""
	if err := repository.SaveBackupConfig(ctx, config, audit); err != nil {
		t.Fatal(err)
	}
	if got, _ := repository.BackupConfig(ctx); got.Repository != nil || got.SealedCredentials != "" {
		t.Fatalf("cleared = %+v", got)
	}
	if count, _ := database.Client().BackupConfig.Query().Count(ctx); count != 1 {
		t.Fatalf("%d configuration rows, want 1", count)
	}
	if count, _ := database.Client().AuditLog.Query().Where(auditlog.TargetType("backup_config")).Count(ctx); count != 2 {
		t.Fatalf("%d audit entries, want 2", count)
	}
}

func TestBackupRunsAndSnapshots(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewBackupRepository(database.Client())
	if err := repository.SaveBackupConfig(ctx, domain.BackupConfig{
		Repository: &domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/backup"}, SealedCredentials: "s", Schedule: domain.DefaultBackupSchedule,
	}, domain.AuditEntry{Actor: "a", TargetType: "backup_config", TargetID: "local", Action: "update", Outcome: "success"}); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC)
	record := func(op domain.BackupOperation, origin domain.BackupOrigin, state domain.BackupRunState, at time.Time) domain.BackupRun {
		t.Helper()
		run := domain.BackupRun{ID: uuid.New(), Operation: op, Origin: origin, StartedAt: at}
		if err := repository.StartBackupRun(ctx, run, nil); err != nil {
			t.Fatal(err)
		}
		if state == domain.BackupRunning {
			return run
		}
		finished := at.Add(time.Minute)
		run.State, run.FinishedAt = state, &finished
		if state == domain.BackupFailed {
			run.Failure, run.Message = domain.BackupFailureRestic, "Fatal: wrong password"
		} else if op == domain.BackupOpBackup {
			run.SnapshotID = "snap-" + at.Format("02")
			run.Stats = &domain.BackupStats{FilesNew: 3, DataAdded: 2048, TotalFilesProcessed: 14, DurationSeconds: 1.5}
		}
		var initialized *time.Time
		if op == domain.BackupOpInit && state == domain.BackupSucceeded {
			initialized = &finished
		}
		if err := repository.FinishBackupRun(ctx, run, ports.BackupFinish{InitializedAt: initialized}); err != nil {
			t.Fatal(err)
		}
		return run
	}
	record(domain.BackupOpInit, domain.BackupManual, domain.BackupSucceeded, start)
	success := record(domain.BackupOpBackup, domain.BackupScheduled, domain.BackupSucceeded, start.Add(24*time.Hour))
	failed := record(domain.BackupOpBackup, domain.BackupManual, domain.BackupFailed, start.Add(48*time.Hour))
	running := record(domain.BackupOpBackup, domain.BackupManual, domain.BackupRunning, start.Add(72*time.Hour))

	config, _ := repository.BackupConfig(ctx)
	if config.InitializedAt == nil || !config.InitializedAt.Equal(start.Add(time.Minute)) {
		t.Fatalf("initialized at = %v", config.InitializedAt)
	}
	latest, err := repository.LatestBackupRuns(ctx)
	if err != nil || latest.Running.ID != running.ID || latest.LastBackup.ID != running.ID ||
		latest.LastSuccess.ID != success.ID || latest.LastScheduled.ID != success.ID {
		t.Fatalf("latest = %+v, %v", latest, err)
	}
	if latest.LastSuccess.Stats == nil || latest.LastSuccess.Stats.DataAdded != 2048 || latest.LastSuccess.SnapshotID != "snap-21" {
		t.Fatalf("stats = %+v", latest.LastSuccess)
	}
	got, err := repository.BackupRun(ctx, failed.ID)
	if err != nil || got.Failure != domain.BackupFailureRestic || got.Message != "Fatal: wrong password" || got.Stats != nil {
		t.Fatalf("failed run = %+v, %v", got, err)
	}
	if _, err := repository.BackupRun(ctx, uuid.New()); !errors.Is(err, domain.ErrBackupRunNotFound) {
		t.Fatalf("missing run = %v", err)
	}
	runs, err := repository.BackupRuns(ctx, domain.BackupRunFilter{Limit: 2})
	if err != nil || len(runs) != 2 || runs[0].ID != running.ID {
		t.Fatalf("runs = %+v, %v", runs, err)
	}

	if err := repository.InterruptRunningBackupRuns(ctx, start.Add(80*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := repository.BackupRun(ctx, running.ID); got.State != domain.BackupFailed || got.Failure != domain.BackupFailureInterrupted {
		t.Fatalf("interrupted = %+v", got)
	}

	snapshots := []domain.BackupSnapshot{
		{ID: "a", ShortID: "a", Time: start, Paths: []string{"/etc"}},
		{ID: "b", ShortID: "b", Time: start.Add(time.Hour), Paths: []string{"/etc"}, Tags: []string{"monoops"}, TotalFiles: 5},
	}
	finished := start.Add(90 * time.Hour)
	last := domain.BackupRun{ID: uuid.New(), Operation: domain.BackupOpBackup, Origin: domain.BackupManual, StartedAt: start.Add(89 * time.Hour)}
	if err := repository.StartBackupRun(ctx, last, &domain.AuditEntry{Actor: "olivia", TargetType: "backup_run", TargetID: last.ID.String(), Action: "start", Outcome: "success"}); err != nil {
		t.Fatal(err)
	}
	last.State, last.FinishedAt = domain.BackupSucceeded, &finished
	if err := repository.FinishBackupRun(ctx, last, ports.BackupFinish{Snapshots: snapshots}); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.BackupSnapshots(ctx)
	if err != nil || len(stored) != 2 || stored[0].ID != "b" || stored[0].TotalFiles != 5 || len(stored[1].Tags) != 0 {
		t.Fatalf("snapshots = %+v, %v", stored, err)
	}
	// A new listing replaces the previous one.
	if err := repository.FinishBackupRun(ctx, last, ports.BackupFinish{Snapshots: snapshots[:1]}); err != nil {
		t.Fatal(err)
	}
	if stored, _ := repository.BackupSnapshots(ctx); len(stored) != 1 {
		t.Fatalf("replaced snapshots = %+v", stored)
	}

	if err := repository.PurgeBackupRuns(ctx, start.Add(50*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if count, _ := database.Client().BackupRun.Query().Count(ctx); count != 2 {
		t.Fatalf("%d runs after the purge, want 2", count)
	}
}

func TestBackupAlertStatus(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewBackupRepository(database.Client())
	if _, known, err := repository.BackupAlertStatus(ctx); err != nil || known {
		t.Fatalf("without repository: known %v, %v", known, err)
	}
	initialized := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	audit := domain.AuditEntry{Actor: "a", TargetType: "backup_config", TargetID: "local", Action: "update", Outcome: "success"}
	if err := repository.SaveBackupConfig(ctx, domain.BackupConfig{
		Repository: &domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/backup"}, SealedCredentials: "s",
		InitializedAt: &initialized, Schedule: domain.DefaultBackupSchedule,
	}, audit); err != nil {
		t.Fatal(err)
	}
	status, known, err := repository.BackupAlertStatus(ctx)
	if err != nil || !known || !status.Reference.Equal(initialized) || status.LastRunFailed {
		t.Fatalf("no backup yet = %+v, %v, %v", status, known, err)
	}
	finish := func(state domain.BackupRunState, at time.Time) {
		run := domain.BackupRun{ID: uuid.New(), Operation: domain.BackupOpBackup, Origin: domain.BackupScheduled, StartedAt: at}
		if err := repository.StartBackupRun(ctx, run, nil); err != nil {
			t.Fatal(err)
		}
		end := at.Add(time.Minute)
		run.State, run.FinishedAt = state, &end
		if err := repository.FinishBackupRun(ctx, run, ports.BackupFinish{}); err != nil {
			t.Fatal(err)
		}
	}
	finish(domain.BackupSucceeded, initialized.Add(24*time.Hour))
	finish(domain.BackupFailed, initialized.Add(48*time.Hour))
	status, _, _ = repository.BackupAlertStatus(ctx)
	if !status.Reference.Equal(initialized.Add(24*time.Hour+time.Minute)) || !status.LastRunFailed {
		t.Fatalf("after a failure = %+v", status)
	}
}

func TestBackupDefaultAlertRule(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	rules, err := NewAlertRepository(database.Client()).AlertRules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range rules {
		if rule.Type == domain.AlertRuleBackup {
			if !rule.Default || !rule.Enabled || rule.Params.MaxAgeHours != 26 || rule.Validate() != nil {
				t.Fatalf("default backup rule = %+v", rule)
			}
			return
		}
	}
	t.Fatal("no default backup rule")
}

func TestBackupRunDetailsAndLastCheck(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewBackupRepository(database.Client())
	at := time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		op      domain.BackupOperation
		state   domain.BackupRunState
		details *domain.BackupRunDetails
	}{
		{domain.BackupOpCheck, domain.BackupSucceeded, &domain.BackupRunDetails{SubsetPercent: 5}},
		{domain.BackupOpCheck, domain.BackupFailed, &domain.BackupRunDetails{SubsetPercent: 5}},
		{domain.BackupOpRestore, domain.BackupSucceeded, &domain.BackupRunDetails{SnapshotID: "3a253ad8", Target: "/var/lib/monoops-backup/restore/x", FilesRestored: 3}},
	} {
		at = at.Add(time.Hour)
		run := domain.BackupRun{ID: uuid.New(), Operation: tc.op, Origin: domain.BackupManual, StartedAt: at}
		if err := repository.StartBackupRun(ctx, run, nil); err != nil {
			t.Fatal(err)
		}
		end := at.Add(time.Minute)
		run.State, run.FinishedAt, run.Details = tc.state, &end, tc.details
		if err := repository.FinishBackupRun(ctx, run, ports.BackupFinish{}); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := repository.LatestBackupRuns(ctx)
	if err != nil || latest.LastCheck == nil || latest.LastCheck.State != domain.BackupSucceeded || latest.LastBackup != nil {
		t.Fatalf("latest = %+v, %v", latest, err)
	}
	runs, _ := repository.BackupRuns(ctx, domain.BackupRunFilter{Limit: 1})
	if runs[0].Details == nil || runs[0].Details.FilesRestored != 3 || runs[0].Details.SnapshotID != "3a253ad8" {
		t.Fatalf("restore details = %+v", runs[0].Details)
	}
}

func TestBackupCopyRepositoryAndRuns(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewBackupRepository(database.Client())
	audit := domain.AuditEntry{Actor: "alice", TargetType: "backup_config", TargetID: "local", Action: "update_copy", Outcome: domain.AuditSuccess}
	initialized := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	config := domain.BackupConfig{
		Repository: &domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/srv/backup"}, InitializedAt: &initialized,
		SealedCredentials: "sealed", Schedule: domain.DefaultBackupSchedule, Check: domain.DefaultBackupCheck,
		Copy: &domain.BackupCopyTarget{
			Repository:        domain.BackupRepository{Type: domain.RepositoryRest, Location: "https://nas.lan:8000/copy"},
			SealedCredentials: "copy-sealed", AfterBackup: true,
		},
	}
	if err := repository.SaveBackupConfig(ctx, config, audit); err != nil {
		t.Fatal(err)
	}
	got, err := repository.BackupConfig(ctx)
	if err != nil || got.Copy == nil || got.Copy.Repository != config.Copy.Repository || got.Copy.SealedCredentials != "copy-sealed" ||
		!got.Copy.AfterBackup || got.Copy.InitializedAt != nil {
		t.Fatalf("copy = %+v, %v", got.Copy, err)
	}

	// The diff and copy operations are accepted by the rebuilt table, and
	// a successful copy marks the copy repository initialized.
	started := time.Date(2026, 11, 25, 3, 0, 0, 0, time.UTC)
	diff := domain.BackupRun{ID: uuid.New(), Operation: domain.BackupOpDiff, Origin: domain.BackupManual, RequestedBy: "alice", StartedAt: started}
	copyRun := domain.BackupRun{ID: uuid.New(), Operation: domain.BackupOpCopy, Origin: domain.BackupScheduled, StartedAt: started.Add(time.Minute)}
	for _, run := range []domain.BackupRun{diff, copyRun} {
		if err := repository.StartBackupRun(ctx, run, nil); err != nil {
			t.Fatalf("start %s: %v", run.Operation, err)
		}
	}
	finished := started.Add(2 * time.Minute)
	diff.State, diff.FinishedAt = domain.BackupSucceeded, &finished
	diff.Details = &domain.BackupRunDetails{Diff: &domain.BackupDiff{
		From: "3a253ad8", To: "9f2c11b0", ChangedFiles: 1, TotalChanges: 1,
		Changes: []domain.BackupDiffChange{{Path: "/etc/hosts", Modifier: "M"}},
	}}
	copyRun.State, copyRun.FinishedAt = domain.BackupSucceeded, &finished
	copyRun.Details = &domain.BackupRunDetails{Copy: &domain.BackupCopyResult{Repository: config.Copy.Repository, Initialized: true, SnapshotsCopied: 3, SnapshotsInTarget: 3}}
	if err := repository.FinishBackupRun(ctx, diff, ports.BackupFinish{}); err != nil {
		t.Fatal(err)
	}
	if err := repository.FinishBackupRun(ctx, copyRun, ports.BackupFinish{CopyInitializedAt: &finished}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repository.BackupConfig(ctx); got.Copy.InitializedAt == nil || !got.Copy.InitializedAt.Equal(finished) {
		t.Fatalf("copy initialized = %+v", got.Copy)
	}
	stored, err := repository.BackupRun(ctx, diff.ID)
	if err != nil || stored.Details.Diff == nil || stored.Details.Diff.Changes[0].Path != "/etc/hosts" {
		t.Fatalf("diff run = %+v, %v", stored, err)
	}
	latest, err := repository.LatestBackupRuns(ctx)
	if err != nil || latest.LastCopy == nil || latest.LastCopy.ID != copyRun.ID || latest.LastCopy.Details.Copy.SnapshotsCopied != 3 {
		t.Fatalf("latest copy = %+v, %v", latest.LastCopy, err)
	}

	// Removing the copy repository clears it; a late copy outcome does not
	// bring it back.
	config.Copy = nil
	if err := repository.SaveBackupConfig(ctx, config, audit); err != nil {
		t.Fatal(err)
	}
	late := domain.BackupRun{ID: uuid.New(), Operation: domain.BackupOpCopy, Origin: domain.BackupManual, StartedAt: finished}
	if err := repository.StartBackupRun(ctx, late, nil); err != nil {
		t.Fatal(err)
	}
	late.State, late.FinishedAt = domain.BackupSucceeded, &finished
	if err := repository.FinishBackupRun(ctx, late, ports.BackupFinish{CopyInitializedAt: &finished}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repository.BackupConfig(ctx); got.Copy != nil {
		t.Fatalf("removed copy came back: %+v", got.Copy)
	}
}

func TestBackupSFTPRepositoriesAndSSHKey(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewBackupRepository(database.Client())
	audit := domain.AuditEntry{Actor: "alice", TargetType: "backup_config", TargetID: "local", Action: "update", Outcome: domain.AuditSuccess}
	created := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	config := domain.BackupConfig{
		Repository: &domain.BackupRepository{
			Type: domain.RepositorySFTP, Location: "sftp://backup@nas.lan:2222/srv/restic", HostKey: "ssh-ed25519 AAAAmain",
		},
		Schedule: domain.DefaultBackupSchedule, Check: domain.DefaultBackupCheck,
		Copy: &domain.BackupCopyTarget{Repository: domain.BackupRepository{
			Type: domain.RepositorySFTP, Location: "sftp://copy@offsite.example.org/srv/copy", HostKey: "ssh-ed25519 AAAAcopy",
		}},
		SSHKey: &domain.BackupSSHKey{PublicKey: "ssh-ed25519 AAAAmanager monoops-backup", SealedPrivateKey: "sealed-key", CreatedAt: created},
	}
	if err := repository.SaveBackupConfig(ctx, config, audit); err != nil {
		t.Fatalf("the rebuilt table refuses sftp: %v", err)
	}
	got, err := repository.BackupConfig(ctx)
	if err != nil || *got.Repository != *config.Repository || got.Copy.Repository != config.Copy.Repository ||
		got.SSHKey == nil || *got.SSHKey != *config.SSHKey {
		t.Fatalf("config = %+v / %+v / %+v, %v", got.Repository, got.Copy, got.SSHKey, err)
	}
	config.Repository, config.Copy, config.SSHKey = &domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/srv/b"}, nil, nil
	if err := repository.SaveBackupConfig(ctx, config, audit); err != nil {
		t.Fatal(err)
	}
	got, err = repository.BackupConfig(ctx)
	if err != nil || got.Repository.HostKey != "" || got.Copy != nil || got.SSHKey != nil {
		t.Fatalf("cleared config = %+v / %+v / %+v, %v", got.Repository, got.Copy, got.SSHKey, err)
	}
}

// TestBackupSFTPMigrationKeepsTheConfiguration applies the migrations up to
// 015, stores a configuration, then applies 016, which rebuilds the table.
func TestBackupSFTPMigrationKeepsTheConfiguration(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open(driverName, dsn(filepath.Join(t.TempDir(), "manager.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	files, err := migrations.Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if name := file.Name(); strings.HasSuffix(name, ".sql") && name < "016" {
			if err := applyMigration(ctx, db, name); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO backup_configs (id, repository_type, repository_location, sealed_credentials,
		sources, keep_daily, copy_repository_type, copy_repository_location, copy_after_backup, updated_at, updated_by)
		VALUES ('7d4c1a52-6c8e-4d55-9a3e-0f8d1b2c3d4e', 'rest', 'https://nas.lan:8000/r', 'sealed', '["/etc"]', 9,
		's3', 'https://s3.example.com/b', 1, '2026-11-25 08:00:00', 'alice')`); err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, db); err != nil {
		t.Fatalf("016: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	config, err := NewBackupRepository(client).BackupConfig(ctx)
	if err != nil || config.Repository == nil || config.Repository.Location != "https://nas.lan:8000/r" ||
		config.SealedCredentials != "sealed" || config.Retention.KeepDaily != 9 || config.Copy == nil ||
		!config.Copy.AfterBackup || config.UpdatedBy != "alice" || len(config.Selection.Sources) != 1 || config.SSHKey != nil {
		t.Fatalf("config after 016 = %+v, %v", config, err)
	}
}
