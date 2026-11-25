package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"go.uber.org/zap"
)

const auditTargetSystem = "system"

// SystemPreferencesUsecase reads the host name, time zone and language of
// the machine and, when system_preferences_enabled is true, changes them
// (SPEC_ADMINISTRATION.md, D-6). Callers of Update must hold
// domain.PermManageSystem; the HTTP layer enforces it. The polkit rule of
// deployments/polkit must also be installed.
type SystemPreferencesUsecase struct {
	settings ports.SystemSettings
	audit    ports.AuditRecorder
	enabled  bool
	logger   *zap.Logger
	// changing allows one change at a time.
	changing sync.Mutex
}

func NewSystemPreferencesUsecase(settings ports.SystemSettings, audit ports.AuditRecorder, enabled bool, logger *zap.Logger) *SystemPreferencesUsecase {
	return &SystemPreferencesUsecase{settings: settings, audit: audit, enabled: enabled, logger: logger}
}

func (u *SystemPreferencesUsecase) Enabled() bool { return u.enabled }

// LocaleEditable reports whether the distribution lets systemd change the
// language.
func (u *SystemPreferencesUsecase) LocaleEditable() bool { return u.settings.LocaleEditable() }

// Preferences returns the current values and the time zones of the
// machine; a failure of the list leaves it empty.
func (u *SystemPreferencesUsecase) Preferences(ctx context.Context) (domain.SystemPreferences, []string, error) {
	preferences, err := u.settings.Preferences(ctx)
	if err != nil {
		return domain.SystemPreferences{}, nil, err
	}
	zones, err := u.settings.Timezones(ctx)
	if err != nil {
		u.logger.Warn("Could not list the time zones", zap.Error(err))
		zones = nil
	}
	return preferences, zones, nil
}

// Update changes the values that differ from the current ones, host name,
// then time zone, then language. Each change is audited before it is asked
// for; on a failure it stops and returns a *domain.SystemChangeError, the
// settings before it staying changed. It returns the settings changed.
func (u *SystemPreferencesUsecase) Update(ctx context.Context, actor domain.User, change domain.SystemPreferencesChange, requestID string) ([]domain.SystemSetting, error) {
	if !u.enabled {
		return nil, domain.ErrSystemPreferencesDisabled
	}
	if err := change.Validate(); err != nil {
		return nil, err
	}
	if !u.changing.TryLock() {
		return nil, domain.ErrActionInProgress
	}
	defer u.changing.Unlock()
	current, err := u.settings.Preferences(ctx)
	if err != nil {
		return nil, err
	}
	// Refused before any change, so that none is left half done.
	if change.Locale != current.Locale && !u.settings.LocaleEditable() {
		return nil, &domain.SystemChangeError{Setting: domain.SettingLocale, Err: domain.ErrSystemSettingReadOnly}
	}
	steps := []struct {
		setting        domain.SystemSetting
		current, value string
		set            func(context.Context, string) error
	}{
		{domain.SettingHostname, current.Hostname, change.Hostname, u.settings.SetHostname},
		{domain.SettingTimezone, current.Timezone, change.Timezone, u.settings.SetTimezone},
		{domain.SettingLocale, current.Locale, change.Locale, u.settings.SetLocale},
	}
	var changed []domain.SystemSetting
	for _, step := range steps {
		if step.value == step.current {
			continue
		}
		entry := domain.AuditEntry{
			Actor: actor.Username, TargetType: auditTargetSystem, TargetID: string(step.setting),
			Action: "set", RequestID: requestID, Outcome: domain.AuditRequested,
		}
		// No trace, no change.
		if err := u.audit.RecordAudit(ctx, entry); err != nil {
			return changed, &domain.SystemChangeError{Setting: step.setting, Err: fmt.Errorf("record audit: %w", err)}
		}
		setErr := step.set(ctx, step.value)
		entry.Outcome = domain.AuditSuccess
		if setErr != nil {
			entry.Outcome = domain.AuditFailure
		}
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		if err := u.audit.RecordAudit(auditCtx, entry); err != nil {
			u.logger.Error("Could not audit the outcome of a system change", zap.String("setting", string(step.setting)), zap.Error(err))
		}
		cancel()
		fields := []zap.Field{
			zap.String("setting", string(step.setting)), zap.String("from", step.current), zap.String("to", step.value),
			zap.String("actor", actor.Username), zap.String("request_id", requestID),
		}
		if setErr != nil {
			u.logger.Warn("System change failed", append(fields, zap.Error(setErr))...)
			return changed, &domain.SystemChangeError{Setting: step.setting, Err: setErr}
		}
		u.logger.Info("System setting changed", fields...)
		changed = append(changed, step.setting)
	}
	return changed, nil
}
