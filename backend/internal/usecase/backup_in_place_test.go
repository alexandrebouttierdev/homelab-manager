package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"go.uber.org/zap"
)

// TestInPlaceRestoreNeedsARecentSimulation covers decision D-2: a real
// in-place restore follows a successful simulation of the same paths, at
// most 30 minutes old, used once.
func TestInPlaceRestoreNeedsARecentSimulation(t *testing.T) {
	executor := &fakeBackupExecutor{}
	backups, store := initializedBackups(t, executor)
	ctx := context.Background()
	restore := func(dryRun bool, paths ...string) (domain.BackupRun, error) {
		return backups.RequestOperation(ctx, backupAdmin, domain.BackupOpRestoreInPlace, BackupOperationRequest{
			Restore: &domain.BackupRestore{SnapshotID: "3a253ad8", Include: paths, DryRun: dryRun},
		}, "req")
	}
	if _, err := restore(false, "/srv/app"); !errors.Is(err, domain.ErrBackupSimulationRequired) {
		t.Fatalf("without simulation = %v", err)
	}
	var validation *domain.BackupValidationError
	if _, err := restore(true, "/etc/nginx"); !errors.As(err, &validation) {
		t.Fatalf("protected path = %v", err)
	}
	if _, err := restore(true); !errors.As(err, &validation) {
		t.Fatalf("no path = %v", err)
	}
	simulation, err := restore(true, "/srv/b", "/srv/app")
	if err != nil || simulation.Details == nil || !simulation.Details.InPlace || simulation.Details.Target != "/" {
		t.Fatalf("simulation = %+v, %v", simulation, err)
	}
	finishedRun(t, store, simulation.ID)
	if _, err := restore(false, "/srv/app"); !errors.Is(err, domain.ErrBackupSimulationRequired) {
		t.Fatalf("other paths = %v", err)
	}
	// The same paths in another order.
	real, err := restore(false, "/srv/app", "/srv/b")
	if err != nil {
		t.Fatal(err)
	}
	finishedRun(t, store, real.ID)
	executor.mu.Lock()
	job := executor.jobs[len(executor.jobs)-1]
	executor.mu.Unlock()
	if job.Operation != domain.BackupOpRestoreInPlace || job.Restore.DryRun {
		t.Fatalf("job = %+v", job)
	}
	if _, err := restore(false, "/srv/app", "/srv/b"); !errors.Is(err, domain.ErrBackupSimulationRequired) {
		t.Fatalf("a simulation was used twice: %v", err)
	}

	// A simulation expires.
	old, err := restore(true, "/srv/app")
	if err != nil {
		t.Fatal(err)
	}
	finishedRun(t, store, old.ID)
	backups.mu.Lock()
	for key := range backups.simulations {
		backups.simulations[key] = time.Now().Add(-domain.InPlaceSimulationValidity - time.Minute)
	}
	backups.mu.Unlock()
	if _, err := restore(false, "/srv/app"); !errors.Is(err, domain.ErrBackupSimulationRequired) {
		t.Fatalf("an old simulation = %v", err)
	}
}

func TestFailedSimulationAllowsNothing(t *testing.T) {
	executor := &fakeBackupExecutor{}
	backups, store := initializedBackups(t, executor)
	executor.mu.Lock()
	executor.err = &domain.BackupRunError{Failure: domain.BackupFailureRunner, Err: errors.New("restic")}
	executor.mu.Unlock()
	ctx := context.Background()
	request := BackupOperationRequest{Restore: &domain.BackupRestore{SnapshotID: "3a253ad8", Include: []string{"/srv/app"}, DryRun: true}}
	run, err := backups.RequestOperation(ctx, backupAdmin, domain.BackupOpRestoreInPlace, request, "req")
	if err != nil {
		t.Fatal(err)
	}
	finishedRun(t, store, run.ID)
	request.Restore.DryRun = false
	if _, err := backups.RequestOperation(ctx, backupAdmin, domain.BackupOpRestoreInPlace, request, "req"); !errors.Is(err, domain.ErrBackupSimulationRequired) {
		t.Fatalf("after a failed simulation = %v", err)
	}
}

func TestArchiveOfADumpRun(t *testing.T) {
	executor := &fakeBackupExecutor{}
	backups, store := initializedBackups(t, executor)
	backups.WithArchiveRoot("/archives")
	ctx := context.Background()
	var validation *domain.BackupValidationError
	if _, err := backups.RequestOperation(ctx, backupAdmin, domain.BackupOpDump, BackupOperationRequest{Dump: &domain.BackupDumpRequest{SnapshotID: "3a253ad8", Path: "srv"}}, "req"); !errors.As(err, &validation) {
		t.Fatalf("relative path = %v", err)
	}
	run, err := backups.RequestOperation(ctx, backupAdmin, domain.BackupOpDump, BackupOperationRequest{Dump: &domain.BackupDumpRequest{SnapshotID: "3a253ad8", Path: "/srv/app"}}, "req")
	if err != nil || run.Details == nil || run.Details.ArchivePath != "/srv/app" {
		t.Fatalf("dump = %+v, %v", run, err)
	}
	finishedRun(t, store, run.ID)
	executor.mu.Lock()
	job := executor.jobs[len(executor.jobs)-1]
	executor.mu.Unlock()
	if job.Dump == nil || job.Dump.Path != "/srv/app" {
		t.Fatalf("job = %+v", job)
	}
	path, _, err := backups.Archive(ctx, run.ID)
	if err != nil || path != "/archives/"+run.ID.String()+".tar.gz" {
		t.Fatalf("archive = %q, %v", path, err)
	}
	// A usecase that does not run reads the same store a day later.
	later := NewBackupUsecase(store, executor, fakeBox{}, nil, zap.NewNop())
	later.now = func() time.Time { return time.Now().Add(domain.BackupArchiveRetention + time.Hour) }
	if _, _, err := later.Archive(ctx, run.ID); !errors.Is(err, domain.ErrBackupArchiveUnavailable) {
		t.Fatalf("expired archive = %v", err)
	}
	other := store.sorted()[len(store.sorted())-1]
	if other.Operation == domain.BackupOpDump {
		t.Fatal("the oldest run should be the init")
	}
	if _, _, err := backups.Archive(ctx, other.ID); !errors.Is(err, domain.ErrBackupArchiveUnavailable) {
		t.Fatalf("archive of another run = %v", err)
	}
}
