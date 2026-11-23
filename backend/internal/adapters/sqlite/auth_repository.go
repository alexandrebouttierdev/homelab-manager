package sqlite

import (
	"context"
	"time"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/ent/session"
	"homelab-manager/backend/ent/user"
	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

type AuthRepository struct {
	client *ent.Client
}

func NewAuthRepository(client *ent.Client) *AuthRepository {
	return &AuthRepository{client: client}
}

func (r *AuthRepository) FindByUsername(ctx context.Context, username string) (domain.User, string, error) {
	record, err := r.client.User.Query().Where(user.Username(username)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.User{}, "", domain.ErrInvalidCredentials
		}
		return domain.User{}, "", err
	}
	return toDomainUser(record), record.PasswordHash, nil
}

func (r *AuthRepository) FindByID(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	record, err := r.client.User.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(record), nil
}

// UpdatePasswordHash stores a password chosen by the user, which also ends
// the obligation to change the initial password.
func (r *AuthRepository) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	return r.client.User.UpdateOneID(userID).SetPasswordHash(passwordHash).SetMustChangePassword(false).Exec(ctx)
}

// RecordLogin stores the time and source address of a successful login.
func (r *AuthRepository) RecordLogin(ctx context.Context, userID uuid.UUID, ip string, at time.Time) error {
	return r.client.User.UpdateOneID(userID).SetLastLoginAt(at.UTC()).SetLastLoginIP(ip).Exec(ctx)
}

func (r *AuthRepository) EnsureAdmin(ctx context.Context, username, passwordHash string) (bool, error) {
	exists, err := r.client.User.Query().Where(user.Username(username)).Exist(ctx)
	if err != nil || exists {
		return false, err
	}
	if _, err := r.client.User.Create().SetUsername(username).SetDisplayName(username).SetRole(user.RoleAdmin).SetPasswordHash(passwordHash).Save(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *AuthRepository) Create(ctx context.Context, userID uuid.UUID, tokenHash, ip, userAgent string, expiresAt time.Time) (domain.Session, error) {
	record, err := r.client.Session.Create().
		SetUserID(userID).
		SetTokenHash(tokenHash).
		SetExpiresAt(expiresAt).
		SetIPAddress(ip).
		SetUserAgent(userAgent).
		Save(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	return domain.Session{ID: record.ID, UserID: userID, ExpiresAt: record.ExpiresAt}, nil
}

func (r *AuthRepository) FindActiveByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error) {
	record, err := r.client.Session.Query().
		Where(
			session.TokenHash(tokenHash),
			session.RevokedAtIsNil(),
			session.ExpiresAtGT(time.Now().UTC()),
		).
		WithUser().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.Session{}, domain.ErrSessionNotFound
		}
		return domain.Session{}, err
	}
	return domain.Session{
		ID: record.ID, UserID: record.Edges.User.ID, ExpiresAt: record.ExpiresAt,
	}, nil
}

func (r *AuthRepository) RevokeByTokenHash(ctx context.Context, tokenHash string) error {
	count, err := r.client.Session.Update().
		Where(session.TokenHash(tokenHash), session.RevokedAtIsNil()).
		SetRevokedAt(time.Now().UTC()).Save(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrSessionNotFound
	}
	return nil
}

func (r *AuthRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.client.Session.Update().
		Where(session.HasUserWith(user.ID(userID)), session.RevokedAtIsNil()).
		SetRevokedAt(time.Now().UTC()).Save(ctx)
	return err
}
