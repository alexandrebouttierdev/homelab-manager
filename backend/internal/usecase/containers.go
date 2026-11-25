package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"go.uber.org/zap"
)

const auditTargetContainer = "container"

// ContainerUsecase reads the logs of the containers and starts, stops or
// restarts them (SPEC_ADMINISTRATION.md, D-5). Actions need
// docker_actions_enabled, the permission domain.PermManageContainers
// (checked by the HTTP layer) and the same setting in the proxy. Run owns
// the goroutines that follow the actions; each is bounded by
// domain.ContainerActionTimeout and cancelled when Run's context ends.
type ContainerUsecase struct {
	lookup     ports.ContainerLookup
	logs       ports.ContainerLogReader
	controller ports.ContainerController
	audit      ports.AuditRecorder
	enabled    bool
	logger     *zap.Logger
	now        func() time.Time
	timeout    time.Duration

	mu       sync.Mutex
	base     context.Context
	running  sync.WaitGroup
	actions  map[string]domain.ContainerAction
	requests map[string][]time.Time
}

func NewContainerUsecase(lookup ports.ContainerLookup, logs ports.ContainerLogReader, controller ports.ContainerController,
	audit ports.AuditRecorder, enabled bool, logger *zap.Logger) *ContainerUsecase {
	return &ContainerUsecase{
		lookup: lookup, logs: logs, controller: controller, audit: audit, enabled: enabled, logger: logger,
		now: time.Now, timeout: domain.ContainerActionTimeout,
		actions: map[string]domain.ContainerAction{}, requests: map[string][]time.Time{},
	}
}

// ActionsEnabled reports docker_actions_enabled.
func (u *ContainerUsecase) ActionsEnabled() bool { return u.enabled }

// Logs returns the end of the log of a container of the inventory. A
// removed container has no log left to read.
func (u *ContainerUsecase) Logs(ctx context.Context, id string, query domain.ContainerLogQuery) (domain.Container, domain.ContainerLogs, error) {
	if err := query.Validate(); err != nil {
		return domain.Container{}, domain.ContainerLogs{}, err
	}
	container, err := u.lookup.GetContainer(ctx, id)
	if err != nil {
		return domain.Container{}, domain.ContainerLogs{}, err
	}
	if container.RemovedAt != nil {
		return container, domain.ContainerLogs{}, domain.ErrContainerRemoved
	}
	logs, err := u.logs.ContainerLogs(ctx, container.DockerID, query)
	return container, logs, err
}

// Run accepts actions until ctx is cancelled, then waits for the actions
// being followed.
func (u *ContainerUsecase) Run(ctx context.Context) {
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
func (u *ContainerUsecase) Request(ctx context.Context, actor domain.User, id string, verb domain.ContainerVerb, requestID string) (domain.ContainerAction, error) {
	if !verb.Valid() {
		return domain.ContainerAction{}, domain.ErrInvalidContainerVerb
	}
	if !u.enabled {
		return domain.ContainerAction{}, domain.ErrContainerActionsDisabled
	}
	container, err := u.lookup.GetContainer(ctx, id)
	if err != nil {
		return domain.ContainerAction{}, err
	}
	if container.RemovedAt != nil {
		return domain.ContainerAction{}, domain.ErrContainerRemoved
	}
	action := domain.ContainerAction{
		ContainerID: container.ID, Name: container.Name, Verb: verb, State: domain.ActionRunning,
		RequestedBy: actor.Username, RequestedAt: u.now().UTC(),
	}
	key := container.ID.String()
	base, previous, hadPrevious, err := u.reserve(key, action)
	if err != nil {
		return domain.ContainerAction{}, err
	}
	entry := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetContainer, TargetID: container.Name,
		Action: string(verb), RequestID: requestID, Outcome: domain.AuditRequested,
	}
	// No trace, no action: the request is recorded before Docker is asked.
	if err := u.audit.RecordAudit(ctx, entry); err != nil {
		u.release(key, previous, hadPrevious)
		return domain.ContainerAction{}, fmt.Errorf("record audit: %w", err)
	}
	go u.follow(base, key, container.DockerID, action, entry)
	return action, nil
}

func (u *ContainerUsecase) reserve(key string, action domain.ContainerAction) (context.Context, domain.ContainerAction, bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.base == nil {
		return nil, domain.ContainerAction{}, false, domain.ErrContainerActionsUnavailable
	}
	previous, hadPrevious := u.actions[key]
	if hadPrevious && previous.State == domain.ActionRunning {
		return nil, domain.ContainerAction{}, false, domain.ErrActionInProgress
	}
	recent := u.requests[action.RequestedBy][:0]
	for _, at := range u.requests[action.RequestedBy] {
		if action.RequestedAt.Sub(at) < time.Minute {
			recent = append(recent, at)
		}
	}
	if len(recent) >= domain.ContainerActionsPerMinute {
		u.requests[action.RequestedBy] = recent
		return nil, domain.ContainerAction{}, false, domain.ErrContainerActionsLimited
	}
	u.requests[action.RequestedBy] = append(recent, action.RequestedAt)
	u.actions[key] = action
	u.running.Add(1)
	return u.base, previous, hadPrevious, nil
}

func (u *ContainerUsecase) release(key string, previous domain.ContainerAction, hadPrevious bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if hadPrevious {
		u.actions[key] = previous
	} else {
		delete(u.actions, key)
	}
	u.running.Done()
}

func (u *ContainerUsecase) follow(base context.Context, key, dockerID string, action domain.ContainerAction, entry domain.AuditEntry) {
	defer u.running.Done()
	ctx, cancel := context.WithTimeout(base, u.timeout)
	err := u.controller.ActOnContainer(ctx, dockerID, action.Verb)
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	cancel()

	finished := u.now().UTC()
	action.FinishedAt = &finished
	action.State = domain.ActionSucceeded
	if err != nil {
		action.State = domain.ActionFailed
		var actionErr *domain.ContainerActionError
		switch {
		case base.Err() != nil:
			action.Failure = domain.ContainerActionInterrupted
		case timedOut:
			action.Failure = domain.ContainerActionTimedOut
		case errors.As(err, &actionErr):
			action.Failure = actionErr.Failure
		default:
			action.Failure = domain.ContainerActionUnavailable
		}
	}
	u.mu.Lock()
	u.actions[key] = action
	u.mu.Unlock()

	entry.Outcome = domain.AuditSuccess
	fields := []zap.Field{
		zap.String("container", action.Name), zap.String("verb", string(action.Verb)),
		zap.String("actor", action.RequestedBy), zap.String("request_id", entry.RequestID),
	}
	if err != nil {
		entry.Outcome = domain.AuditFailure
		u.logger.Warn("Container action failed", append(fields, zap.String("failure", string(action.Failure)), zap.Error(err))...)
	} else {
		u.logger.Info("Container action done", fields...)
	}
	auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(base), unitActionAuditTimeout)
	defer cancelAudit()
	if err := u.audit.RecordAudit(auditCtx, entry); err != nil {
		u.logger.Error("Could not audit the outcome of a container action", append(fields, zap.Error(err))...)
	}
}

// Last returns the last action requested on a container since the Manager
// started.
func (u *ContainerUsecase) Last(ctx context.Context, id string) (domain.ContainerAction, bool, error) {
	container, err := u.lookup.GetContainer(ctx, id)
	if err != nil {
		return domain.ContainerAction{}, false, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	action, ok := u.actions[container.ID.String()]
	return action, ok, nil
}
