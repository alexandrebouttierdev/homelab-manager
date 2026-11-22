package ports

import (
	"context"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (domain.User, string, error)
	FindByID(ctx context.Context, userID uuid.UUID) (domain.User, error)
	UpdatePasswordHash(ctx context.Context, userID uuid.UUID, passwordHash string) error
	// EnsureAdmin creates the administrator when absent and reports whether it
	// did; an existing account (and its current password) is never modified.
	EnsureAdmin(ctx context.Context, username, passwordHash string) (bool, error)
}

type SessionRepository interface {
	Create(ctx context.Context, userID uuid.UUID, tokenHash, ip, userAgent string, expiresAt time.Time) (domain.Session, error)
	FindActiveByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error)
	RevokeByTokenHash(ctx context.Context, tokenHash string) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encodedHash string) (bool, error)
}
