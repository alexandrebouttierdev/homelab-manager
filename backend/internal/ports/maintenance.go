package ports

import (
	"context"

	"monoops/backend/internal/domain"
)

// MaintenanceStore keeps the settings and the last run of the tasks of the
// catalog (D-1). A task never saved is absent.
type MaintenanceStore interface {
	MaintenanceTasks(ctx context.Context) ([]domain.MaintenanceTask, error)
	// SaveMaintenanceTask stores the settings and the audit entry atomically.
	SaveMaintenanceTask(ctx context.Context, task domain.MaintenanceTask, audit domain.AuditEntry) error
	RecordMaintenanceRun(ctx context.Context, kind domain.MaintenanceKind, run domain.MaintenanceRun) error
}
