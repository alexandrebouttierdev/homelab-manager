package usecase

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"go.uber.org/zap"
)

type memoryMaintenanceStore struct {
	mu    sync.Mutex
	tasks map[domain.MaintenanceKind]domain.MaintenanceTask
}

func (s *memoryMaintenanceStore) MaintenanceTasks(context.Context) ([]domain.MaintenanceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks := make([]domain.MaintenanceTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (s *memoryMaintenanceStore) SaveMaintenanceTask(_ context.Context, task domain.MaintenanceTask, _ domain.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.tasks[task.Kind]; ok {
		task.LastRun = previous.LastRun
	}
	s.tasks[task.Kind] = task
	return nil
}

func (s *memoryMaintenanceStore) RecordMaintenanceRun(_ context.Context, kind domain.MaintenanceKind, run domain.MaintenanceRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[kind]
	if !ok {
		task = domain.DefaultMaintenanceTask(kind)
	}
	task.LastRun = &run
	s.tasks[kind] = task
	return nil
}

func (s *memoryMaintenanceStore) lastRun(kind domain.MaintenanceKind) *domain.MaintenanceRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[kind].LastRun
}

func startMaintenance(t *testing.T, controller *fakeController, audit *syncAudit, now func() time.Time) (*MaintenanceUsecase, *memoryMaintenanceStore) {
	t.Helper()
	store := &memoryMaintenanceStore{tasks: map[domain.MaintenanceKind]domain.MaintenanceTask{}}
	maintenance := NewMaintenanceUsecase(store, controller, audit, zap.NewNop())
	maintenance.tick, maintenance.location = 5*time.Millisecond, time.UTC
	if now != nil {
		maintenance.now = now
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		maintenance.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	waitFor(t, func() bool {
		maintenance.mu.Lock()
		defer maintenance.mu.Unlock()
		return maintenance.accepting
	})
	return maintenance, store
}

func TestMaintenanceRunsOnRequestWithItsUnit(t *testing.T) {
	controller := &fakeController{}
	audit := &syncAudit{}
	maintenance, store := startMaintenance(t, controller, audit, nil)
	ctx := context.Background()
	views, err := maintenance.Tasks(ctx)
	if err != nil || len(views) != 1 || views[0].Kind != domain.MaintenanceJournalVacuum || views[0].Schedule.Enabled || views[0].NextRun != nil {
		t.Fatalf("default tasks = %+v, %v", views, err)
	}
	task := views[0].MaintenanceTask
	task.MaxSizeMiB = 1024
	if _, err := maintenance.Update(ctx, systemAdmin, task, "req"); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.Request(ctx, systemAdmin, domain.MaintenanceJournalVacuum, "req"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return store.lastRun(domain.MaintenanceJournalVacuum) != nil })
	run := store.lastRun(domain.MaintenanceJournalVacuum)
	if !run.Succeeded || run.Origin != domain.MaintenanceManual {
		t.Fatalf("run = %+v", run)
	}
	controller.mu.Lock()
	calls := slices.Clone(controller.calls)
	controller.mu.Unlock()
	if !slices.Equal(calls, []string{"start homelab-journal-vacuum@1024M.service"}) {
		t.Fatalf("calls = %v", calls)
	}
	waitFor(t, func() bool { return len(audit.outcomes()) == 2 })
	if got := audit.outcomes(); got[0] != domain.AuditRequested || got[1] != domain.AuditSuccess {
		t.Fatalf("audit = %v", got)
	}
	if err := maintenance.Request(ctx, systemAdmin, "rm-rf", "req"); !errors.Is(err, domain.ErrUnknownMaintenanceTask) {
		t.Fatalf("unknown task = %v", err)
	}
	task.MaxSizeMiB = 300
	if _, err := maintenance.Update(ctx, systemAdmin, task, "req"); !errors.Is(err, domain.ErrInvalidMaintenanceTask) {
		t.Fatalf("size outside the list = %v", err)
	}
}

func TestMaintenanceRunsWhenScheduled(t *testing.T) {
	controller := &fakeController{err: &domain.UnitActionError{Failure: domain.ActionNotAllowed, Err: errors.New("polkit")}}
	var mu sync.Mutex
	clock := time.Date(2026, 11, 25, 3, 59, 0, 0, time.UTC)
	now := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	maintenance, store := startMaintenance(t, controller, &syncAudit{}, now)
	task := domain.DefaultMaintenanceTask(domain.MaintenanceJournalVacuum)
	task.Schedule.Enabled, task.Schedule.Frequency = true, domain.BackupDaily
	if _, err := maintenance.Update(context.Background(), systemAdmin, task, "req"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if store.lastRun(domain.MaintenanceJournalVacuum) != nil {
		t.Fatal("the task ran before its time")
	}
	mu.Lock()
	clock = clock.Add(2 * time.Minute)
	mu.Unlock()
	waitFor(t, func() bool { return store.lastRun(domain.MaintenanceJournalVacuum) != nil })
	run := store.lastRun(domain.MaintenanceJournalVacuum)
	if run.Succeeded || run.Origin != domain.MaintenanceSchedule || run.Message == "" {
		t.Fatalf("scheduled run = %+v", run)
	}
	time.Sleep(30 * time.Millisecond)
	controller.mu.Lock()
	calls := len(controller.calls)
	controller.mu.Unlock()
	if calls != 1 {
		t.Fatalf("the task ran %d times for one schedule", calls)
	}
}
