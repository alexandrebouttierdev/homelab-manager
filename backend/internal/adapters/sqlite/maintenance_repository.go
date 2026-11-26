package sqlite

import (
	"context"
	"fmt"
	"time"

	"monoops/backend/ent"
	"monoops/backend/ent/maintenancetask"
	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"
)

var _ ports.MaintenanceStore = (*MaintenanceRepository)(nil)

// MaintenanceRepository keeps the tasks of the maintenance catalog (D-1).
type MaintenanceRepository struct {
	client *ent.Client
}

func NewMaintenanceRepository(client *ent.Client) *MaintenanceRepository {
	return &MaintenanceRepository{client: client}
}

func (r *MaintenanceRepository) MaintenanceTasks(ctx context.Context) ([]domain.MaintenanceTask, error) {
	records, err := r.client.MaintenanceTask.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list maintenance tasks: %w", err)
	}
	tasks := make([]domain.MaintenanceTask, 0, len(records))
	for _, record := range records {
		task := domain.MaintenanceTask{
			Kind: domain.MaintenanceKind(record.ID),
			Schedule: domain.BackupSchedule{
				Enabled: record.Enabled, Frequency: domain.BackupFrequency(record.Frequency),
				Weekday: time.Weekday(record.Weekday), Hour: record.Hour, Minute: record.Minute,
			},
			MaxSizeMiB: record.MaxSizeMib, UpdatedAt: record.UpdatedAt, UpdatedBy: record.UpdatedBy,
		}
		if record.LastRunAt != nil && record.LastSucceeded != nil {
			task.LastRun = &domain.MaintenanceRun{
				At: record.LastRunAt.UTC(), Origin: domain.MaintenanceOrigin(record.LastOrigin),
				Succeeded: *record.LastSucceeded, Message: record.LastMessage,
			}
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (r *MaintenanceRepository) SaveMaintenanceTask(ctx context.Context, task domain.MaintenanceTask, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		updated := time.Now().UTC()
		if task.UpdatedAt != nil {
			updated = task.UpdatedAt.UTC()
		}
		exists, err := tx.MaintenanceTask.Query().Where(maintenancetask.ID(string(task.Kind))).Exist(ctx)
		if err != nil {
			return fmt.Errorf("find maintenance task: %w", err)
		}
		if exists {
			err = tx.MaintenanceTask.UpdateOneID(string(task.Kind)).
				SetEnabled(task.Schedule.Enabled).SetFrequency(string(task.Schedule.Frequency)).
				SetWeekday(int(task.Schedule.Weekday)).SetHour(task.Schedule.Hour).SetMinute(task.Schedule.Minute).
				SetMaxSizeMib(task.MaxSizeMiB).SetUpdatedAt(updated).SetUpdatedBy(task.UpdatedBy).Exec(ctx)
		} else {
			err = tx.MaintenanceTask.Create().SetID(string(task.Kind)).
				SetEnabled(task.Schedule.Enabled).SetFrequency(string(task.Schedule.Frequency)).
				SetWeekday(int(task.Schedule.Weekday)).SetHour(task.Schedule.Hour).SetMinute(task.Schedule.Minute).
				SetMaxSizeMib(task.MaxSizeMiB).SetUpdatedAt(updated).SetUpdatedBy(task.UpdatedBy).Exec(ctx)
		}
		if err != nil {
			return fmt.Errorf("save maintenance task: %w", err)
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *MaintenanceRepository) RecordMaintenanceRun(ctx context.Context, kind domain.MaintenanceKind, run domain.MaintenanceRun) error {
	updated, err := r.client.MaintenanceTask.Update().Where(maintenancetask.ID(string(kind))).SetLastRunAt(run.At.UTC()).SetLastSucceeded(run.Succeeded).SetLastMessage(run.Message).
		SetLastOrigin(string(run.Origin)).Save(ctx)
	if err != nil {
		return fmt.Errorf("record maintenance run: %w", err)
	}
	if updated == 0 {
		// A task run before any setting was saved: keep its defaults.
		defaults := domain.DefaultMaintenanceTask(kind)
		return r.client.MaintenanceTask.Create().SetID(string(kind)).SetFrequency(string(defaults.Schedule.Frequency)).
			SetWeekday(int(defaults.Schedule.Weekday)).SetHour(defaults.Schedule.Hour).SetMaxSizeMib(defaults.MaxSizeMiB).
			SetLastRunAt(run.At.UTC()).SetLastSucceeded(run.Succeeded).SetLastMessage(run.Message).
			SetLastOrigin(string(run.Origin)).Exec(ctx)
	}
	return nil
}
