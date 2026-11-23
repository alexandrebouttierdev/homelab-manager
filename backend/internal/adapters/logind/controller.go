// Package logind asks systemd-logind, over the system D-Bus, to reboot or
// power off the machine. polkit decides whether the Manager account may do
// so; the Manager never runs a command and never runs as root.
package logind

import (
	"context"
	"errors"
	"fmt"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/godbus/dbus/v5"
)

const (
	busName    = "org.freedesktop.login1"
	objectPath = dbus.ObjectPath("/org/freedesktop/login1")
	// callTimeout bounds the call; logind answers as soon as the shutdown
	// job is queued, long before the machine stops.
	callTimeout = 10 * time.Second
)

var methods = map[domain.PowerAction]string{
	domain.PowerReboot:   "org.freedesktop.login1.Manager.Reboot",
	domain.PowerPowerOff: "org.freedesktop.login1.Manager.PowerOff",
}

// refusals are the D-Bus errors by which polkit or logind decline: no
// matching polkit rule, an inhibitor lock (the -ignore-inhibit actions are
// never granted), or a shutdown already running.
var refusals = map[string]bool{
	"org.freedesktop.DBus.Error.AccessDenied":                     true,
	"org.freedesktop.DBus.Error.InteractiveAuthorizationRequired": true,
	"org.freedesktop.login1.OperationInProgress":                  true,
}

var _ ports.PowerController = (*Controller)(nil)

type Controller struct{}

func New() *Controller { return &Controller{} }

func (c *Controller) Request(ctx context.Context, action domain.PowerAction) error {
	method, ok := methods[action]
	if !ok {
		return domain.ErrInvalidPowerAction
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("connect to the system bus: %w", err)
	}
	defer func() { _ = conn.Close() }()
	// interactive = false: polkit answers from its rules, never by prompting.
	call := conn.Object(busName, objectPath).CallWithContext(ctx, method, 0, false)
	return translate(call.Err)
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	var dbusErr dbus.Error
	if errors.As(err, &dbusErr) && refusals[dbusErr.Name] {
		return fmt.Errorf("%w: %s", domain.ErrPowerActionRefused, dbusErr.Name)
	}
	return fmt.Errorf("call logind: %w", err)
}
