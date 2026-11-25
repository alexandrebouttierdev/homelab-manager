// Package systemsettings reads and changes the host name, time zone and
// language of the machine through systemd-hostnamed, systemd-timedated and
// systemd-localed on the system D-Bus (D-6). polkit decides whether the
// Manager account may change them; the Manager never runs a command
// (hostnamectl, timedatectl, localectl) and never runs as root.
package systemsettings

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/godbus/dbus/v5"
)

const (
	hostnameBus  = "org.freedesktop.hostname1"
	hostnamePath = dbus.ObjectPath("/org/freedesktop/hostname1")
	timedateBus  = "org.freedesktop.timedate1"
	timedatePath = dbus.ObjectPath("/org/freedesktop/timedate1")
	localeBus    = "org.freedesktop.locale1"
	localePath   = dbus.ObjectPath("/org/freedesktop/locale1")
	propertyGet  = "org.freedesktop.DBus.Properties.Get"
	// callTimeout bounds each call; the services answer at once.
	callTimeout = 10 * time.Second
	// maxTimezones bounds the list read from timedated.
	maxTimezones = 1000
)

// refused are the D-Bus errors by which polkit declines.
var refused = map[string]bool{
	"org.freedesktop.DBus.Error.AccessDenied":                     true,
	"org.freedesktop.DBus.Error.InteractiveAuthorizationRequired": true,
}

// rejected are the errors of a value the service does not accept.
var rejected = map[string]bool{
	"org.freedesktop.DBus.Error.InvalidArgs":    true,
	"org.freedesktop.timedate1.InvalidTimeZone": true,
}

// Connector opens a connection to the bus where the services run.
type Connector func(ctx context.Context) (*dbus.Conn, error)

// SystemBus connects to the system bus.
func SystemBus(ctx context.Context) (*dbus.Conn, error) {
	return dbus.ConnectSystemBus(dbus.WithContext(ctx))
}

var _ ports.SystemSettings = (*Settings)(nil)

type Settings struct {
	connect Connector
}

func New(connect Connector) *Settings {
	return &Settings{connect: connect}
}

func (s *Settings) with(ctx context.Context, run func(context.Context, *dbus.Conn) error) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	conn, err := s.connect(ctx)
	if err != nil {
		return fmt.Errorf("%w: connect to the system bus: %v", domain.ErrSystemSettingsUnavailable, err)
	}
	defer func() { _ = conn.Close() }()
	return run(ctx, conn)
}

func property(ctx context.Context, conn *dbus.Conn, bus string, path dbus.ObjectPath, name string, target any) error {
	var variant dbus.Variant
	if err := conn.Object(bus, path).CallWithContext(ctx, propertyGet, 0, bus, name).Store(&variant); err != nil {
		return translate(err)
	}
	if err := dbus.Store([]any{variant.Value()}, target); err != nil {
		return fmt.Errorf("%w: %s.%s: %v", domain.ErrSystemSettingsUnavailable, bus, name, err)
	}
	return nil
}

func (s *Settings) Preferences(ctx context.Context) (domain.SystemPreferences, error) {
	var preferences domain.SystemPreferences
	err := s.with(ctx, func(ctx context.Context, conn *dbus.Conn) error {
		if err := property(ctx, conn, hostnameBus, hostnamePath, "StaticHostname", &preferences.Hostname); err != nil {
			return err
		}
		if preferences.Hostname == "" {
			// No static name: the machine runs with its transient one.
			if err := property(ctx, conn, hostnameBus, hostnamePath, "Hostname", &preferences.Hostname); err != nil {
				return err
			}
		}
		if err := property(ctx, conn, timedateBus, timedatePath, "Timezone", &preferences.Timezone); err != nil {
			return err
		}
		var locale []string
		if err := property(ctx, conn, localeBus, localePath, "Locale", &locale); err != nil {
			return err
		}
		preferences.Locale = lang(locale)
		return nil
	})
	return preferences, err
}

// lang returns the value of LANG in the locale of localed.
func lang(locale []string) string {
	for _, entry := range locale {
		if value, ok := strings.CutPrefix(entry, "LANG="); ok {
			return value
		}
	}
	return ""
}

func (s *Settings) Timezones(ctx context.Context) ([]string, error) {
	var zones []string
	err := s.with(ctx, func(ctx context.Context, conn *dbus.Conn) error {
		call := conn.Object(timedateBus, timedatePath).CallWithContext(ctx, timedateBus+".ListTimezones", 0)
		if err := call.Store(&zones); err != nil {
			return translate(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	valid := zones[:0]
	for _, zone := range zones {
		if domain.ValidTimezone(zone) {
			valid = append(valid, zone)
		}
	}
	sort.Strings(valid)
	if len(valid) > maxTimezones {
		valid = valid[:maxTimezones]
	}
	return valid, nil
}

// interactive = false in every call: polkit answers from its rules, never
// by prompting.

func (s *Settings) SetHostname(ctx context.Context, name string) error {
	if !domain.ValidHostname(name) {
		return domain.ErrInvalidHostname
	}
	return s.with(ctx, func(ctx context.Context, conn *dbus.Conn) error {
		return translate(conn.Object(hostnameBus, hostnamePath).CallWithContext(ctx, hostnameBus+".SetStaticHostname", 0, name, false).Err)
	})
}

func (s *Settings) SetTimezone(ctx context.Context, name string) error {
	if !domain.ValidTimezone(name) {
		return domain.ErrInvalidTimezone
	}
	return s.with(ctx, func(ctx context.Context, conn *dbus.Conn) error {
		return translate(conn.Object(timedateBus, timedatePath).CallWithContext(ctx, timedateBus+".SetTimezone", 0, name, false).Err)
	})
}

func (s *Settings) SetLocale(ctx context.Context, name string) error {
	if !domain.ValidLocale(name) {
		return domain.ErrInvalidLocale
	}
	return s.with(ctx, func(ctx context.Context, conn *dbus.Conn) error {
		var current []string
		if err := property(ctx, conn, localeBus, localePath, "Locale", &current); err != nil {
			return err
		}
		// SetLocale replaces the whole list: keep the LC_* variables.
		locale := []string{"LANG=" + name}
		for _, entry := range current {
			if !strings.HasPrefix(entry, "LANG=") {
				locale = append(locale, entry)
			}
		}
		return translate(conn.Object(localeBus, localePath).CallWithContext(ctx, localeBus+".SetLocale", 0, locale, false).Err)
	})
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	var dbusErr dbus.Error
	if errors.As(err, &dbusErr) {
		switch {
		case refused[dbusErr.Name]:
			return fmt.Errorf("%w: %s", domain.ErrSystemChangeRefused, dbusErr.Name)
		case rejected[dbusErr.Name]:
			return fmt.Errorf("%w: %s", domain.ErrSystemValueRejected, dbusErrorMessage(dbusErr))
		}
		return fmt.Errorf("%w: %s", domain.ErrSystemSettingsUnavailable, dbusErr.Name)
	}
	return fmt.Errorf("%w: %v", domain.ErrSystemSettingsUnavailable, err)
}

// dbusErrorMessage keeps the first line of the message of the service,
// bounded, for the logs.
func dbusErrorMessage(err dbus.Error) string {
	if len(err.Body) == 0 {
		return err.Name
	}
	message, _ := err.Body[0].(string)
	message, _, _ = strings.Cut(message, "\n")
	if len(message) > 200 {
		message = message[:200]
	}
	return err.Name + ": " + message
}
