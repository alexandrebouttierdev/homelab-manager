package ports

import (
	"context"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// APITokenRepository stores the API tokens as their SHA-256
// (SPEC_ADMINISTRATION.md § 3.5).
type APITokenRepository interface {
	// CreateAPIToken stores the token and its audit entry in one
	// transaction, or domain.ErrTooManyAPITokens past
	// domain.MaxAPITokensPerUser for the account.
	CreateAPIToken(ctx context.Context, token domain.APIToken, hash string, audit domain.AuditEntry) error
	// APITokens lists the tokens of an account, or of every account when
	// userID is nil, newest first, with the account name.
	APITokens(ctx context.Context, userID *uuid.UUID) ([]domain.APIToken, error)
	// APIToken returns one token or domain.ErrAPITokenNotFound.
	APIToken(ctx context.Context, id uuid.UUID) (domain.APIToken, error)
	// APITokenByHash returns the token of a hash or domain.ErrAPITokenNotFound.
	APITokenByHash(ctx context.Context, hash string) (domain.APIToken, error)
	DeleteAPIToken(ctx context.Context, id uuid.UUID, audit domain.AuditEntry) error
	TouchAPIToken(ctx context.Context, id uuid.UUID, at time.Time) error
}
