package domain

import (
	"errors"
	"regexp"
	"strings"
)

// Bounds of the system preferences (SPEC_ADMINISTRATION.md, D-6).
const (
	// MaxHostnameLength is HOST_NAME_MAX of Linux.
	MaxHostnameLength = 64
	maxTimezoneLength = 64
	maxLocaleLength   = 64
)

var (
	ErrInvalidHostname = errors.New("hostname must be 1 to 64 letters, digits, hyphens and dots, in labels of at most 63 characters")
	ErrInvalidTimezone = errors.New("time zone must be a name of the tz database, such as Europe/Paris")
	ErrInvalidLocale   = errors.New("locale must look like fr_FR.UTF-8, C.UTF-8 or POSIX")
	// ErrSystemPreferencesDisabled: system_preferences_enabled is false.
	ErrSystemPreferencesDisabled = errors.New("system preferences are disabled in the service configuration")
	// ErrSystemChangeRefused: polkit refused (rule missing).
	ErrSystemChangeRefused = errors.New("polkit refused the change")
	// ErrSystemValueRejected: the systemd service refused the value (time
	// zone unknown, locale not installed…).
	ErrSystemValueRejected = errors.New("the system refused the value")
	// ErrSystemSettingsUnavailable: the system bus or a systemd service
	// cannot be reached.
	ErrSystemSettingsUnavailable = errors.New("system settings are unavailable")
)

var (
	hostnameLabel   = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	timezonePattern = regexp.MustCompile(`^[A-Za-z0-9_+-]+(/[A-Za-z0-9_+-]+){0,3}$`)
	localePattern   = regexp.MustCompile(`^([a-z]{2,3}(_[A-Z]{2})?(\.[A-Za-z0-9-]{1,20})?(@[a-z]{1,20})?|C(\.(UTF-8|utf8))?|POSIX)$`)
)

// SystemPreferences are the settings of the machine the Manager may read
// and, when enabled, change: its static host name, its time zone and its
// language (LANG).
type SystemPreferences struct {
	Hostname string
	Timezone string
	Locale   string
}

// SystemPreferencesChange is what an administrator asks: the three values
// as they should be. Only those that differ from the current ones change.
type SystemPreferencesChange struct {
	Hostname string
	Timezone string
	Locale   string
}

// SystemSetting names one of the three settings, in the audit and in a
// failure.
type SystemSetting string

const (
	SettingHostname SystemSetting = "hostname"
	SettingTimezone SystemSetting = "timezone"
	SettingLocale   SystemSetting = "locale"
)

// SystemChangeError reports the setting a change stopped at; the settings
// before it were changed.
type SystemChangeError struct {
	Setting SystemSetting
	Err     error
}

func (e *SystemChangeError) Error() string { return string(e.Setting) + ": " + e.Err.Error() }

func (e *SystemChangeError) Unwrap() error { return e.Err }

func ValidHostname(name string) bool {
	if name == "" || len(name) > MaxHostnameLength {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if !hostnameLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func ValidTimezone(name string) bool {
	return len(name) <= maxTimezoneLength && timezonePattern.MatchString(name)
}

func ValidLocale(name string) bool {
	return len(name) <= maxLocaleLength && localePattern.MatchString(name)
}

// Validate checks the three values; the systemd services check again that
// the time zone and the locale exist.
func (c SystemPreferencesChange) Validate() error {
	switch {
	case !ValidHostname(c.Hostname):
		return ErrInvalidHostname
	case !ValidTimezone(c.Timezone):
		return ErrInvalidTimezone
	case !ValidLocale(c.Locale):
		return ErrInvalidLocale
	}
	return nil
}
