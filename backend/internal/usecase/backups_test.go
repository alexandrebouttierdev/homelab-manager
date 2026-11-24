package usecase

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// memoryBackupStore is an in-memory ports.BackupStore.
type memoryBackupStore struct {
	mu        sync.Mutex
	config    domain.BackupConfig
	runs      map[uuid.UUID]domain.BackupRun
	snapshots []domain.BackupSnapshot
	audits    []domain.AuditEntry
}

func newMemoryBackupStore() *memoryBackupStore {
	return &memoryBackupStore{
		config: domain.BackupConfig{Selection: domain.BackupSelection{ExcludeCaches: true}, Schedule: domain.DefaultBackupSchedule},
		runs:   map[uuid.UUID]domain.BackupRun{},
	}
}

func (s *memoryBackupStore) BackupConfig(context.Context) (domain.BackupConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config, nil
}

func (s *memoryBackupStore) SaveBackupConfig(_ context.Context, config domain.BackupConfig, audit domain.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
	s.audits = append(s.audits, audit)
	return nil
}

func (s *memoryBackupStore) StartBackupRun(_ context.Context, run domain.BackupRun, audit *domain.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run.State = domain.BackupRunning
	s.runs[run.ID] = run
	if audit != nil {
		s.audits = append(s.audits, *audit)
	}
	return nil
}

func (s *memoryBackupStore) FinishBackupRun(_ context.Context, run domain.BackupRun, snapshots []domain.BackupSnapshot, initializedAt *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
	if snapshots != nil {
		s.snapshots = snapshots
	}
	if initializedAt != nil {
		s.config.InitializedAt = initializedAt
	}
	return nil
}

func (s *memoryBackupStore) InterruptRunningBackupRuns(_ context.Context, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, run := range s.runs {
		if run.State == domain.BackupRunning {
			run.State, run.Failure, run.FinishedAt = domain.BackupFailed, domain.BackupFailureInterrupted, &at
			s.runs[id] = run
		}
	}
	return nil
}

func (s *memoryBackupStore) sorted() []domain.BackupRun {
	runs := make([]domain.BackupRun, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
	return runs
}

func (s *memoryBackupStore) LatestBackupRuns(context.Context) (ports.BackupLatest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest ports.BackupLatest
	for _, run := range s.sorted() {
		run := run
		if run.State == domain.BackupRunning && latest.Running == nil {
			latest.Running = &run
		}
		if run.Operation != domain.BackupOpBackup {
			continue
		}
		if latest.LastBackup == nil {
			latest.LastBackup = &run
		}
		if run.State == domain.BackupSucceeded && latest.LastSuccess == nil {
			latest.LastSuccess = &run
		}
		if run.Origin != domain.BackupManual && latest.LastScheduled == nil {
			latest.LastScheduled = &run
		}
	}
	return latest, nil
}

func (s *memoryBackupStore) BackupRuns(_ context.Context, filter domain.BackupRunFilter) ([]domain.BackupRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := s.sorted()
	return runs[:min(filter.Limit, len(runs))], nil
}

func (s *memoryBackupStore) BackupRun(_ context.Context, id uuid.UUID) (domain.BackupRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return domain.BackupRun{}, domain.ErrBackupRunNotFound
	}
	return run, nil
}

func (s *memoryBackupStore) BackupSnapshots(context.Context) ([]domain.BackupSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshots, nil
}

func (s *memoryBackupStore) PurgeBackupRuns(context.Context, time.Time) error { return nil }

func (s *memoryBackupStore) run(id uuid.UUID) domain.BackupRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[id]
}

// fakeBackupExecutor records the jobs and answers with its result, once
// release is closed when it is set.
type fakeBackupExecutor struct {
	mu          sync.Mutex
	jobs        []ports.BackupJob
	credentials []domain.BackupCredentials
	release     chan struct{}
	result      domain.BackupResult
	err         error
}

func (f *fakeBackupExecutor) Execute(ctx context.Context, job ports.BackupJob, credentials domain.BackupCredentials) (domain.BackupResult, error) {
	f.mu.Lock()
	f.jobs = append(f.jobs, job)
	f.credentials = append(f.credentials, credentials)
	f.mu.Unlock()
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureInterrupted, Err: ctx.Err()}
		}
	}
	return f.result, f.err
}

func (f *fakeBackupExecutor) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

var backupAdmin = domain.User{ID: uuid.New(), Username: "alice", Role: domain.RoleAdmin}

func newBackups(t *testing.T, store *memoryBackupStore, executor *fakeBackupExecutor, box ports.SecretBox) (*BackupUsecase, context.CancelFunc) {
	t.Helper()
	backups := NewBackupUsecase(store, executor, box, zap.NewNop())
	backups.tick = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		backups.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return backups, cancel
}

func finishedRun(t *testing.T, store *memoryBackupStore, id uuid.UUID) domain.BackupRun {
	t.Helper()
	var run domain.BackupRun
	waitFor(t, func() bool {
		run = store.run(id)
		return run.State != domain.BackupRunning
	})
	return run
}

func readyLocalConfig(t *testing.T, backups *BackupUsecase) {
	t.Helper()
	_, err := backups.UpdateConfig(context.Background(), backupAdmin, BackupConfigUpdate{
		Repository: &domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/backup"},
		Selection:  domain.BackupSelection{Sources: []string{"/etc"}, ExcludeCaches: true},
		Schedule:   domain.DefaultBackupSchedule,
	}, "req-config")
	if err != nil {
		t.Fatal(err)
	}
}

func TestBackupInitGeneratesThePasswordOnce(t *testing.T) {
	store, executor := newMemoryBackupStore(), &fakeBackupExecutor{}
	backups, _ := newBackups(t, store, executor, fakeBox{})
	if _, _, err := backups.InitRepository(context.Background(), backupAdmin, "r"); !errors.Is(err, domain.ErrBackupNotConfigured) {
		t.Fatalf("init without repository = %v", err)
	}
	readyLocalConfig(t, backups)
	password, run, err := backups.InitRepository(context.Background(), backupAdmin, "req-init")
	if err != nil || len(password) < 40 || run.Operation != domain.BackupOpInit {
		t.Fatalf("init = %q, %+v, %v", password, run, err)
	}
	if done := finishedRun(t, store, run.ID); done.State != domain.BackupSucceeded {
		t.Fatalf("init run = %+v", done)
	}
	if executor.credentials[0].Password != password || executor.jobs[0].Repository.Location != "/mnt/backup" {
		t.Fatalf("executor got %+v %+v", executor.jobs[0], executor.credentials[0])
	}
	view, _ := backups.Config(context.Background())
	if !view.Initialized || !view.HasPassword {
		t.Fatalf("view = %+v", view)
	}
	if _, _, err := backups.InitRepository(context.Background(), backupAdmin, "r"); !errors.Is(err, domain.ErrBackupAlreadyInitialized) {
		t.Fatalf("second init = %v", err)
	}
	if store.audits[len(store.audits)-1].Action != "init_repository" {
		t.Fatalf("audits = %+v", store.audits)
	}
}

func TestBackupRunStoresTheResult(t *testing.T) {
	store := newMemoryBackupStore()
	executor := &fakeBackupExecutor{result: domain.BackupResult{
		SnapshotID: "abc", Stats: &domain.BackupStats{FilesNew: 2},
		Snapshots: []domain.BackupSnapshot{{ID: "abc", ShortID: "abc"}},
	}}
	backups, _ := newBackups(t, store, executor, fakeBox{})
	readyLocalConfig(t, backups)
	if _, err := backups.RequestBackup(context.Background(), backupAdmin, "r"); !errors.Is(err, domain.ErrBackupNotInitialized) {
		t.Fatalf("backup before init = %v", err)
	}
	_, initRun, _ := backups.InitRepository(context.Background(), backupAdmin, "r")
	finishedRun(t, store, initRun.ID)

	operator := domain.User{Username: "olivia", Role: domain.RoleOperator}
	run, err := backups.RequestBackup(context.Background(), operator, "req-backup")
	if err != nil {
		t.Fatal(err)
	}
	done := finishedRun(t, store, run.ID)
	if done.State != domain.BackupSucceeded || done.SnapshotID != "abc" || done.Stats.FilesNew != 2 || done.RequestedBy != "olivia" {
		t.Fatalf("run = %+v", done)
	}
	if snapshots, _ := backups.Snapshots(context.Background()); len(snapshots) != 1 {
		t.Fatalf("snapshots = %+v", snapshots)
	}
	audit := store.audits[len(store.audits)-1]
	if audit.TargetType != "backup_run" || audit.TargetID != run.ID.String() || audit.Actor != "olivia" {
		t.Fatalf("audit = %+v", audit)
	}
	summary, err := backups.Summary(context.Background())
	if err != nil || !summary.Initialized || summary.LastSuccess.ID != run.ID || summary.Running != nil {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
}

func TestBackupOneAtATimeAndFailure(t *testing.T) {
	store := newMemoryBackupStore()
	executor := &fakeBackupExecutor{}
	backups, _ := newBackups(t, store, executor, fakeBox{})
	readyLocalConfig(t, backups)
	_, initRun, _ := backups.InitRepository(context.Background(), backupAdmin, "r")
	finishedRun(t, store, initRun.ID)

	executor.mu.Lock()
	executor.release = make(chan struct{})
	executor.err = &domain.BackupRunError{Failure: domain.BackupFailureRestic, Message: "Fatal: wrong password"}
	executor.mu.Unlock()
	run, err := backups.RequestBackup(context.Background(), backupAdmin, "r")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backups.RequestBackup(context.Background(), backupAdmin, "r"); !errors.Is(err, domain.ErrBackupAlreadyRunning) {
		t.Fatalf("second backup = %v", err)
	}
	moved := domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/other"}
	if _, err := backups.UpdateConfig(context.Background(), backupAdmin, BackupConfigUpdate{
		Repository: &moved, Selection: domain.BackupSelection{Sources: []string{"/etc"}}, Schedule: domain.DefaultBackupSchedule,
	}, "r"); !errors.Is(err, domain.ErrBackupRepositoryInUse) {
		t.Fatalf("repository change while running = %v", err)
	}
	close(executor.release)
	done := finishedRun(t, store, run.ID)
	if done.State != domain.BackupFailed || done.Failure != domain.BackupFailureRestic || done.Message != "Fatal: wrong password" {
		t.Fatalf("run = %+v", done)
	}
}

func TestBackupRepositoryChangeAsksForANewInit(t *testing.T) {
	store, executor := newMemoryBackupStore(), &fakeBackupExecutor{}
	backups, _ := newBackups(t, store, executor, fakeBox{})
	key, secret := "key-id", "key-secret"
	view, err := backups.UpdateConfig(context.Background(), backupAdmin, BackupConfigUpdate{
		Repository: &domain.BackupRepository{Type: domain.RepositoryS3, Location: "https://s3.example.com/bucket"},
		Username:   &key, Secret: &secret,
		Selection: domain.BackupSelection{Sources: []string{"/etc"}}, Schedule: domain.DefaultBackupSchedule,
	}, "r")
	if err != nil || !view.HasUsername || !view.HasSecret || view.HasPassword {
		t.Fatalf("view = %+v, %v", view, err)
	}
	_, initRun, err := backups.InitRepository(context.Background(), backupAdmin, "r")
	if err != nil {
		t.Fatal(err)
	}
	finishedRun(t, store, initRun.ID)
	if executor.credentials[0].Username != "key-id" || executor.credentials[0].Secret != "key-secret" {
		t.Fatalf("credentials = %+v", executor.credentials[0])
	}
	// Keeping the keys and moving the bucket resets the password and the
	// initialisation.
	view, err = backups.UpdateConfig(context.Background(), backupAdmin, BackupConfigUpdate{
		Repository: &domain.BackupRepository{Type: domain.RepositoryS3, Location: "https://s3.example.com/other"},
		Selection:  domain.BackupSelection{Sources: []string{"/etc"}}, Schedule: domain.DefaultBackupSchedule,
	}, "r")
	if err != nil || view.Initialized || view.HasPassword || !view.HasUsername {
		t.Fatalf("after the move = %+v, %v", view, err)
	}
}

func TestBackupWithoutSecretsKey(t *testing.T) {
	store, executor := newMemoryBackupStore(), &fakeBackupExecutor{}
	backups, _ := newBackups(t, store, executor, nil)
	readyLocalConfig(t, backups)
	if _, _, err := backups.InitRepository(context.Background(), backupAdmin, "r"); !errors.Is(err, domain.ErrBackupSecretsMissing) {
		t.Fatalf("init = %v", err)
	}
	key := "k"
	if _, err := backups.UpdateConfig(context.Background(), backupAdmin, BackupConfigUpdate{
		Repository: &domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/backup"}, Username: &key,
		Schedule: domain.DefaultBackupSchedule,
	}, "r"); !errors.Is(err, domain.ErrBackupSecretsMissing) {
		t.Fatalf("credentials without key = %v", err)
	}
	if view, _ := backups.Config(context.Background()); view.SecretsEnabled {
		t.Fatal("the view must tell that secrets are unavailable")
	}
}

func TestBackupUpdateConfigValidates(t *testing.T) {
	backups, _ := newBackups(t, newMemoryBackupStore(), &fakeBackupExecutor{}, fakeBox{})
	var validation *domain.BackupValidationError
	_, err := backups.UpdateConfig(context.Background(), backupAdmin, BackupConfigUpdate{
		Selection: domain.BackupSelection{Sources: []string{"relative"}}, Schedule: domain.DefaultBackupSchedule,
	}, "r")
	if !errors.As(err, &validation) || validation.Field != "sources" {
		t.Fatalf("err = %v", err)
	}
	if _, err := backups.Runs(context.Background(), domain.BackupRunFilter{Limit: 0}); !errors.Is(err, domain.ErrInvalidBackupRunFilter) {
		t.Fatalf("runs = %v", err)
	}
}

func TestBackupScheduleRunsAndCatchesUp(t *testing.T) {
	store, executor := newMemoryBackupStore(), &fakeBackupExecutor{result: domain.BackupResult{SnapshotID: "s"}}
	initialized := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	store.config = domain.BackupConfig{
		Repository:        &domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/backup"},
		SealedCredentials: domain.BackupSecretsAssociatedData + `|{"password":"p"}`, InitializedAt: &initialized,
		Selection: domain.BackupSelection{Sources: []string{"/etc"}},
		Schedule:  domain.BackupSchedule{Enabled: true, Frequency: domain.BackupDaily, Hour: 3},
		UpdatedAt: &initialized,
	}
	// The Manager starts on the 24th at 10:00: the backups of the 21st to
	// the 24th were missed; one catch-up runs, after the start-up delay.
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	set := func(at time.Time) {
		mu.Lock()
		now = at
		mu.Unlock()
	}
	backups := NewBackupUsecase(store, executor, fakeBox{}, zap.NewNop())
	backups.now, backups.location, backups.tick = clock, time.UTC, 5*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		backups.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()
	time.Sleep(50 * time.Millisecond)
	if executor.calls() != 0 {
		t.Fatal("the catch-up must wait for the start-up delay")
	}
	set(now.Add(domain.BackupCatchUpDelay + time.Second))
	waitFor(t, func() bool { return executor.calls() == 1 })
	latest, _ := store.LatestBackupRuns(context.Background())
	if latest.LastScheduled == nil || latest.LastScheduled.Origin != domain.BackupCatchUp {
		t.Fatalf("catch-up run = %+v", latest.LastScheduled)
	}
	time.Sleep(50 * time.Millisecond)
	if executor.calls() != 1 {
		t.Fatalf("%d runs, want a single catch-up", executor.calls())
	}
	// The next day at 03:00, the scheduled backup runs on time.
	set(time.Date(2026, 11, 25, 3, 0, 10, 0, time.UTC))
	waitFor(t, func() bool { return executor.calls() == 2 })
	waitFor(t, func() bool {
		latest, _ = store.LatestBackupRuns(context.Background())
		return latest.LastScheduled != nil && latest.LastScheduled.Origin == domain.BackupScheduled
	})
	summary, _ := backups.Summary(context.Background())
	if summary.NextRunAt == nil || !summary.NextRunAt.Equal(time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("next run = %v", summary.NextRunAt)
	}
}

func TestBackupInterruptedWhenTheManagerStops(t *testing.T) {
	store := newMemoryBackupStore()
	executor := &fakeBackupExecutor{}
	backups, stop := newBackups(t, store, executor, fakeBox{})
	readyLocalConfig(t, backups)
	_, initRun, _ := backups.InitRepository(context.Background(), backupAdmin, "r")
	finishedRun(t, store, initRun.ID)
	executor.mu.Lock()
	executor.release = make(chan struct{})
	executor.mu.Unlock()
	run, err := backups.RequestBackup(context.Background(), backupAdmin, "r")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return executor.calls() == 2 })
	stop()
	if done := finishedRun(t, store, run.ID); done.Failure != domain.BackupFailureInterrupted {
		t.Fatalf("run = %+v", done)
	}
}
