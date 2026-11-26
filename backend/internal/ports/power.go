package ports

import (
	"context"

	"monoops/backend/internal/domain"
)

// PowerController asks the operating system to reboot or power off.
type PowerController interface {
	// Request returns once the system accepted or refused the action; a
	// refusal (polkit, inhibitor lock, shutdown already running) wraps
	// domain.ErrPowerActionRefused.
	Request(ctx context.Context, action domain.PowerAction) error
}

// AuditRecorder writes an audit entry that is not part of another change,
// such as a request whose effect happens outside the database.
type AuditRecorder interface {
	RecordAudit(ctx context.Context, entry domain.AuditEntry) error
}
