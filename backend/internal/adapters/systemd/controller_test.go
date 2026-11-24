package systemd

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const controlledPath = dbus.ObjectPath("/org/freedesktop/systemd1/unit/nginx_2eservice")

// fakeManager exports the four verbs and GetUnit for one unit. A verb
// creates a job that disappears after jobLife; the unit then passes
// through the states of steps, one per poll, and stays on the last.
type fakeManager struct {
	mu      sync.Mutex
	conn    *dbus.Conn
	unit    *prop.Properties
	calls   []string
	fail    *dbus.Error
	jobLife time.Duration
	steps   []string
	// unloaded makes GetUnit answer NoSuchUnit once the job is gone.
	unloaded bool
	jobDone  bool
}

func (f *fakeManager) verb(method, name, mode string) (dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	f.calls = append(f.calls, method+" "+name+" "+mode)
	f.mu.Unlock()
	if f.fail != nil {
		return "", f.fail
	}
	if name != "nginx.service" {
		return "", dbus.NewError("org.freedesktop.systemd1.NoSuchUnit", []any{"Unit " + name + " not found."})
	}
	job := dbus.ObjectPath("/org/freedesktop/systemd1/job/7")
	if _, err := prop.Export(f.conn, job, prop.Map{jobInterface: {"State": {Value: "running"}}}); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	go func() {
		time.Sleep(f.jobLife)
		_ = f.conn.Export(nil, job, "org.freedesktop.DBus.Properties")
		f.mu.Lock()
		f.jobDone = true
		f.mu.Unlock()
	}()
	return job, nil
}

// recorded returns a copy of the calls received so far.
func (f *fakeManager) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeManager) StartUnit(name, mode string) (dbus.ObjectPath, *dbus.Error) {
	return f.verb("StartUnit", name, mode)
}

func (f *fakeManager) StopUnit(name, mode string) (dbus.ObjectPath, *dbus.Error) {
	return f.verb("StopUnit", name, mode)
}

func (f *fakeManager) RestartUnit(name, mode string) (dbus.ObjectPath, *dbus.Error) {
	return f.verb("RestartUnit", name, mode)
}

func (f *fakeManager) ReloadUnit(name, mode string) (dbus.ObjectPath, *dbus.Error) {
	return f.verb("ReloadUnit", name, mode)
}

// GetUnit moves the unit one step forward, so that each poll reads the
// next state.
func (f *fakeManager) GetUnit(name string) (dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "GetUnit "+name)
	if f.unloaded && f.jobDone {
		return "", dbus.NewError("org.freedesktop.systemd1.NoSuchUnit", []any{"not loaded"})
	}
	if len(f.steps) > 0 {
		f.unit.SetMust(unitInterface, "ActiveState", f.steps[0])
		if len(f.steps) > 1 {
			f.steps = f.steps[1:]
		}
	}
	return controlledPath, nil
}

func serveManager(t *testing.T, fake *fakeManager) Connector {
	t.Helper()
	address := startBus(t, nil)
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	fake.conn = conn
	if err := conn.Export(fake, managerPath, managerInterface); err != nil {
		t.Fatal(err)
	}
	fake.unit, err = prop.Export(conn, controlledPath, prop.Map{unitInterface: {
		"ActiveState": {Value: "inactive", Writable: true},
		"SubState":    {Value: "dead"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request name: %v %v", reply, err)
	}
	return connector(address)
}

func act(t *testing.T, fake *fakeManager, name string, verb domain.UnitVerb, timeout time.Duration) (domain.UnitStableState, error) {
	t.Helper()
	controller := NewController(serveManager(t, fake))
	controller.poll = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return controller.Act(ctx, name, verb)
}

func actionFailure(err error) domain.UnitActionFailure {
	var actionErr *domain.UnitActionError
	if errors.As(err, &actionErr) {
		return actionErr.Failure
	}
	return ""
}

func TestControllerCallsOnlyTheRequestedVerb(t *testing.T) {
	for verb, method := range verbMethods {
		fake := &fakeManager{jobLife: 30 * time.Millisecond, steps: []string{"active"}}
		state, err := act(t, fake, "nginx.service", verb, 5*time.Second)
		if err != nil || state.ActiveState != "active" {
			t.Fatalf("%s = (%+v, %v)", verb, state, err)
		}
		calls := fake.recorded()
		if calls[0] != method+" nginx.service replace" {
			t.Fatalf("%s called %v", verb, calls)
		}
		for _, call := range calls[1:] {
			if !slices.Contains([]string{"GetUnit nginx.service"}, call) {
				t.Fatalf("%s made an unexpected call %q", verb, call)
			}
		}
	}
}

func TestControllerWaitsForTheJobThenAStableState(t *testing.T) {
	fake := &fakeManager{jobLife: 100 * time.Millisecond, steps: []string{"activating", "activating", "active"}}
	started := time.Now()
	state, err := act(t, fake, "nginx.service", domain.UnitRestart, 5*time.Second)
	if err != nil || state != (domain.UnitStableState{ActiveState: "active", SubState: "dead"}) {
		t.Fatalf("restart = (%+v, %v)", state, err)
	}
	if time.Since(started) < 100*time.Millisecond {
		t.Fatal("the controller did not wait for the job")
	}
	if got := len(slices.DeleteFunc(fake.recorded(), func(c string) bool { return c != "GetUnit nginx.service" })); got != 3 {
		t.Fatalf("GetUnit calls = %d, want 3 (two transitional states, then stable)", got)
	}
}

func TestControllerReportsAFailedUnit(t *testing.T) {
	fake := &fakeManager{jobLife: 20 * time.Millisecond, steps: []string{"failed"}}
	state, err := act(t, fake, "nginx.service", domain.UnitStart, 5*time.Second)
	if actionFailure(err) != domain.ActionUnitFailed || state.ActiveState != "failed" {
		t.Fatalf("start = (%+v, %v), want failed", state, err)
	}
}

func TestControllerTreatsAnUnloadedUnitAsStopped(t *testing.T) {
	fake := &fakeManager{jobLife: 20 * time.Millisecond, unloaded: true}
	state, err := act(t, fake, "nginx.service", domain.UnitStop, 5*time.Second)
	if err != nil || state.ActiveState != "inactive" {
		t.Fatalf("stop = (%+v, %v)", state, err)
	}
}

func TestControllerReportsEachFailure(t *testing.T) {
	cases := map[string]struct {
		fake *fakeManager
		unit string
		want domain.UnitActionFailure
	}{
		"polkit refusal": {&fakeManager{fail: dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []any{"denied"})}, "nginx.service", domain.ActionNotAllowed},
		"interactive":    {&fakeManager{fail: dbus.NewError("org.freedesktop.DBus.Error.InteractiveAuthorizationRequired", []any{"auth"})}, "nginx.service", domain.ActionNotAllowed},
		"unknown unit":   {&fakeManager{}, "missing.service", domain.ActionNotFound},
		"masked":         {&fakeManager{fail: dbus.NewError("org.freedesktop.systemd1.UnitMasked", []any{"masked"})}, "nginx.service", domain.ActionRejected},
		"not applicable": {&fakeManager{fail: dbus.NewError("org.freedesktop.systemd1.JobTypeNotApplicable", []any{"no reload"})}, "nginx.service", domain.ActionRejected},
		"stuck":          {&fakeManager{jobLife: 10 * time.Millisecond, steps: []string{"activating"}}, "nginx.service", domain.ActionTimeout},
		"job never ends": {&fakeManager{jobLife: time.Hour}, "nginx.service", domain.ActionTimeout},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := act(t, tc.fake, tc.unit, domain.UnitReload, 300*time.Millisecond)
			if got := actionFailure(err); got != tc.want {
				t.Fatalf("failure = %q (%v), want %q", got, err, tc.want)
			}
		})
	}
}

func TestControllerReportsABusWithoutSystemd(t *testing.T) {
	controller := NewController(connector(startBus(t, nil)))
	if _, err := controller.Act(context.Background(), "nginx.service", domain.UnitStart); actionFailure(err) != domain.ActionUnavailable {
		t.Fatalf("Act = %v, want unavailable", err)
	}
	unreachable := NewController(func(context.Context) (*dbus.Conn, error) { return nil, errors.New("no socket") })
	if _, err := unreachable.Act(context.Background(), "nginx.service", domain.UnitStart); actionFailure(err) != domain.ActionUnavailable {
		t.Fatalf("Act = %v, want unavailable", err)
	}
}

func TestControllerRefusesAnUnknownVerbWithoutConnecting(t *testing.T) {
	controller := NewController(func(context.Context) (*dbus.Conn, error) {
		t.Fatal("connected for an invalid verb")
		return nil, nil
	})
	if _, err := controller.Act(context.Background(), "nginx.service", "enable"); !errors.Is(err, domain.ErrInvalidUnitVerb) {
		t.Fatalf("Act = %v, want ErrInvalidUnitVerb", err)
	}
}
