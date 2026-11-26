package ports

import (
	"context"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

// SecurityRepository stores the security policy (SPEC_ADMINISTRATION.md § 3.4).
type SecurityRepository interface {
	// SecurityPolicy returns the default policy when none was stored.
	SecurityPolicy(ctx context.Context) (domain.SecurityPolicy, error)
	// SetSecurityPolicy stores the policy and its audit entry atomically.
	SetSecurityPolicy(ctx context.Context, policy domain.SecurityPolicy, audit domain.AuditEntry) error
}

// LoginFailureRepository stores the consecutive failed passwords of each
// account.
type LoginFailureRepository interface {
	// LoginFailures returns zero failures for an account that has none.
	LoginFailures(ctx context.Context, userID uuid.UUID) (domain.LoginFailures, error)
	SaveLoginFailures(ctx context.Context, userID uuid.UUID, failures domain.LoginFailures, at time.Time) error
	ClearLoginFailures(ctx context.Context, userID uuid.UUID) error
}

// AuditReader reads the audit log.
type AuditReader interface {
	// ListAudit returns the matching records newest first; the filter is
	// already validated.
	ListAudit(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, error)
}
