package ports

import (
	"context"

	"monoops/backend/internal/domain"
)

// SystemSettings reads and changes the host name, time zone and language
// of the machine through systemd-hostnamed, -timedated and -localed (D-6).
// A refused change wraps domain.ErrSystemChangeRefused, a value the service
// rejects domain.ErrSystemValueRejected, and an unreachable bus
// domain.ErrSystemSettingsUnavailable.
type SystemSettings interface {
	Preferences(ctx context.Context) (domain.SystemPreferences, error)
	// Timezones lists the time zones the machine knows, sorted.
	Timezones(ctx context.Context) ([]string, error)
	SetHostname(ctx context.Context, name string) error
	SetTimezone(ctx context.Context, name string) error
	// SetLocale sets LANG and keeps the other LC_* variables.
	SetLocale(ctx context.Context, name string) error
	// LocaleEditable is false where the distribution forbids SetLocale
	// (Debian ships a D-Bus policy that denies it to every account).
	LocaleEditable() bool
}
