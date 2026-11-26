package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"

	"monoops/backend/internal/domain"

	"go.uber.org/zap"
)

type fakeSystemSettings struct {
	current        domain.SystemPreferences
	calls          []string
	fail           map[string]error
	localeReadOnly bool
}

func (f *fakeSystemSettings) LocaleEditable() bool { return !f.localeReadOnly }

func (f *fakeSystemSettings) Preferences(context.Context) (domain.SystemPreferences, error) {
	return f.current, f.fail["read"]
}

func (f *fakeSystemSettings) Timezones(context.Context) ([]string, error) {
	return []string{"Europe/Paris", "UTC"}, f.fail["zones"]
}

func (f *fakeSystemSettings) set(name, value string, apply func()) error {
	f.calls = append(f.calls, name+" "+value)
	if err := f.fail[name]; err != nil {
		return err
	}
	apply()
	return nil
}

func (f *fakeSystemSettings) SetHostname(_ context.Context, value string) error {
	return f.set("hostname", value, func() { f.current.Hostname = value })
}

func (f *fakeSystemSettings) SetTimezone(_ context.Context, value string) error {
	return f.set("timezone", value, func() { f.current.Timezone = value })
}

func (f *fakeSystemSettings) SetLocale(_ context.Context, value string) error {
	return f.set("locale", value, func() { f.current.Locale = value })
}

var systemAdmin = domain.User{Username: "alice", Role: domain.RoleAdmin}

func TestSystemPreferencesChangeOnlyWhatDiffers(t *testing.T) {
	settings := &fakeSystemSettings{current: domain.SystemPreferences{Hostname: "monoops", Timezone: "Europe/Paris", Locale: "fr_FR.UTF-8"}}
	audit := &syncAudit{}
	preferences := NewSystemPreferencesUsecase(settings, audit, true, zap.NewNop())
	changed, err := preferences.Update(context.Background(), systemAdmin,
		domain.SystemPreferencesChange{Hostname: "nas", Timezone: "Europe/Paris", Locale: "en_US.UTF-8"}, "req")
	if err != nil || !slices.Equal(changed, []domain.SystemSetting{domain.SettingHostname, domain.SettingLocale}) {
		t.Fatalf("changed = %v, %v", changed, err)
	}
	if !slices.Equal(settings.calls, []string{"hostname nas", "locale en_US.UTF-8"}) {
		t.Fatalf("calls = %v", settings.calls)
	}
	if got := audit.outcomes(); !slices.Equal(got, []string{domain.AuditRequested, domain.AuditSuccess, domain.AuditRequested, domain.AuditSuccess}) {
		t.Fatalf("audit = %v", got)
	}
	if audit.entries[0].TargetType != "system" || audit.entries[0].TargetID != "hostname" || audit.entries[0].Action != "set" {
		t.Fatalf("audit entry = %+v", audit.entries[0])
	}
	if changed, err := preferences.Update(context.Background(), systemAdmin,
		domain.SystemPreferencesChange{Hostname: "nas", Timezone: "Europe/Paris", Locale: "en_US.UTF-8"}, "req"); err != nil || len(changed) != 0 {
		t.Fatalf("same values = %v, %v", changed, err)
	}
}

func TestSystemPreferencesStopAtTheFirstFailure(t *testing.T) {
	settings := &fakeSystemSettings{
		current: domain.SystemPreferences{Hostname: "monoops", Timezone: "Europe/Paris", Locale: "fr_FR.UTF-8"},
		fail:    map[string]error{"timezone": domain.ErrSystemValueRejected},
	}
	audit := &syncAudit{}
	preferences := NewSystemPreferencesUsecase(settings, audit, true, zap.NewNop())
	changed, err := preferences.Update(context.Background(), systemAdmin,
		domain.SystemPreferencesChange{Hostname: "nas", Timezone: "Mars/Olympus", Locale: "en_US.UTF-8"}, "req")
	var changeErr *domain.SystemChangeError
	if !errors.As(err, &changeErr) || changeErr.Setting != domain.SettingTimezone || !errors.Is(err, domain.ErrSystemValueRejected) {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(changed, []domain.SystemSetting{domain.SettingHostname}) || slices.Contains(settings.calls, "locale en_US.UTF-8") {
		t.Fatalf("changed = %v, calls %v", changed, settings.calls)
	}
	if got := audit.outcomes(); got[len(got)-1] != domain.AuditFailure {
		t.Fatalf("audit = %v", got)
	}
}

func TestSystemPreferencesNeedTheSettingAndTheAudit(t *testing.T) {
	settings := &fakeSystemSettings{current: domain.SystemPreferences{Hostname: "monoops", Timezone: "UTC", Locale: "C.UTF-8"}}
	change := domain.SystemPreferencesChange{Hostname: "nas", Timezone: "UTC", Locale: "C.UTF-8"}
	disabled := NewSystemPreferencesUsecase(settings, &syncAudit{}, false, zap.NewNop())
	if _, err := disabled.Update(context.Background(), systemAdmin, change, "req"); !errors.Is(err, domain.ErrSystemPreferencesDisabled) {
		t.Fatalf("disabled = %v", err)
	}
	broken := NewSystemPreferencesUsecase(settings, &syncAudit{err: errors.New("disk full")}, true, zap.NewNop())
	if _, err := broken.Update(context.Background(), systemAdmin, change, "req"); err == nil || len(settings.calls) != 0 {
		t.Fatalf("without audit = %v, calls %v", err, settings.calls)
	}
	invalid := domain.SystemPreferencesChange{Hostname: "-nas", Timezone: "UTC", Locale: "C.UTF-8"}
	if _, err := broken.Update(context.Background(), systemAdmin, invalid, "req"); !errors.Is(err, domain.ErrInvalidHostname) {
		t.Fatalf("invalid = %v", err)
	}
	preferences := NewSystemPreferencesUsecase(settings, &syncAudit{}, true, zap.NewNop())
	settings.fail = map[string]error{"zones": errors.New("no list")}
	current, zones, err := preferences.Preferences(context.Background())
	if err != nil || current.Hostname != "monoops" || zones != nil {
		t.Fatalf("preferences without zones = %+v, %v, %v", current, zones, err)
	}
}

// TestLockedLocaleIsRefusedBeforeAnyChange: on Debian the language cannot
// change through localed; nothing else changes either.
func TestLockedLocaleIsRefusedBeforeAnyChange(t *testing.T) {
	settings := &fakeSystemSettings{
		current:        domain.SystemPreferences{Hostname: "monoops", Timezone: "UTC", Locale: "en_US.UTF-8"},
		localeReadOnly: true,
	}
	preferences := NewSystemPreferencesUsecase(settings, &syncAudit{}, true, zap.NewNop())
	_, err := preferences.Update(context.Background(), systemAdmin,
		domain.SystemPreferencesChange{Hostname: "nas", Timezone: "UTC", Locale: "C.UTF-8"}, "req")
	if !errors.Is(err, domain.ErrSystemSettingReadOnly) || len(settings.calls) != 0 {
		t.Fatalf("locked locale = %v, calls %v", err, settings.calls)
	}
	changed, err := preferences.Update(context.Background(), systemAdmin,
		domain.SystemPreferencesChange{Hostname: "nas", Timezone: "UTC", Locale: "en_US.UTF-8"}, "req")
	if err != nil || !slices.Equal(changed, []domain.SystemSetting{domain.SettingHostname}) {
		t.Fatalf("same locale = %v, %v", changed, err)
	}
}
