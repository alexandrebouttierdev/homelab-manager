package sqlite

import (
	"context"
	"fmt"
	"time"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/ent/predicate"
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

// RequirePasswordChange makes the account choose a new password at its
// next login.
func (r *AuthRepository) RequirePasswordChange(ctx context.Context, userID uuid.UUID) error {
	return r.client.User.UpdateOneID(userID).SetMustChangePassword(true).Exec(ctx)
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
		SetLastSeenAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	owner, err := r.client.User.Get(ctx, userID)
	if err != nil {
		return domain.Session{}, err
	}
	record.Edges.User = owner
	return toDomainSession(record), nil
}

func (r *AuthRepository) FindActiveByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error) {
	return r.findActive(ctx, session.TokenHash(tokenHash))
}

// FindActiveByID returns domain.ErrSessionNotFound for a revoked or expired
// session.
func (r *AuthRepository) FindActiveByID(ctx context.Context, id uuid.UUID) (domain.Session, error) {
	return r.findActive(ctx, session.ID(id))
}

func (r *AuthRepository) findActive(ctx context.Context, match predicate.Session) (domain.Session, error) {
	record, err := r.client.Session.Query().
		Where(match, session.RevokedAtIsNil(), session.ExpiresAtGT(time.Now().UTC())).
		WithUser().
		Only(ctx)
	if ent.IsNotFound(err) {
		return domain.Session{}, domain.ErrSessionNotFound
	}
	if err != nil {
		return domain.Session{}, err
	}
	return toDomainSession(record), nil
}

// ListActive returns at most limit active sessions, most recently seen
// first; a nil userID lists the sessions of every account.
func (r *AuthRepository) ListActive(ctx context.Context, userID *uuid.UUID, now time.Time, limit int) ([]domain.Session, error) {
	query := r.client.Session.Query().
		Where(session.RevokedAtIsNil(), session.ExpiresAtGT(now.UTC())).
		WithUser().
		Order(ent.Desc(session.FieldLastSeenAt), ent.Desc(session.FieldID)).
		Limit(limit)
	if userID != nil {
		query.Where(session.UserID(*userID))
	}
	records, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	sessions := make([]domain.Session, 0, len(records))
	for _, record := range records {
		sessions = append(sessions, toDomainSession(record))
	}
	return sessions, nil
}

// Touch records the last activity of a session.
func (r *AuthRepository) Touch(ctx context.Context, id uuid.UUID, at time.Time) error {
	return r.client.Session.UpdateOneID(id).SetLastSeenAt(at.UTC()).Exec(ctx)
}

// RevokeByID returns domain.ErrSessionNotFound when the session is not
// active.
func (r *AuthRepository) RevokeByID(ctx context.Context, id uuid.UUID) error {
	count, err := r.client.Session.Update().
		Where(session.ID(id), session.RevokedAtIsNil()).
		SetRevokedAt(time.Now().UTC()).Save(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrSessionNotFound
	}
	return nil
}

// RevokeAllForUserExcept revokes the other sessions of an account and
// reports how many it revoked.
func (r *AuthRepository) RevokeAllForUserExcept(ctx context.Context, userID, keep uuid.UUID) (int, error) {
	return r.client.Session.Update().
		Where(session.UserID(userID), session.IDNEQ(keep), session.RevokedAtIsNil()).
		SetRevokedAt(time.Now().UTC()).Save(ctx)
}

func toDomainSession(record *ent.Session) domain.Session {
	result := domain.Session{
		ID: record.ID, UserID: record.UserID, IPAddress: record.IPAddress,
		UserAgent: record.UserAgent, AuthMethod: record.AuthMethod,
		CreatedAt: record.CreatedAt, LastSeenAt: record.LastSeenAt, ExpiresAt: record.ExpiresAt,
	}
	if record.Edges.User != nil {
		result.Username = record.Edges.User.Username
	}
	if result.LastSeenAt.IsZero() {
		result.LastSeenAt = record.CreatedAt
	}
	return result
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
