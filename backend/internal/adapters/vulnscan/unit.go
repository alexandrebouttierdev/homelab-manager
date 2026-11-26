package vulnscan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"monoops/backend/internal/domain"

	"github.com/godbus/dbus/v5"
)

const (
	busName          = "org.freedesktop.systemd1"
	managerPath      = dbus.ObjectPath("/org/freedesktop/systemd1")
	managerInterface = "org.freedesktop.systemd1.Manager"
	unitInterface    = "org.freedesktop.systemd1.Unit"
	jobInterface     = "org.freedesktop.systemd1.Job"
	propertyGet      = "org.freedesktop.DBus.Properties.Get"
	// pollInterval is how often the start job is checked.
	pollInterval = 2 * time.Second
)

// refused are the D-Bus errors by which polkit declines: no rule grants
// start on the unit to the Manager account.
var refused = map[string]bool{
	"org.freedesktop.DBus.Error.AccessDenied":                     true,
	"org.freedesktop.DBus.Error.InteractiveAuthorizationRequired": true,
}

// missing are the D-Bus errors of a unit not installed, or of a bus
// without systemd.
var missing = map[string]bool{
	"org.freedesktop.systemd1.NoSuchUnit":          true,
	"org.freedesktop.systemd1.LoadFailed":          true,
	"org.freedesktop.DBus.Error.ServiceUnknown":    true,
	"org.freedesktop.DBus.Error.NameHasNoOwner":    true,
	"org.freedesktop.DBus.Error.FileNotFound":      true,
	"org.freedesktop.DBus.Error.UnknownObject":     true,
	"org.freedesktop.DBus.Error.Spawn.ChildExited": true,
}

// jobGone are the D-Bus errors of a finished job: systemd answers
// UnknownObject once the job object is removed; a bus that keeps the path
// answers UnknownInterface.
var jobGone = map[string]bool{
	"org.freedesktop.DBus.Error.UnknownObject":    true,
	"org.freedesktop.DBus.Error.UnknownInterface": true,
	"org.freedesktop.systemd1.NoSuchJob":          true,
}

// Connector opens a connection to the bus where systemd runs.
type Connector func(ctx context.Context) (*dbus.Conn, error)

// SystemBus connects to the system bus.
func SystemBus(ctx context.Context) (*dbus.Conn, error) {
	return dbus.ConnectSystemBus(dbus.WithContext(ctx))
}

// UnitStarter starts one systemd unit and waits for its start job to end.
// It calls StartUnit on that unit only, with mode "replace", and reads the
// job and the unit state: nothing else (SPEC_ADMINISTRATION.md § 2.1). The
// phase 5 service actions reuse it.
type UnitStarter struct {
	connect Connector
	poll    time.Duration
}

func NewUnitStarter(connect Connector) *UnitStarter {
	return &UnitStarter{connect: connect, poll: pollInterval}
}

// Run starts the unit and returns once its start job is gone: for a
// Type=oneshot service, once the program exited. The context bounds the
// wait. A refusal is FailureNotAllowed, a unit that does not exist
// FailureNotInstalled, a unit that ended failed FailureScan (FailureTimeout
// when systemd stopped it at TimeoutStartSec=).
func (s *UnitStarter) Run(ctx context.Context, unit string) error {
	conn, err := s.connect(ctx)
	if err != nil {
		return &domain.ScanError{Failure: domain.FailureNotInstalled, Err: fmt.Errorf("connect to the system bus: %w", err)}
	}
	defer func() { _ = conn.Close() }()
	manager := conn.Object(busName, managerPath)
	var job dbus.ObjectPath
	// The flag 0 never asks polkit for an interactive authentication.
	if err := manager.CallWithContext(ctx, managerInterface+".StartUnit", 0, unit, "replace").Store(&job); err != nil {
		return classifyCall(err)
	}
	for {
		var state dbus.Variant
		err := conn.Object(busName, job).CallWithContext(ctx, propertyGet, 0, jobInterface, "State").Store(&state)
		if err != nil {
			var dbusErr dbus.Error
			if !errors.As(err, &dbusErr) || !jobGone[dbusErr.Name] {
				return timeoutOr(ctx, &domain.ScanError{Failure: domain.FailureScan, Err: fmt.Errorf("read the start job: %w", err)})
			}
			// The job is gone: the unit finished starting.
			return s.outcome(ctx, conn, unit)
		}
		select {
		case <-ctx.Done():
			return &domain.ScanError{Failure: domain.FailureTimeout, Err: ctx.Err()}
		case <-time.After(s.poll):
		}
	}
}

func (s *UnitStarter) outcome(ctx context.Context, conn *dbus.Conn, unit string) error {
	var path dbus.ObjectPath
	if err := conn.Object(busName, managerPath).CallWithContext(ctx, managerInterface+".GetUnit", 0, unit).Store(&path); err != nil {
		var dbusErr dbus.Error
		if errors.As(err, &dbusErr) && dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit" {
			// StartUnit found the unit, so it exists: systemd unloads a
			// static unit that ended inactive as soon as its job is gone,
			// while a failed unit stays loaded. The result file, written
			// after the old one was removed, is still checked by the
			// scanner.
			return nil
		}
		return classifyCall(err)
	}
	object := conn.Object(busName, path)
	read := func(name string) (string, error) {
		var value dbus.Variant
		if err := object.CallWithContext(ctx, propertyGet, 0, unitInterface, name).Store(&value); err != nil {
			return "", err
		}
		text, ok := value.Value().(string)
		if !ok {
			return "", fmt.Errorf("property %s is not a string", name)
		}
		return text, nil
	}
	active, err := read("ActiveState")
	if err != nil {
		return timeoutOr(ctx, &domain.ScanError{Failure: domain.FailureScan, Err: fmt.Errorf("read the unit state: %w", err)})
	}
	// Result is a property of the service interface; the unit interface
	// has none, so it is read from org.freedesktop.systemd1.Service.
	var result dbus.Variant
	if err := object.CallWithContext(ctx, propertyGet, 0, "org.freedesktop.systemd1.Service", "Result").Store(&result); err != nil {
		return timeoutOr(ctx, &domain.ScanError{Failure: domain.FailureScan, Err: fmt.Errorf("read the unit result: %w", err)})
	}
	outcome, _ := result.Value().(string)
	switch {
	case active != "failed" && outcome == "success":
		return nil
	case outcome == "timeout":
		return &domain.ScanError{Failure: domain.FailureTimeout, Err: fmt.Errorf("%s stopped at its start timeout", unit)}
	default:
		return &domain.ScanError{Failure: domain.FailureScan, Err: fmt.Errorf("%s ended %s (%s)", unit, active, outcome)}
	}
}

func classifyCall(err error) error {
	var dbusErr dbus.Error
	if errors.As(err, &dbusErr) {
		switch {
		case refused[dbusErr.Name]:
			return &domain.ScanError{Failure: domain.FailureNotAllowed, Err: fmt.Errorf("polkit refused: %s", dbusErr.Name)}
		case missing[dbusErr.Name]:
			return &domain.ScanError{Failure: domain.FailureNotInstalled, Err: fmt.Errorf("unit unavailable: %s", dbusErr.Name)}
		}
	}
	return &domain.ScanError{Failure: domain.FailureScan, Err: fmt.Errorf("call systemd: %w", err)}
}

func timeoutOr(ctx context.Context, err *domain.ScanError) error {
	if ctx.Err() != nil {
		return &domain.ScanError{Failure: domain.FailureTimeout, Err: ctx.Err()}
	}
	return err
}
