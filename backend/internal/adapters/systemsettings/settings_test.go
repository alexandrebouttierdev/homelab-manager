package systemsettings

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"homelab-manager/backend/internal/adapters/dbustest"
	"homelab-manager/backend/internal/domain"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

// fakeServices exports hostnamed, timedated and localed on a private bus:
// their properties and the only methods the adapter may call.
type fakeServices struct {
	mu       sync.Mutex
	calls    []string
	refuse   bool
	hostname *prop.Properties
	timedate *prop.Properties
	locale   *prop.Properties
}

func (f *fakeServices) record(call string) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if f.refuse {
		return dbus.NewError("org.freedesktop.DBus.Error.InteractiveAuthorizationRequired", []any{"Interactive authentication required."})
	}
	return nil
}

func (f *fakeServices) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

type hostnamed struct{ f *fakeServices }

func (h hostnamed) SetStaticHostname(name string, interactive bool) *dbus.Error {
	if err := h.f.record("SetStaticHostname " + name + " " + boolText(interactive)); err != nil {
		return err
	}
	h.f.hostname.SetMust(hostnameBus, "StaticHostname", name)
	return nil
}

type timedated struct{ f *fakeServices }

func (t timedated) SetTimezone(name string, interactive bool) *dbus.Error {
	if err := t.f.record("SetTimezone " + name + " " + boolText(interactive)); err != nil {
		return err
	}
	if name == "Mars/Olympus" {
		return dbus.NewError("org.freedesktop.timedate1.InvalidTimeZone", []any{"Invalid or not installed time zone 'Mars/Olympus'"})
	}
	t.f.timedate.SetMust(timedateBus, "Timezone", name)
	return nil
}

func (t timedated) ListTimezones() ([]string, *dbus.Error) {
	t.f.record("ListTimezones")
	return []string{"Europe/Paris", "UTC", "America/New_York", "../etc/passwd"}, nil
}

type localed struct{ f *fakeServices }

func (l localed) SetLocale(locale []string, interactive bool) *dbus.Error {
	if err := l.f.record("SetLocale " + joinText(locale) + " " + boolText(interactive)); err != nil {
		return err
	}
	l.f.locale.SetMust(localeBus, "Locale", locale)
	return nil
}

func boolText(value bool) string {
	if value {
		return "interactive"
	}
	return "non-interactive"
}

func joinText(values []string) string {
	text := ""
	for i, value := range values {
		if i > 0 {
			text += ","
		}
		text += value
	}
	return text
}

func serve(t *testing.T) (*Settings, *fakeServices) {
	t.Helper()
	address := dbustest.StartBus(t, nil)
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	fake := &fakeServices{}
	exports := []struct {
		bus, iface string
		path       dbus.ObjectPath
		object     any
		props      map[string]*prop.Prop
		target     **prop.Properties
	}{
		{hostnameBus, hostnameBus, hostnamePath, hostnamed{fake}, map[string]*prop.Prop{
			"StaticHostname": {Value: "homelab", Writable: true}, "Hostname": {Value: "homelab"},
		}, &fake.hostname},
		{timedateBus, timedateBus, timedatePath, timedated{fake}, map[string]*prop.Prop{
			"Timezone": {Value: "Europe/Paris", Writable: true},
		}, &fake.timedate},
		{localeBus, localeBus, localePath, localed{fake}, map[string]*prop.Prop{
			"Locale": {Value: []string{"LANG=fr_FR.UTF-8", "LC_TIME=en_GB.UTF-8"}, Writable: true},
		}, &fake.locale},
	}
	for _, export := range exports {
		if err := conn.Export(export.object, export.path, export.iface); err != nil {
			t.Fatal(err)
		}
		properties, err := prop.Export(conn, export.path, prop.Map{export.iface: export.props})
		if err != nil {
			t.Fatal(err)
		}
		*export.target = properties
		if reply, err := conn.RequestName(export.bus, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
			t.Fatalf("request %s: %v %v", export.bus, reply, err)
		}
	}
	return New(func(ctx context.Context) (*dbus.Conn, error) {
		return dbus.Connect(address, dbus.WithContext(ctx))
	}), fake
}

func TestReadAndChangeTheSystemPreferences(t *testing.T) {
	settings, fake := serve(t)
	ctx := context.Background()
	preferences, err := settings.Preferences(ctx)
	if err != nil || preferences != (domain.SystemPreferences{Hostname: "homelab", Timezone: "Europe/Paris", Locale: "fr_FR.UTF-8"}) {
		t.Fatalf("preferences = %+v, %v", preferences, err)
	}
	zones, err := settings.Timezones(ctx)
	if err != nil || !slices.Equal(zones, []string{"America/New_York", "Europe/Paris", "UTC"}) {
		t.Fatalf("time zones = %v, %v", zones, err)
	}
	if err := settings.SetHostname(ctx, "nas"); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetTimezone(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetLocale(ctx, "en_US.UTF-8"); err != nil {
		t.Fatal(err)
	}
	preferences, err = settings.Preferences(ctx)
	if err != nil || preferences != (domain.SystemPreferences{Hostname: "nas", Timezone: "UTC", Locale: "en_US.UTF-8"}) {
		t.Fatalf("after the changes = %+v, %v", preferences, err)
	}
	want := []string{
		"ListTimezones",
		"SetStaticHostname nas non-interactive",
		"SetTimezone UTC non-interactive",
		"SetLocale LANG=en_US.UTF-8,LC_TIME=en_GB.UTF-8 non-interactive",
	}
	if got := fake.recorded(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestChangesAreClassified(t *testing.T) {
	settings, fake := serve(t)
	ctx := context.Background()
	if err := settings.SetTimezone(ctx, "Mars/Olympus"); !errors.Is(err, domain.ErrSystemValueRejected) {
		t.Fatalf("unknown time zone = %v", err)
	}
	fake.mu.Lock()
	fake.refuse = true
	fake.mu.Unlock()
	if err := settings.SetHostname(ctx, "nas"); !errors.Is(err, domain.ErrSystemChangeRefused) {
		t.Fatalf("refused by polkit = %v", err)
	}
	calls := len(fake.recorded())
	for _, err := range []error{
		settings.SetHostname(ctx, "-bad"),
		settings.SetTimezone(ctx, "../../etc/passwd"),
		settings.SetLocale(ctx, "fr_FR.UTF-8; rm"),
	} {
		if err == nil || errors.Is(err, domain.ErrSystemChangeRefused) {
			t.Fatalf("an invalid value reached the bus: %v", err)
		}
	}
	if len(fake.recorded()) != calls {
		t.Fatal("an invalid value was sent")
	}
}

func TestUnreachableBus(t *testing.T) {
	settings := New(func(context.Context) (*dbus.Conn, error) { return nil, errors.New("no bus") })
	if _, err := settings.Preferences(context.Background()); !errors.Is(err, domain.ErrSystemSettingsUnavailable) {
		t.Fatalf("unreachable = %v", err)
	}
}
