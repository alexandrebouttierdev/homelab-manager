package systemd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"

	"github.com/godbus/dbus/v5"
)

const (
	jobInterface = "org.freedesktop.systemd1.Job"
	propertyGet  = "org.freedesktop.DBus.Properties.Get"
	// actionPoll is how often the job, then the unit state, are read.
	actionPoll = time.Second
)

// verbMethods are the only Manager methods that change a unit the Manager
// ever calls (SPEC_ADMINISTRATION.md § 5.2); polkit names them by the same
// verbs.
var verbMethods = map[domain.UnitVerb]string{
	domain.UnitStart:   "StartUnit",
	domain.UnitStop:    "StopUnit",
	domain.UnitRestart: "RestartUnit",
	domain.UnitReload:  "ReloadUnit",
}

// refused are the D-Bus errors by which polkit declines.
var refused = map[string]bool{
	"org.freedesktop.DBus.Error.AccessDenied":                     true,
	"org.freedesktop.DBus.Error.InteractiveAuthorizationRequired": true,
}

// unknownUnit are the D-Bus errors of a unit systemd cannot find or load.
var unknownUnit = map[string]bool{
	"org.freedesktop.systemd1.NoSuchUnit": true,
	"org.freedesktop.systemd1.LoadFailed": true,
}

// jobGone are the D-Bus errors of a finished job: systemd removes the job
// object once it ends.
var jobGone = map[string]bool{
	"org.freedesktop.DBus.Error.UnknownObject":    true,
	"org.freedesktop.DBus.Error.UnknownInterface": true,
	"org.freedesktop.DBus.Error.UnknownMethod":    true,
	"org.freedesktop.systemd1.NoSuchJob":          true,
}

// transitional are the active states systemd leaves on its own.
var transitional = map[string]bool{
	"activating": true, "deactivating": true, "reloading": true,
}

var _ ports.UnitController = (*Controller)(nil)

// Controller asks systemd to start, stop, restart or reload one unit and
// follows it to a stable state. It calls one of the four methods of
// verbMethods with the mode "replace" and never asks polkit for an
// interactive authentication; everything else is a read.
type Controller struct {
	connect Connector
	poll    time.Duration
}

func NewController(connect Connector) *Controller {
	return &Controller{connect: connect, poll: actionPoll}
}

// Act requests the verb on the unit and returns its state once the job is
// gone and the unit is neither activating, deactivating nor reloading. The
// context bounds the whole action. Failures are *domain.UnitActionError; a
// unit that ends failed returns its state with ActionUnitFailed.
func (c *Controller) Act(ctx context.Context, name string, verb domain.UnitVerb) (domain.UnitStableState, error) {
	method, ok := verbMethods[verb]
	if !ok {
		return domain.UnitStableState{}, domain.ErrInvalidUnitVerb
	}
	conn, err := c.connect(ctx)
	if err != nil {
		return domain.UnitStableState{}, actionError(ctx, domain.ActionUnavailable, fmt.Errorf("connect to the system bus: %w", err))
	}
	defer func() { _ = conn.Close() }()
	var job dbus.ObjectPath
	// The flag 0 never asks polkit for an interactive authentication.
	err = conn.Object(busName, managerPath).CallWithContext(ctx, managerInterface+"."+method, 0, name, "replace").Store(&job)
	if err != nil {
		return domain.UnitStableState{}, classifyAction(ctx, err)
	}
	if err := c.waitJob(ctx, conn, job); err != nil {
		return domain.UnitStableState{}, err
	}
	return c.waitStable(ctx, conn, name)
}

func (c *Controller) waitJob(ctx context.Context, conn *dbus.Conn, job dbus.ObjectPath) error {
	for {
		var state dbus.Variant
		err := conn.Object(busName, job).CallWithContext(ctx, propertyGet, 0, jobInterface, "State").Store(&state)
		if err != nil {
			var dbusErr dbus.Error
			if errors.As(err, &dbusErr) && jobGone[dbusErr.Name] {
				return nil
			}
			return actionError(ctx, domain.ActionUnavailable, fmt.Errorf("read the job: %w", err))
		}
		if err := c.sleep(ctx); err != nil {
			return err
		}
	}
}

func (c *Controller) waitStable(ctx context.Context, conn *dbus.Conn, name string) (domain.UnitStableState, error) {
	for {
		var path dbus.ObjectPath
		err := conn.Object(busName, managerPath).CallWithContext(ctx, managerInterface+".GetUnit", 0, name).Store(&path)
		var dbusErr dbus.Error
		switch {
		case errors.As(err, &dbusErr) && dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit":
			// systemd unloads a stopped unit nothing refers to.
			return domain.UnitStableState{ActiveState: "inactive", SubState: "dead"}, nil
		case err != nil:
			return domain.UnitStableState{}, actionError(ctx, domain.ActionUnavailable, fmt.Errorf("find the unit: %w", err))
		}
		state, err := readState(ctx, conn.Object(busName, path))
		if err != nil {
			return domain.UnitStableState{}, actionError(ctx, domain.ActionUnavailable, fmt.Errorf("read the unit state: %w", err))
		}
		if !transitional[state.ActiveState] {
			if state.ActiveState == "failed" {
				return state, &domain.UnitActionError{Failure: domain.ActionUnitFailed, Err: fmt.Errorf("%s is failed (%s)", name, state.SubState)}
			}
			return state, nil
		}
		if err := c.sleep(ctx); err != nil {
			return state, err
		}
	}
}

func readState(ctx context.Context, unit dbus.BusObject) (domain.UnitStableState, error) {
	read := func(property string) (string, error) {
		var value dbus.Variant
		if err := unit.CallWithContext(ctx, propertyGet, 0, unitInterface, property).Store(&value); err != nil {
			return "", err
		}
		text, ok := value.Value().(string)
		if !ok {
			return "", fmt.Errorf("property %s is not a string", property)
		}
		return text, nil
	}
	active, err := read("ActiveState")
	if err != nil {
		return domain.UnitStableState{}, err
	}
	sub, err := read("SubState")
	if err != nil {
		return domain.UnitStableState{}, err
	}
	return domain.UnitStableState{ActiveState: active, SubState: sub}, nil
}

func (c *Controller) sleep(ctx context.Context) error {
	timer := time.NewTimer(c.poll)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return &domain.UnitActionError{Failure: domain.ActionTimeout, Err: ctx.Err()}
	case <-timer.C:
		return nil
	}
}

// classifyAction maps the error of the call that requests the verb.
func classifyAction(ctx context.Context, err error) error {
	var dbusErr dbus.Error
	if errors.As(err, &dbusErr) {
		switch {
		case refused[dbusErr.Name]:
			return &domain.UnitActionError{Failure: domain.ActionNotAllowed, Err: fmt.Errorf("polkit refused: %s", dbusErr.Name)}
		case unknownUnit[dbusErr.Name]:
			return &domain.UnitActionError{Failure: domain.ActionNotFound, Err: fmt.Errorf("%s: %v", dbusErr.Name, dbusErr.Body)}
		case unavailable[dbusErr.Name] || strings.HasPrefix(dbusErr.Name, spawnErrors):
			return &domain.UnitActionError{Failure: domain.ActionUnavailable, Err: fmt.Errorf("systemd is unavailable: %s", dbusErr.Name)}
		case strings.HasPrefix(dbusErr.Name, "org.freedesktop.systemd1."):
			return &domain.UnitActionError{Failure: domain.ActionRejected, Err: fmt.Errorf("%s: %v", dbusErr.Name, dbusErr.Body)}
		}
	}
	return actionError(ctx, domain.ActionUnavailable, fmt.Errorf("call systemd: %w", err))
}

// actionError reports a timeout when the context ended, the given failure
// otherwise.
func actionError(ctx context.Context, failure domain.UnitActionFailure, err error) error {
	if ctx.Err() != nil {
		return &domain.UnitActionError{Failure: domain.ActionTimeout, Err: ctx.Err()}
	}
	return &domain.UnitActionError{Failure: failure, Err: err}
}
