package sqlite

import (
	"context"
	"fmt"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/ent/appsetting"
	"homelab-manager/backend/internal/domain"
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
