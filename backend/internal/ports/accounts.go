package ports

import (
	"context"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// AccountRepository stores the Manager accounts. Each change writes its
// audit entry atomically; the repository sets AuditEntry.TargetID.
type AccountRepository interface {
	// ListUsers returns at most limit accounts ordered by username.
	ListUsers(ctx context.Context, limit int) ([]domain.User, error)
	// GetUser returns domain.ErrUserNotFound for an unknown account.
	GetUser(ctx context.Context, id uuid.UUID) (domain.User, error)
	CountActiveAdmins(ctx context.Context) (int, error)
	// CreateUser returns domain.ErrUsernameTaken for a duplicate username;
	// the account must change its password at first login.
	CreateUser(ctx context.Context, user domain.NewUser, passwordHash string, audit domain.AuditEntry) (domain.User, error)
	UpdateUser(ctx context.Context, id uuid.UUID, change domain.UserChange, audit domain.AuditEntry) (domain.User, error)
	// ResetPassword stores a new initial password that must be changed at
	// the next login, and lifts a lockout of the account.
	ResetPassword(ctx context.Context, id uuid.UUID, passwordHash string, audit domain.AuditEntry) (domain.User, error)
	// DeleteUser also deletes the sessions of the account.
	DeleteUser(ctx context.Context, id uuid.UUID, audit domain.AuditEntry) error
}
