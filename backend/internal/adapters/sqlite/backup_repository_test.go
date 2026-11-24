package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"homelab-manager/backend/ent/auditlog"
	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

func TestBackupConfigRoundTrip(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewBackupRepository(database.Client())

	empty, err := repository.BackupConfig(ctx)
	if err != nil || empty.Repository != nil || empty.Schedule != domain.DefaultBackupSchedule || !empty.Selection.ExcludeCaches {
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
		UpdatedAt: &updated, UpdatedBy: "alice",
	}
	audit := domain.AuditEntry{Actor: "alice", TargetType: "backup_config", TargetID: "local", Action: "update", Outcome: domain.AuditSuccess}
	if err := repository.SaveBackupConfig(ctx, config, audit); err != nil {
		t.Fatal(err)
	}
	got, err := repository.BackupConfig(ctx)
	if err != nil || *got.Repository != *config.Repository || got.SealedCredentials != "sealed" || got.Schedule != config.Schedule ||
		len(got.Selection.Sources) != 2 || got.Selection.ExcludeIfPresent != ".nobackup" || got.Selection.ExcludeCaches ||
		got.InitializedAt != nil || got.UpdatedBy != "alice" || !got.UpdatedAt.Equal(updated) {
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
		if err := repository.FinishBackupRun(ctx, run, nil, initialized); err != nil {
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
		{ID: "b", ShortID: "b", Time: start.Add(time.Hour), Paths: []string{"/etc"}, Tags: []string{"homelab-manager"}, TotalFiles: 5},
	}
	finished := start.Add(90 * time.Hour)
	last := domain.BackupRun{ID: uuid.New(), Operation: domain.BackupOpBackup, Origin: domain.BackupManual, StartedAt: start.Add(89 * time.Hour)}
	if err := repository.StartBackupRun(ctx, last, &domain.AuditEntry{Actor: "olivia", TargetType: "backup_run", TargetID: last.ID.String(), Action: "start", Outcome: "success"}); err != nil {
		t.Fatal(err)
	}
	last.State, last.FinishedAt = domain.BackupSucceeded, &finished
	if err := repository.FinishBackupRun(ctx, last, snapshots, nil); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.BackupSnapshots(ctx)
	if err != nil || len(stored) != 2 || stored[0].ID != "b" || stored[0].TotalFiles != 5 || len(stored[1].Tags) != 0 {
		t.Fatalf("snapshots = %+v, %v", stored, err)
	}
	// A new listing replaces the previous one.
	if err := repository.FinishBackupRun(ctx, last, snapshots[:1], nil); err != nil {
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
		if err := repository.FinishBackupRun(ctx, run, nil, nil); err != nil {
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
