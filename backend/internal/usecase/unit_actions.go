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
	auditTargetSystemUnit = "system_unit"
	// unitActionAuditTimeout bounds the audit entry written at the end of
	// an action, including while the Manager stops.
	unitActionAuditTimeout = 5 * time.Second
)

// UnitActionUsecase starts, stops, restarts and reloads the units of the
// allow list (SPEC_ADMINISTRATION.md § 5.2). Callers must hold
// domain.PermManageServices; the HTTP layer enforces it. Run owns the
// goroutines that follow the actions: each is bounded by
// domain.UnitActionTimeout and cancelled when Run's context ends.
type UnitActionUsecase struct {
	controller ports.UnitController
	audit      ports.AuditRecorder
	allowed    domain.ManageableUnits
	logger     *zap.Logger
	now        func() time.Time
	timeout    time.Duration

	mu sync.Mutex
	// base is the context of Run; nil while Run is not running, when
	// actions are refused.
	base    context.Context
	running sync.WaitGroup
	// actions holds the last action of each unit of the allow list.
	actions map[string]domain.UnitAction
	// requests holds, per user, the times of the requests of the last
	// minute.
	requests map[string][]time.Time
}

func NewUnitActionUsecase(controller ports.UnitController, audit ports.AuditRecorder, allowed domain.ManageableUnits, logger *zap.Logger) *UnitActionUsecase {
	return &UnitActionUsecase{
		controller: controller, audit: audit, allowed: allowed, logger: logger,
		now: time.Now, timeout: domain.UnitActionTimeout,
		actions: map[string]domain.UnitAction{}, requests: map[string][]time.Time{},
	}
}

// ManageableUnits returns the allow list, sorted.
func (u *UnitActionUsecase) ManageableUnits() []string {
	return u.allowed.Names()
}

// Run accepts actions until ctx is cancelled, then waits for the actions
// being followed, which end as interrupted.
func (u *UnitActionUsecase) Run(ctx context.Context) {
	u.mu.Lock()
	u.base = ctx
	u.mu.Unlock()
	<-ctx.Done()
	u.mu.Lock()
	u.base = nil
	u.mu.Unlock()
	u.running.Wait()
}

// Request checks and audits an action, then follows it in the background.
// It returns the action as running; Last gives its outcome.
func (u *UnitActionUsecase) Request(ctx context.Context, actor domain.User, name string, verb domain.UnitVerb, requestID string) (domain.UnitAction, error) {
	if !verb.Valid() {
		return domain.UnitAction{}, domain.ErrInvalidUnitVerb
	}
	if !domain.ValidUnitName(name) {
		return domain.UnitAction{}, domain.ErrInvalidUnitName
	}
	if !u.allowed.Contains(name) {
		return domain.UnitAction{}, domain.ErrUnitNotManageable
	}
	now := u.now().UTC()
	action := domain.UnitAction{
		Unit: name, Verb: verb, State: domain.ActionRunning, RequestedBy: actor.Username, RequestedAt: now,
	}
	base, previous, hadPrevious, err := u.reserve(actor.Username, action)
	if err != nil {
		return domain.UnitAction{}, err
	}
	entry := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetSystemUnit, TargetID: name,
		Action: string(verb), RequestID: requestID, Outcome: domain.AuditRequested,
	}
	// No trace, no action: the request is recorded before systemd is asked.
	if err := u.audit.RecordAudit(ctx, entry); err != nil {
		u.release(name, previous, hadPrevious)
		return domain.UnitAction{}, fmt.Errorf("record audit: %w", err)
	}
	go u.follow(base, action, entry)
	return action, nil
}

// reserve checks the worker, the action in progress and the rate of the
// user, then records the action as running. The caller owns one count of
// u.running and must call follow or release.
func (u *UnitActionUsecase) reserve(username string, action domain.UnitAction) (context.Context, domain.UnitAction, bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.base == nil {
		return nil, domain.UnitAction{}, false, domain.ErrUnitActionsUnavailable
	}
	previous, hadPrevious := u.actions[action.Unit]
	if hadPrevious && previous.State == domain.ActionRunning {
		return nil, domain.UnitAction{}, false, domain.ErrActionInProgress
	}
	recent := u.requests[username][:0]
	for _, at := range u.requests[username] {
		if action.RequestedAt.Sub(at) < time.Minute {
			recent = append(recent, at)
		}
	}
	if len(recent) >= domain.UnitActionsPerMinute {
		u.requests[username] = recent
		return nil, domain.UnitAction{}, false, domain.ErrUnitActionsLimited
	}
	u.requests[username] = append(recent, action.RequestedAt)
	u.actions[action.Unit] = action
	u.running.Add(1)
	return u.base, previous, hadPrevious, nil
}

// release undoes reserve when the action could not be audited.
func (u *UnitActionUsecase) release(name string, previous domain.UnitAction, hadPrevious bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if hadPrevious {
		u.actions[name] = previous
	} else {
		delete(u.actions, name)
	}
	u.running.Done()
}

func (u *UnitActionUsecase) follow(base context.Context, action domain.UnitAction, entry domain.AuditEntry) {
	defer u.running.Done()
	ctx, cancel := context.WithTimeout(base, u.timeout)
	final, err := u.controller.Act(ctx, action.Unit, action.Verb)
	cancel()

	finished := u.now().UTC()
	action.FinishedAt = &finished
	action.State = domain.ActionSucceeded
	if final.ActiveState != "" {
		action.Final = &final
	}
	if err != nil {
		action.State = domain.ActionFailed
		var actionErr *domain.UnitActionError
		switch {
		case base.Err() != nil:
			action.Failure = domain.ActionInterrupted
		case errors.As(err, &actionErr):
			action.Failure = actionErr.Failure
		default:
			action.Failure = domain.ActionUnavailable
		}
	}
	u.mu.Lock()
	u.actions[action.Unit] = action
	u.mu.Unlock()

	entry.Outcome = domain.AuditSuccess
	fields := []zap.Field{
		zap.String("unit", action.Unit), zap.String("verb", string(action.Verb)),
		zap.String("actor", action.RequestedBy), zap.String("request_id", entry.RequestID),
	}
	if err != nil {
		entry.Outcome = domain.AuditFailure
		u.logger.Warn("Unit action failed", append(fields, zap.String("failure", string(action.Failure)), zap.Error(err))...)
	} else {
		u.logger.Info("Unit action done", fields...)
	}
	auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(base), unitActionAuditTimeout)
	defer cancelAudit()
	if err := u.audit.RecordAudit(auditCtx, entry); err != nil {
		u.logger.Error("Could not audit the outcome of a unit action", append(fields, zap.Error(err))...)
	}
}

// Last returns the last action requested on a unit since the Manager
// started.
func (u *UnitActionUsecase) Last(name string) (domain.UnitAction, bool, error) {
	if !domain.ValidUnitName(name) {
		return domain.UnitAction{}, false, domain.ErrInvalidUnitName
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	action, ok := u.actions[name]
	return action, ok, nil
}
