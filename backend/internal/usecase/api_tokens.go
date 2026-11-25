package usecase

import (
	"context"
	"errors"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const auditTargetAPIToken = "api_token"

// APITokenUsecase manages the read-only API tokens
// (SPEC_ADMINISTRATION.md § 3.5). Every account manages its own tokens; an
// administrator (domain.PermManageUsers) lists and revokes those of all.
type APITokenUsecase struct {
	tokens ports.APITokenRepository
	users  ports.UserRepository
	logger *zap.Logger
	now    func() time.Time
}

func NewAPITokenUsecase(tokens ports.APITokenRepository, users ports.UserRepository, logger *zap.Logger) *APITokenUsecase {
	return &APITokenUsecase{tokens: tokens, users: users, logger: logger, now: time.Now}
}

// Create stores a token of the actor and returns it with its secret, shown
// this once.
func (u *APITokenUsecase) Create(ctx context.Context, actor domain.User, request domain.APITokenRequest, requestID string) (string, domain.APIToken, error) {
	request, err := request.Validate()
	if err != nil {
		return "", domain.APIToken{}, err
	}
	for _, scope := range request.Scopes {
		if permission, action := scope.Permission(); action && !actor.Role.Can(permission) {
			return "", domain.APIToken{}, domain.ErrAPITokenScopeNotAllowed
		}
	}
	secret, hash, err := domain.NewAPITokenSecret()
	if err != nil {
		return "", domain.APIToken{}, err
	}
	now := u.now().UTC()
	token := domain.APIToken{
		ID: uuid.New(), UserID: actor.ID, Username: actor.Username, Name: request.Name,
		Prefix: secret[:domain.APITokenDisplayLength], Scopes: request.Scopes,
		CreatedAt: now, ExpiresAt: now.AddDate(0, 0, request.LifetimeDays),
	}
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetAPIToken, TargetID: token.ID.String(), Action: "create",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	if err := u.tokens.CreateAPIToken(ctx, token, hash, audit); err != nil {
		return "", domain.APIToken{}, err
	}
	return secret, token, nil
}

// List returns the tokens of the actor, or of every account when all is
// set (the caller checks domain.PermManageUsers).
func (u *APITokenUsecase) List(ctx context.Context, actor domain.User, all bool) ([]domain.APIToken, error) {
	if all {
		return u.tokens.APITokens(ctx, nil)
	}
	return u.tokens.APITokens(ctx, &actor.ID)
}

// Revoke deletes a token of the actor, or any token for an administrator.
// A token of another account is reported as not found to the others.
func (u *APITokenUsecase) Revoke(ctx context.Context, actor domain.User, id uuid.UUID, requestID string) error {
	token, err := u.tokens.APIToken(ctx, id)
	if err != nil {
		return err
	}
	if token.UserID != actor.ID && !actor.Role.Can(domain.PermManageUsers) {
		return domain.ErrAPITokenNotFound
	}
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetAPIToken, TargetID: id.String(), Action: "revoke",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	return u.tokens.DeleteAPIToken(ctx, id, audit)
}

// Authenticate returns the account and the token of a bearer token, or
// domain.ErrAPITokenUnauthorized: unknown, expired, or account disabled.
func (u *APITokenUsecase) Authenticate(ctx context.Context, secret string) (domain.User, domain.APIToken, error) {
	if !domain.LooksLikeAPIToken(secret) {
		return domain.User{}, domain.APIToken{}, domain.ErrAPITokenUnauthorized
	}
	token, err := u.tokens.APITokenByHash(ctx, domain.HashAPIToken(secret))
	if errors.Is(err, domain.ErrAPITokenNotFound) {
		return domain.User{}, domain.APIToken{}, domain.ErrAPITokenUnauthorized
	}
	if err != nil {
		return domain.User{}, domain.APIToken{}, err
	}
	now := u.now().UTC()
	if !now.Before(token.ExpiresAt) {
		return domain.User{}, domain.APIToken{}, domain.ErrAPITokenUnauthorized
	}
	user, err := u.users.FindByID(ctx, token.UserID)
	if err != nil || user.Disabled {
		return domain.User{}, domain.APIToken{}, domain.ErrAPITokenUnauthorized
	}
	if token.LastUsedAt == nil || now.Sub(*token.LastUsedAt) >= domain.APITokenTouchEvery {
		if err := u.tokens.TouchAPIToken(ctx, token.ID, now); err != nil {
			u.logger.Warn("could not record the use of an API token", zap.Error(err))
		}
	}
	token.Username = user.Username
	return user, token, nil
}
