package sqlite

import (
	"context"
	"fmt"

	"monoops/backend/ent"
	"monoops/backend/ent/appsetting"
	"monoops/backend/internal/domain"
)

const settingsSingletonKey = "local"

type SettingsRepository struct {
	client *ent.Client
}

func NewSettingsRepository(database *Database) *SettingsRepository {
	return &SettingsRepository{client: database.client}
}

func (r *SettingsRepository) RetentionDays(ctx context.Context) (*int, error) {
	record, err := r.client.AppSetting.Query().Where(appsetting.SingletonKey(settingsSingletonKey)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record.MetricsRetentionDays, nil
}

func (r *SettingsRepository) SetRetentionDays(ctx context.Context, days int, audit domain.AuditEntry) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := setRetention(ctx, tx, days, audit); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func setRetention(ctx context.Context, tx *ent.Tx, days int, audit domain.AuditEntry) error {
	record, err := tx.AppSetting.Query().Where(appsetting.SingletonKey(settingsSingletonKey)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		if _, err := tx.AppSetting.Create().SetSingletonKey(settingsSingletonKey).SetMetricsRetentionDays(days).Save(ctx); err != nil {
			return fmt.Errorf("create settings: %w", err)
		}
	case err != nil:
		return err
	default:
		if _, err := tx.AppSetting.UpdateOneID(record.ID).SetMetricsRetentionDays(days).Save(ctx); err != nil {
			return fmt.Errorf("update settings: %w", err)
		}
	}
	return writeAudit(ctx, tx, audit)
}

// VulnerabilityScanIntervalHours returns the interval chosen in the
// interface, or nil when the service configuration applies.
func (r *SettingsRepository) VulnerabilityScanIntervalHours(ctx context.Context) (*int, error) {
	record, err := r.client.AppSetting.Query().Where(appsetting.SingletonKey(settingsSingletonKey)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	return record.VulnerabilityScanIntervalHours, nil
}

// SetVulnerabilityScanIntervalHours stores the interval with its audit
// entry.
func (r *SettingsRepository) SetVulnerabilityScanIntervalHours(ctx context.Context, hours int, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		record, err := tx.AppSetting.Query().Where(appsetting.SingletonKey(settingsSingletonKey)).Only(ctx)
		switch {
		case ent.IsNotFound(err):
			if err := tx.AppSetting.Create().SetSingletonKey(settingsSingletonKey).SetVulnerabilityScanIntervalHours(hours).Exec(ctx); err != nil {
				return fmt.Errorf("create settings: %w", err)
			}
		case err != nil:
			return fmt.Errorf("read settings: %w", err)
		default:
			if err := tx.AppSetting.UpdateOneID(record.ID).SetVulnerabilityScanIntervalHours(hours).Exec(ctx); err != nil {
				return fmt.Errorf("update settings: %w", err)
			}
		}
		return writeAudit(ctx, tx, audit)
	})
}
