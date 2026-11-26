package ports

import (
	"context"

	"monoops/backend/internal/domain"
)

// SettingsRepository persists settings changed from the interface.
type SettingsRepository interface {
	// RetentionDays returns the stored retention, or nil when the service
	// configuration applies.
	RetentionDays(ctx context.Context) (*int, error)
	// SetRetentionDays stores the retention and its audit entry atomically.
	SetRetentionDays(ctx context.Context, days int, audit domain.AuditEntry) error
}
