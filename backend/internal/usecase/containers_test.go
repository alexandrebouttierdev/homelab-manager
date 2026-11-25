package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeContainers struct {
	byID map[string]domain.Container
}

func (f fakeContainers) GetContainer(_ context.Context, id string) (domain.Container, error) {
	container, ok := f.byID[id]
	if !ok {
		return domain.Container{}, domain.ErrContainerNotFound
	}
	return container, nil
}

type fakeDocker struct {
	mu      sync.Mutex
	calls   []string
	release chan struct{}
	err     error
	query   domain.ContainerLogQuery
}

func (f *fakeDocker) ContainerLogs(_ context.Context, dockerID string, query domain.ContainerLogQuery) (domain.ContainerLogs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "logs "+dockerID)
	f.query = query
	return domain.ContainerLogs{Lines: []domain.ContainerLogLine{{Stream: domain.ContainerStdout, Message: "ready"}}}, nil
}

func (f *fakeDocker) ActOnContainer(ctx context.Context, dockerID string, verb domain.ContainerVerb) error {
	f.mu.Lock()
	f.calls = append(f.calls, string(verb)+" "+dockerID)
	f.mu.Unlock()
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.err
}

func newContainerFixture(t *testing.T, enabled bool, docker *fakeDocker, audit *syncAudit) (*ContainerUsecase, domain.Container, domain.Container) {
	t.Helper()
	removedAt := time.Now()
	web := domain.Container{ID: uuid.New(), DockerID: "0123456789ab", Name: "web"}
	old := domain.Container{ID: uuid.New(), DockerID: "ba9876543210", Name: "old", RemovedAt: &removedAt}
	lookup := fakeContainers{byID: map[string]domain.Container{web.ID.String(): web, old.ID.String(): old}}
	containers := NewContainerUsecase(lookup, docker, docker, audit, enabled, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		containers.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	// Wait for Run to accept actions.
	deadline := time.Now().Add(2 * time.Second)
	for {
		containers.mu.Lock()
		ready := containers.base != nil
		containers.mu.Unlock()
		if ready || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	return containers, web, old
}

func waitContainerAction(t *testing.T, containers *ContainerUsecase, id string) domain.ContainerAction {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		action, found, err := containers.Last(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if found && action.State != domain.ActionRunning {
			return action
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the container action did not end")
	return domain.ContainerAction{}
}

func TestContainerLogsOfTheInventory(t *testing.T) {
	docker := &fakeDocker{}
	containers, web, old := newContainerFixture(t, false, docker, &syncAudit{})
	_, logs, err := containers.Logs(context.Background(), web.ID.String(), domain.ContainerLogQuery{Tail: 50})
	if err != nil || len(logs.Lines) != 1 || docker.query.Tail != 50 || docker.calls[0] != "logs 0123456789ab" {
		t.Fatalf("logs = %+v, %v, calls %v", logs, err, docker.calls)
	}
	if _, _, err := containers.Logs(context.Background(), old.ID.String(), domain.ContainerLogQuery{Tail: 50}); !errors.Is(err, domain.ErrContainerRemoved) {
		t.Fatalf("removed container = %v", err)
	}
	if _, _, err := containers.Logs(context.Background(), web.ID.String(), domain.ContainerLogQuery{Tail: domain.MaxContainerLogLines + 1}); !errors.Is(err, domain.ErrInvalidLogQuery) {
		t.Fatalf("too many lines = %v", err)
	}
	if _, _, err := containers.Logs(context.Background(), uuid.NewString(), domain.ContainerLogQuery{Tail: 1}); !errors.Is(err, domain.ErrContainerNotFound) {
		t.Fatalf("unknown container = %v", err)
	}
}

func TestContainerActionsNeedTheSetting(t *testing.T) {
	docker := &fakeDocker{}
	audit := &syncAudit{}
	containers, web, _ := newContainerFixture(t, false, docker, audit)
	if _, err := containers.Request(context.Background(), unitOperator, web.ID.String(), domain.ContainerRestart, "req"); !errors.Is(err, domain.ErrContainerActionsDisabled) {
		t.Fatalf("disabled = %v", err)
	}
	if len(docker.calls) != 0 || len(audit.outcomes()) != 0 {
		t.Fatalf("a disabled action reached Docker or the audit: %v %v", docker.calls, audit.outcomes())
	}
}

func TestContainerActionIsAuditedAndFollowed(t *testing.T) {
	docker := &fakeDocker{release: make(chan struct{})}
	audit := &syncAudit{}
	containers, web, old := newContainerFixture(t, true, docker, audit)

	action, err := containers.Request(context.Background(), unitOperator, web.ID.String(), domain.ContainerRestart, "req-1")
	if err != nil || action.State != domain.ActionRunning || action.Name != "web" {
		t.Fatalf("request = %+v, %v", action, err)
	}
	if _, err := containers.Request(context.Background(), unitOperator, web.ID.String(), domain.ContainerStop, "req-2"); !errors.Is(err, domain.ErrActionInProgress) {
		t.Fatalf("second action = %v", err)
	}
	close(docker.release)
	final := waitContainerAction(t, containers, web.ID.String())
	if final.State != domain.ActionSucceeded || final.FinishedAt == nil {
		t.Fatalf("final = %+v", final)
	}
	// The outcome is audited just after the action ends.
	deadline := time.Now().Add(2 * time.Second)
	for len(audit.outcomes()) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := audit.outcomes(); len(got) != 2 || got[0] != domain.AuditRequested || got[1] != domain.AuditSuccess {
		t.Fatalf("audit = %v", got)
	}
	audit.mu.Lock()
	first := audit.entries[0]
	audit.mu.Unlock()
	if first.TargetType != "container" || first.TargetID != "web" || first.Action != "restart" {
		t.Fatalf("audit entry = %+v", first)
	}

	docker.err = &domain.ContainerActionError{Failure: domain.ContainerActionNotAllowed, Err: errors.New("403")}
	if _, err := containers.Request(context.Background(), unitOperator, web.ID.String(), domain.ContainerStop, "req-3"); err != nil {
		t.Fatal(err)
	}
	if failed := waitContainerAction(t, containers, web.ID.String()); failed.State != domain.ActionFailed || failed.Failure != domain.ContainerActionNotAllowed {
		t.Fatalf("refused by the proxy = %+v", failed)
	}
	if _, err := containers.Request(context.Background(), unitOperator, old.ID.String(), domain.ContainerStart, "req-4"); !errors.Is(err, domain.ErrContainerRemoved) {
		t.Fatalf("removed = %v", err)
	}
	if _, err := containers.Request(context.Background(), unitOperator, web.ID.String(), "kill", "req-5"); !errors.Is(err, domain.ErrInvalidContainerVerb) {
		t.Fatalf("kill = %v", err)
	}
}

func TestContainerActionsAreRateLimitedAndNeedTheAudit(t *testing.T) {
	docker := &fakeDocker{}
	audit := &syncAudit{err: errors.New("disk full")}
	containers, web, _ := newContainerFixture(t, true, docker, audit)
	if _, err := containers.Request(context.Background(), unitOperator, web.ID.String(), domain.ContainerStart, "req"); err == nil {
		t.Fatal("an action without audit was accepted")
	}
	if _, found, _ := containers.Last(context.Background(), web.ID.String()); found || len(docker.calls) != 0 {
		t.Fatalf("an unaudited action left a trace: %v", docker.calls)
	}

	// A fresh usecase on a fixed clock: the refused request above counts in
	// the rate of its user.
	containers, web, _ = newContainerFixture(t, true, docker, &syncAudit{})
	containers.now = func() time.Time { return time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC) }
	for range domain.ContainerActionsPerMinute {
		if _, err := containers.Request(context.Background(), unitOperator, web.ID.String(), domain.ContainerStart, "req"); err != nil {
			t.Fatal(err)
		}
		waitContainerAction(t, containers, web.ID.String())
	}
	if _, err := containers.Request(context.Background(), unitOperator, web.ID.String(), domain.ContainerStart, "req"); !errors.Is(err, domain.ErrContainerActionsLimited) {
		t.Fatalf("31st action = %v", err)
	}
}
