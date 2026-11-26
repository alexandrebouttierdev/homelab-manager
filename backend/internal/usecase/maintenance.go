package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"

	"go.uber.org/zap"
)

const (
	auditTargetMaintenance = "maintenance_task"
	// maintenanceTick is how often the schedules are checked.
	maintenanceTick = 30 * time.Second
	// maintenanceStoreTimeout bounds the writes of a run outcome.
	maintenanceStoreTimeout = 5 * time.Second
)

// MaintenanceTaskView is a task of the catalog with where it stands.
type MaintenanceTaskView struct {
	domain.MaintenanceTask
	Running bool
	NextRun *time.Time
}

// MaintenanceUsecase schedules and runs the tasks of the maintenance
// catalog (decision D-1). Run owns the goroutine that runs them, one at a
// time, each bounded by domain.MaintenanceTimeout; a task missed while the
// Manager was stopped is not caught up. Callers of Update and Request must
// hold domain.PermManageSettings; the HTTP layer enforces it.
type MaintenanceUsecase struct {
	store    ports.MaintenanceStore
	units    ports.UnitController
	audit    ports.AuditRecorder
	logger   *zap.Logger
	now      func() time.Time
	location *time.Location
	tick     time.Duration

	mu        sync.Mutex
	accepting bool
	running   domain.MaintenanceKind
	requests  chan maintenanceRequest
}

type maintenanceRequest struct {
	kind  domain.MaintenanceKind
	audit domain.AuditEntry
}

func NewMaintenanceUsecase(store ports.MaintenanceStore, units ports.UnitController, audit ports.AuditRecorder, logger *zap.Logger) *MaintenanceUsecase {
	return &MaintenanceUsecase{
		store: store, units: units, audit: audit, logger: logger, now: time.Now, location: time.Local,
		tick: maintenanceTick, requests: make(chan maintenanceRequest, 1),
	}
}

// Tasks returns every task of the catalog, with its defaults when never
// saved.
func (u *MaintenanceUsecase) Tasks(ctx context.Context) ([]MaintenanceTaskView, error) {
	stored, err := u.store.MaintenanceTasks(ctx)
	if err != nil {
		return nil, err
	}
	byKind := map[domain.MaintenanceKind]domain.MaintenanceTask{}
	for _, task := range stored {
		byKind[task.Kind] = task
	}
	u.mu.Lock()
	running := u.running
	u.mu.Unlock()
	now := u.now().In(u.location)
	views := make([]MaintenanceTaskView, 0, len(domain.MaintenanceKinds))
	for _, kind := range domain.MaintenanceKinds {
		task, ok := byKind[kind]
		if !ok {
			task = domain.DefaultMaintenanceTask(kind)
		}
		view := MaintenanceTaskView{MaintenanceTask: task, Running: running == kind}
		if next, ok := task.Schedule.Next(now); ok {
			next = next.UTC()
			view.NextRun = &next
		}
		views = append(views, view)
	}
	return views, nil
}

// Update stores the schedule and the parameters of a task, audited.
func (u *MaintenanceUsecase) Update(ctx context.Context, actor domain.User, task domain.MaintenanceTask, requestID string) (MaintenanceTaskView, error) {
	if err := task.Validate(); err != nil {
		return MaintenanceTaskView{}, err
	}
	now := u.now().UTC()
	task.UpdatedAt, task.UpdatedBy, task.LastRun = &now, actor.Username, nil
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetMaintenance, TargetID: string(task.Kind), Action: "update",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	if err := u.store.SaveMaintenanceTask(ctx, task, audit); err != nil {
		return MaintenanceTaskView{}, err
	}
	views, err := u.Tasks(ctx)
	if err != nil {
		return MaintenanceTaskView{}, err
	}
	for _, view := range views {
		if view.Kind == task.Kind {
			return view, nil
		}
	}
	return MaintenanceTaskView{}, domain.ErrUnknownMaintenanceTask
}

// Request runs a task now, after an audit entry; one run at a time.
func (u *MaintenanceUsecase) Request(ctx context.Context, actor domain.User, kind domain.MaintenanceKind, requestID string) error {
	if !kind.Valid() {
		return domain.ErrUnknownMaintenanceTask
	}
	u.mu.Lock()
	accepting, busy := u.accepting, u.running != "" || len(u.requests) > 0
	u.mu.Unlock()
	if !accepting {
		return domain.ErrMaintenanceUnavailable
	}
	if busy {
		return domain.ErrActionInProgress
	}
	entry := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetMaintenance, TargetID: string(kind), Action: "run",
		RequestID: requestID, Outcome: domain.AuditRequested,
	}
	// No trace, no run.
	if err := u.audit.RecordAudit(ctx, entry); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	select {
	case u.requests <- maintenanceRequest{kind: kind, audit: entry}:
		return nil
	default:
		return domain.ErrActionInProgress
	}
}

// Run checks the schedules every tick and runs the requested tasks until
// ctx is cancelled; a run in progress ends with the context.
func (u *MaintenanceUsecase) Run(ctx context.Context) {
	u.mu.Lock()
	u.accepting = true
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.accepting = false
		u.mu.Unlock()
	}()
	checked := u.now()
	ticker := time.NewTicker(u.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-u.requests:
			u.runTask(ctx, request.kind, domain.MaintenanceManual, &request.audit)
		case <-ticker.C:
			now := u.now()
			for _, kind := range u.due(ctx, checked, now) {
				u.runTask(ctx, kind, domain.MaintenanceSchedule, nil)
			}
			checked = now
		}
	}
}

// due lists the enabled tasks whose next run after checked is not after now.
func (u *MaintenanceUsecase) due(ctx context.Context, checked, now time.Time) []domain.MaintenanceKind {
	tasks, err := u.store.MaintenanceTasks(ctx)
	if err != nil {
		u.logger.Warn("Could not read the maintenance tasks", zap.Error(err))
		return nil
	}
	var due []domain.MaintenanceKind
	for _, task := range tasks {
		if next, ok := task.Schedule.Next(checked.In(u.location)); ok && !next.After(now) {
			due = append(due, task.Kind)
		}
	}
	return due
}

func (u *MaintenanceUsecase) runTask(ctx context.Context, kind domain.MaintenanceKind, origin domain.MaintenanceOrigin, audit *domain.AuditEntry) {
	u.mu.Lock()
	u.running = kind
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.running = ""
		u.mu.Unlock()
	}()
	task := domain.DefaultMaintenanceTask(kind)
	if tasks, err := u.store.MaintenanceTasks(ctx); err == nil {
		for _, stored := range tasks {
			if stored.Kind == kind {
				task = stored
			}
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, domain.MaintenanceTimeout)
	_, err := u.units.Act(runCtx, task.Unit(), domain.UnitStart)
	cancel()
	run := domain.MaintenanceRun{At: u.now().UTC(), Origin: origin, Succeeded: err == nil}
	fields := []zap.Field{zap.String("task", string(kind)), zap.String("unit", task.Unit()), zap.String("origin", string(origin))}
	if err != nil {
		run.Message = maintenanceFailure(err)
		u.logger.Warn("Maintenance task failed", append(fields, zap.Error(err))...)
	} else {
		u.logger.Info("Maintenance task done", fields...)
	}
	storeCtx, cancelStore := context.WithTimeout(context.WithoutCancel(ctx), maintenanceStoreTimeout)
	defer cancelStore()
	if err := u.store.RecordMaintenanceRun(storeCtx, kind, run); err != nil {
		u.logger.Error("Could not record a maintenance run", append(fields, zap.Error(err))...)
	}
	entry := domain.AuditEntry{Actor: "schedule", TargetType: auditTargetMaintenance, TargetID: string(kind), Action: "run"}
	if audit != nil {
		entry = *audit
	}
	entry.Outcome = domain.AuditSuccess
	if err != nil {
		entry.Outcome = domain.AuditFailure
	}
	if err := u.audit.RecordAudit(storeCtx, entry); err != nil {
		u.logger.Error("Could not audit a maintenance run", append(fields, zap.Error(err))...)
	}
}

// maintenanceFailure explains a failed run in French for the interface.
func maintenanceFailure(err error) string {
	var actionErr *domain.UnitActionError
	if !errors.As(err, &actionErr) {
		return "systemd injoignable"
	}
	switch actionErr.Failure {
	case domain.ActionNotAllowed:
		return "polkit a refusé : installez la règle 50-monoops-maintenance.rules"
	case domain.ActionNotFound:
		return "unité absente : installez monoops-journal-vacuum@.service"
	case domain.ActionUnitFailed:
		return "l'unité a échoué : consultez son journal"
	case domain.ActionTimeout:
		return "pas de fin en 10 minutes"
	case domain.ActionInterrupted:
		return "interrompue par l'arrêt du Manager"
	}
	return "systemd a refusé le démarrage"
}
