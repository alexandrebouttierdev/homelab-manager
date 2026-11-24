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
	// RequirePasswordChange makes the account choose a new password at
	// its next login.
	RequirePasswordChange(ctx context.Context, userID uuid.UUID) error
	// RecordLogin stores the time and source address of a successful login.
	RecordLogin(ctx context.Context, userID uuid.UUID, ip string, at time.Time) error
}

type SessionRepository interface {
	// Create opens a session; authMethod is "password", "password+totp" or
	// "password+recovery".
	Create(ctx context.Context, userID uuid.UUID, tokenHash, ip, userAgent, authMethod string, expiresAt time.Time) (domain.Session, error)
	FindActiveByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error)
	RevokeByTokenHash(ctx context.Context, tokenHash string) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
	// FindActiveByID returns domain.ErrSessionNotFound for a revoked or
	// expired session.
	FindActiveByID(ctx context.Context, id uuid.UUID) (domain.Session, error)
	// ListActive returns at most limit active sessions, most recently seen
	// first; a nil userID lists the sessions of every account.
	ListActive(ctx context.Context, userID *uuid.UUID, now time.Time, limit int) ([]domain.Session, error)
	Touch(ctx context.Context, id uuid.UUID, at time.Time) error
	// RevokeByID returns domain.ErrSessionNotFound for a session that is
	// not active.
	RevokeByID(ctx context.Context, id uuid.UUID) error
	RevokeAllForUserExcept(ctx context.Context, userID, keep uuid.UUID) (int, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encodedHash string) (bool, error)
}
