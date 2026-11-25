package sqlite

import (
	"context"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"
)

func TestMaintenanceTasksRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	repository := NewMaintenanceRepository(database.Client())
	tasks, err := repository.MaintenanceTasks(ctx)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("tasks = %+v, %v", tasks, err)
	}
	at := time.Date(2026, 11, 25, 4, 0, 0, 0, time.UTC)
	// A run before any setting keeps the defaults.
	if err := repository.RecordMaintenanceRun(ctx, domain.MaintenanceJournalVacuum, domain.MaintenanceRun{At: at, Origin: domain.MaintenanceManual, Succeeded: true}); err != nil {
		t.Fatal(err)
	}
	task := domain.DefaultMaintenanceTask(domain.MaintenanceJournalVacuum)
	task.Schedule.Enabled, task.Schedule.Frequency, task.Schedule.Hour, task.MaxSizeMiB = true, domain.BackupDaily, 2, 1024
	task.UpdatedBy = "alice"
	audit := domain.AuditEntry{Actor: "alice", TargetType: "maintenance_task", TargetID: "journal-vacuum", Action: "update", Outcome: domain.AuditSuccess}
	if err := repository.SaveMaintenanceTask(ctx, task, audit); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordMaintenanceRun(ctx, domain.MaintenanceJournalVacuum, domain.MaintenanceRun{At: at.Add(time.Hour), Origin: domain.MaintenanceSchedule, Message: "unit failed"}); err != nil {
		t.Fatal(err)
	}
	tasks, err = repository.MaintenanceTasks(ctx)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %+v, %v", tasks, err)
	}
	got := tasks[0]
	if !got.Schedule.Enabled || got.Schedule.Frequency != domain.BackupDaily || got.Schedule.Hour != 2 || got.MaxSizeMiB != 1024 || got.UpdatedBy != "alice" {
		t.Fatalf("task = %+v", got)
	}
	if got.LastRun == nil || got.LastRun.Succeeded || got.LastRun.Origin != domain.MaintenanceSchedule || got.LastRun.Message != "unit failed" || !got.LastRun.At.Equal(at.Add(time.Hour)) {
		t.Fatalf("last run = %+v", got.LastRun)
	}
	count, err := database.Client().AuditLog.Query().Count(ctx)
	if err != nil || count != 1 {
		t.Fatalf("audit = %d, %v", count, err)
	}
}
