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

// syncAudit is an AuditRecorder safe for the action goroutines.
type syncAudit struct {
	mu      sync.Mutex
	entries []domain.AuditEntry
	err     error
}

func (a *syncAudit) RecordAudit(_ context.Context, entry domain.AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	a.entries = append(a.entries, entry)
	return nil
}

func (a *syncAudit) outcomes() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	outcomes := make([]string, 0, len(a.entries))
	for _, entry := range a.entries {
		outcomes = append(outcomes, entry.Outcome)
	}
	return outcomes
}

// fakeController answers each Act with its result once release is closed,
// or when the action context ends.
type fakeController struct {
	mu      sync.Mutex
	calls   []string
	release chan struct{}
	state   domain.UnitStableState
	err     error
}

func (f *fakeController) Act(ctx context.Context, name string, verb domain.UnitVerb) (domain.UnitStableState, error) {
	f.mu.Lock()
	f.calls = append(f.calls, string(verb)+" "+name)
	f.mu.Unlock()
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return domain.UnitStableState{}, &domain.UnitActionError{Failure: domain.ActionTimeout, Err: ctx.Err()}
		}
	}
	return f.state, f.err
}

var unitOperator = domain.User{ID: uuid.New(), Username: "olivia", Role: domain.RoleOperator}

// startUnitActions runs the usecase until the test ends.
func startUnitActions(t *testing.T, controller *fakeController, audit *syncAudit, units ...string) (*UnitActionUsecase, context.CancelFunc) {
	t.Helper()
	allowed, err := domain.NewManageableUnits(units)
	if err != nil {
		t.Fatal(err)
	}
	usecase := NewUnitActionUsecase(controller, audit, allowed, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		usecase.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	waitFor(t, func() bool {
		usecase.mu.Lock()
		defer usecase.mu.Unlock()
		return usecase.base != nil
	})
	return usecase, cancel
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func finished(t *testing.T, usecase *UnitActionUsecase, name string) domain.UnitAction {
	t.Helper()
	var action domain.UnitAction
	waitFor(t, func() bool {
		last, ok, _ := usecase.Last(name)
		action = last
		return ok && last.State != domain.ActionRunning
	})
	return action
}

func TestUnitActionIsAuditedFollowedAndKept(t *testing.T) {
	audit := &syncAudit{}
	controller := &fakeController{state: domain.UnitStableState{ActiveState: "active", SubState: "running"}}
	usecase, _ := startUnitActions(t, controller, audit, "nginx.service")
	action, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitRestart, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if action.State != domain.ActionRunning || action.RequestedBy != "olivia" || action.Verb != domain.UnitRestart {
		t.Fatalf("returned action = %+v", action)
	}
	done := finished(t, usecase, "nginx.service")
	if done.State != domain.ActionSucceeded || done.Final == nil || done.Final.ActiveState != "active" || done.FinishedAt == nil {
		t.Fatalf("finished action = %+v", done)
	}
	waitFor(t, func() bool { return len(audit.outcomes()) == 2 })
	audit.mu.Lock()
	first, last := audit.entries[0], audit.entries[1]
	audit.mu.Unlock()
	want := domain.AuditEntry{Actor: "olivia", TargetType: "system_unit", TargetID: "nginx.service", Action: "restart", RequestID: "req-1", Outcome: "requested"}
	if first != want {
		t.Fatalf("first audit = %+v, want %+v", first, want)
	}
	want.Outcome = "success"
	if last != want {
		t.Fatalf("last audit = %+v, want %+v", last, want)
	}
}

func TestUnitActionRefusesBeforeCallingSystemd(t *testing.T) {
	audit := &syncAudit{}
	controller := &fakeController{}
	usecase, _ := startUnitActions(t, controller, audit, "nginx.service")
	cases := map[string]struct {
		name string
		verb domain.UnitVerb
		want error
	}{
		"unknown verb":    {"nginx.service", "enable", domain.ErrInvalidUnitVerb},
		"invalid name":    {"../nginx.service", domain.UnitStart, domain.ErrInvalidUnitName},
		"not in the list": {"ssh.service", domain.UnitStop, domain.ErrUnitNotManageable},
	}
	for label, tc := range cases {
		if _, err := usecase.Request(context.Background(), unitOperator, tc.name, tc.verb, "req"); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", label, err, tc.want)
		}
	}
	if len(controller.calls) != 0 || len(audit.outcomes()) != 0 {
		t.Fatalf("calls = %v, audit = %v; want none", controller.calls, audit.outcomes())
	}
}

func TestUnitActionOneAtATimePerUnit(t *testing.T) {
	controller := &fakeController{release: make(chan struct{}), state: domain.UnitStableState{ActiveState: "active"}}
	usecase, _ := startUnitActions(t, controller, &syncAudit{}, "nginx.service", "redis.service")
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStop, "b"); !errors.Is(err, domain.ErrActionInProgress) {
		t.Fatalf("second action = %v, want ErrActionInProgress", err)
	}
	if _, err := usecase.Request(context.Background(), unitOperator, "redis.service", domain.UnitStart, "c"); err != nil {
		t.Fatalf("another unit = %v, want accepted", err)
	}
	close(controller.release)
	finished(t, usecase, "nginx.service")
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStop, "d"); err != nil {
		t.Fatalf("after the first ended = %v, want accepted", err)
	}
}

func TestUnitActionRateIsLimitedPerUser(t *testing.T) {
	controller := &fakeController{state: domain.UnitStableState{ActiveState: "active"}}
	usecase, _ := startUnitActions(t, controller, &syncAudit{}, "nginx.service")
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	usecase.mu.Lock()
	usecase.now = func() time.Time { return now }
	usecase.mu.Unlock()
	for i := 0; i < domain.UnitActionsPerMinute; i++ {
		if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "r"); err != nil {
			t.Fatalf("request %d = %v", i, err)
		}
		finished(t, usecase, "nginx.service")
	}
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "r"); !errors.Is(err, domain.ErrUnitActionsLimited) {
		t.Fatalf("request over the limit = %v, want ErrUnitActionsLimited", err)
	}
	other := domain.User{ID: uuid.New(), Username: "alice", Role: domain.RoleAdmin}
	if _, err := usecase.Request(context.Background(), other, "nginx.service", domain.UnitStart, "r"); err != nil {
		t.Fatalf("another user = %v, want accepted", err)
	}
	finished(t, usecase, "nginx.service")
	usecase.mu.Lock()
	usecase.now = func() time.Time { return now.Add(time.Minute) }
	usecase.mu.Unlock()
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "r"); err != nil {
		t.Fatalf("a minute later = %v, want accepted", err)
	}
}

func TestUnitActionReportsTheFailure(t *testing.T) {
	audit := &syncAudit{}
	controller := &fakeController{
		state: domain.UnitStableState{ActiveState: "failed", SubState: "failed"},
		err:   &domain.UnitActionError{Failure: domain.ActionUnitFailed, Err: errors.New("failed")},
	}
	usecase, _ := startUnitActions(t, controller, audit, "nginx.service")
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "r"); err != nil {
		t.Fatal(err)
	}
	done := finished(t, usecase, "nginx.service")
	if done.State != domain.ActionFailed || done.Failure != domain.ActionUnitFailed || done.Final.ActiveState != "failed" {
		t.Fatalf("action = %+v", done)
	}
	waitFor(t, func() bool { return len(audit.outcomes()) == 2 })
	if got := audit.outcomes(); got[1] != "failure" {
		t.Fatalf("audit outcomes = %v", got)
	}
}

func TestUnitActionTimesOut(t *testing.T) {
	controller := &fakeController{release: make(chan struct{})}
	usecase, _ := startUnitActions(t, controller, &syncAudit{}, "nginx.service")
	usecase.timeout = 50 * time.Millisecond
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "r"); err != nil {
		t.Fatal(err)
	}
	if done := finished(t, usecase, "nginx.service"); done.Failure != domain.ActionTimeout || done.Final != nil {
		t.Fatalf("action = %+v, want timeout", done)
	}
}

func TestUnitActionIsInterruptedWhenTheManagerStops(t *testing.T) {
	audit := &syncAudit{}
	controller := &fakeController{release: make(chan struct{})}
	usecase, stop := startUnitActions(t, controller, audit, "nginx.service")
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStop, "r"); err != nil {
		t.Fatal(err)
	}
	stop()
	if done := finished(t, usecase, "nginx.service"); done.Failure != domain.ActionInterrupted {
		t.Fatalf("action = %+v, want interrupted", done)
	}
	waitFor(t, func() bool { return len(audit.outcomes()) == 2 })
	waitFor(t, func() bool {
		usecase.mu.Lock()
		defer usecase.mu.Unlock()
		return usecase.base == nil
	})
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "r"); !errors.Is(err, domain.ErrUnitActionsUnavailable) {
		t.Fatalf("after stop = %v, want ErrUnitActionsUnavailable", err)
	}
}

func TestUnitActionWithoutAuditIsNotRequested(t *testing.T) {
	audit := &syncAudit{err: errors.New("disk full")}
	controller := &fakeController{}
	usecase, _ := startUnitActions(t, controller, audit, "nginx.service")
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "r"); err == nil {
		t.Fatal("an action that cannot be audited must be refused")
	}
	if len(controller.calls) != 0 {
		t.Fatalf("systemd was called: %v", controller.calls)
	}
	if _, ok, _ := usecase.Last("nginx.service"); ok {
		t.Fatal("the refused action must not be kept")
	}
	audit.mu.Lock()
	audit.err = nil
	audit.mu.Unlock()
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "r"); err != nil {
		t.Fatalf("after the audit recovered = %v", err)
	}
}

func TestUnitActionRefusedBeforeRun(t *testing.T) {
	allowed, _ := domain.NewManageableUnits([]string{"nginx.service"})
	usecase := NewUnitActionUsecase(&fakeController{}, &syncAudit{}, allowed, zap.NewNop())
	if _, err := usecase.Request(context.Background(), unitOperator, "nginx.service", domain.UnitStart, "r"); !errors.Is(err, domain.ErrUnitActionsUnavailable) {
		t.Fatalf("before Run = %v, want ErrUnitActionsUnavailable", err)
	}
	if got := usecase.ManageableUnits(); len(got) != 1 || got[0] != "nginx.service" {
		t.Fatalf("ManageableUnits = %v", got)
	}
}
