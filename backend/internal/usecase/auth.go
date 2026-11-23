package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
)

type AuthUsecase struct {
	users    ports.UserRepository
	sessions ports.SessionRepository
	hasher   ports.PasswordHasher
	now      func() time.Time

	// Set by WithSecurity; without them, lockout, idle expiry and audit of
	// session revocations are off (unit tests of the basic flow).
	failures ports.LoginFailureRepository
	audit    ports.AuditRecorder
	policy   securityPolicySource

	// dummyHash is verified when the username is unknown, so that a missing
	// account costs the same Argon2id work as a wrong password.
	dummyOnce sync.Once
	dummyHash string
}

func NewAuthUsecase(users ports.UserRepository, sessions ports.SessionRepository, hasher ports.PasswordHasher) *AuthUsecase {
	return &AuthUsecase{users: users, sessions: sessions, hasher: hasher, now: time.Now}
}

// securityPolicySource gives the current policy; SecurityUsecase keeps it
// in memory.
type securityPolicySource interface {
	Policy() domain.SecurityPolicy
}

// maxSessionsListed bounds a session listing.
const maxSessionsListed = 500

// WithSecurity enables the account lockout, the idle expiry of sessions
// and the audit of session revocations (SPEC_ADMINISTRATION.md §§ 3.4-3.5).
func (u *AuthUsecase) WithSecurity(failures ports.LoginFailureRepository, audit ports.AuditRecorder, policy securityPolicySource) *AuthUsecase {
	u.failures, u.audit, u.policy = failures, audit, policy
	return u
}

func (u *AuthUsecase) Login(ctx context.Context, username, password, ip, userAgent string) (string, domain.User, time.Time, error) {
	user, storedHash, err := u.users.FindByUsername(ctx, username)
	if errors.Is(err, domain.ErrInvalidCredentials) {
		u.verifyDummy(password)
		return "", domain.User{}, time.Time{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	now := u.now().UTC()
	var failures domain.LoginFailures
	if u.failures != nil {
		if failures, err = u.failures.LoginFailures(ctx, user.ID); err != nil {
			return "", domain.User{}, time.Time{}, fmt.Errorf("read login failures: %w", err)
		}
	}
	// The password is always verified, so that a disabled or locked account
	// costs the same Argon2id work as any other refusal and answers alike.
	valid, err := u.hasher.Verify(password, storedHash)
	if failures.Locked(now) || user.Disabled {
		return "", domain.User{}, time.Time{}, domain.ErrInvalidCredentials
	}
	if err != nil || !valid {
		if err := u.countFailure(ctx, user, failures, now); err != nil {
			return "", domain.User{}, time.Time{}, err
		}
		return "", domain.User{}, time.Time{}, domain.ErrInvalidCredentials
	}
	if u.failures != nil && (failures.Count > 0 || failures.LockedUntil != nil) {
		if err := u.failures.ClearLoginFailures(ctx, user.ID); err != nil {
			return "", domain.User{}, time.Time{}, fmt.Errorf("clear login failures: %w", err)
		}
	}

	token, err := randomToken()
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	if err := u.users.RecordLogin(ctx, user.ID, ip, now); err != nil {
		return "", domain.User{}, time.Time{}, fmt.Errorf("record login: %w", err)
	}
	expiresAt := now.Add(12 * time.Hour)
	tokenHash := sha256Hex(token)
	session, err := u.sessions.Create(ctx, user.ID, tokenHash, ip, userAgent, expiresAt)
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	return token, user, session.ExpiresAt, nil
}

// EnsureAdministrator creates the initial administrator when absent. With
// the built-in default password (isDefault), the account must change it at
// its next login, including an existing account that still accepts it
// (SPEC_ADMINISTRATION.md § 3.1, decision Q-2). forced reports that change.
func (u *AuthUsecase) EnsureAdministrator(ctx context.Context, username, password string, isDefault bool) (created, forced bool, err error) {
	hash, err := u.hasher.Hash(password)
	if err != nil {
		return false, false, fmt.Errorf("hash administrator password: %w", err)
	}
	created, err = u.users.EnsureAdmin(ctx, username, hash)
	if err != nil || !isDefault {
		return created, false, err
	}
	user, storedHash, err := u.users.FindByUsername(ctx, username)
	if err != nil {
		return created, false, err
	}
	if user.MustChangePassword {
		return created, false, nil
	}
	if !created {
		// One Argon2id verification at startup: is the default still valid?
		valid, err := u.hasher.Verify(password, storedHash)
		if err != nil {
			return false, false, fmt.Errorf("verify administrator password: %w", err)
		}
		if !valid {
			return false, false, nil
		}
	}
	if err := u.users.RequirePasswordChange(ctx, user.ID); err != nil {
		return created, false, fmt.Errorf("require password change: %w", err)
	}
	return created, true, nil
}

func (u *AuthUsecase) Logout(ctx context.Context, token string) error {
	if token == "" {
		return domain.ErrSessionNotFound
	}
	return u.sessions.RevokeByTokenHash(ctx, sha256Hex(token))
}

func (u *AuthUsecase) Session(ctx context.Context, token string) (domain.User, domain.Session, error) {
	if token == "" {
		return domain.User{}, domain.Session{}, domain.ErrSessionNotFound
	}
	session, err := u.sessions.FindActiveByTokenHash(ctx, sha256Hex(token))
	if err != nil {
		return domain.User{}, domain.Session{}, err
	}
	if !session.ExpiresAt.After(u.now()) {
		return domain.User{}, domain.Session{}, domain.ErrSessionNotFound
	}

	user, err := u.users.FindByID(ctx, session.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return domain.User{}, domain.Session{}, domain.ErrSessionNotFound
	}
	if err != nil {
		return domain.User{}, domain.Session{}, err
	}
	// Disabling an account revokes its sessions; this check also covers a
	// revocation that failed after the account was disabled.
	if user.Disabled {
		return domain.User{}, domain.Session{}, domain.ErrSessionNotFound
	}
	if u.policy != nil {
		now := u.now().UTC()
		if session.IdleExpired(now, u.policy.Policy()) {
			if err := u.sessions.RevokeByID(ctx, session.ID); err != nil && !errors.Is(err, domain.ErrSessionNotFound) {
				return domain.User{}, domain.Session{}, fmt.Errorf("revoke idle session: %w", err)
			}
			return domain.User{}, domain.Session{}, domain.ErrSessionNotFound
		}
		if session.NeedsTouch(now) {
			if err := u.sessions.Touch(ctx, session.ID, now); err != nil {
				return domain.User{}, domain.Session{}, fmt.Errorf("touch session: %w", err)
			}
			session.LastSeenAt = now
		}
	}
	return user, session, nil
}

// ListSessions returns the active sessions of the actor, or of every
// account when all is set (administrators only).
func (u *AuthUsecase) ListSessions(ctx context.Context, actor domain.User, all bool) ([]domain.Session, error) {
	now := u.now().UTC()
	if all {
		if !actor.Role.Can(domain.PermManageUsers) {
			return nil, domain.ErrForbidden
		}
		return u.sessions.ListActive(ctx, nil, now, maxSessionsListed)
	}
	return u.sessions.ListActive(ctx, &actor.ID, now, maxSessionsListed)
}

// RevokeSession ends one session. Anyone may end their own sessions; an
// administrator may end any. Another account's session is reported as not
// found, so that its identifier reveals nothing.
func (u *AuthUsecase) RevokeSession(ctx context.Context, actor domain.User, id uuid.UUID, requestID string) error {
	target, err := u.sessions.FindActiveByID(ctx, id)
	if err != nil {
		return err
	}
	if target.UserID != actor.ID && !actor.Role.Can(domain.PermManageUsers) {
		return domain.ErrSessionNotFound
	}
	if err := u.sessions.RevokeByID(ctx, id); err != nil {
		return err
	}
	return u.record(ctx, domain.AuditEntry{
		Actor: actor.Username, TargetType: "session", TargetID: id.String(),
		Action: "revoke", RequestID: requestID, Outcome: domain.AuditSuccess,
	})
}

// RevokeOtherSessions ends every session of the actor but the current one
// and reports how many it ended.
func (u *AuthUsecase) RevokeOtherSessions(ctx context.Context, actor domain.User, current uuid.UUID, requestID string) (int, error) {
	count, err := u.sessions.RevokeAllForUserExcept(ctx, actor.ID, current)
	if err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}
	return count, u.record(ctx, domain.AuditEntry{
		Actor: actor.Username, TargetType: "user", TargetID: actor.ID.String(),
		Action: "revoke_other_sessions", RequestID: requestID, Outcome: domain.AuditSuccess,
	})
}

// countFailure records a failed password and locks the account at the limit.
func (u *AuthUsecase) countFailure(ctx context.Context, user domain.User, failures domain.LoginFailures, now time.Time) error {
	if u.failures == nil {
		return nil
	}
	next, locked := failures.AfterFailure(now)
	if err := u.failures.SaveLoginFailures(ctx, user.ID, next, now); err != nil {
		return fmt.Errorf("save login failures: %w", err)
	}
	if !locked {
		return nil
	}
	return u.record(ctx, domain.AuditEntry{
		Actor: "system", TargetType: "user", TargetID: user.ID.String(),
		Action: "lock", Outcome: domain.AuditSuccess,
	})
}

func (u *AuthUsecase) record(ctx context.Context, entry domain.AuditEntry) error {
	if u.audit == nil {
		return nil
	}
	if err := u.audit.RecordAudit(ctx, entry); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	return nil
}

func (u *AuthUsecase) ChangePassword(ctx context.Context, token, currentPassword, newPassword string) error {
	user, _, err := u.Session(ctx, token)
	if err != nil {
		return err
	}
	_, storedHash, err := u.users.FindByUsername(ctx, user.Username)
	if err != nil {
		return err
	}
	valid, err := u.hasher.Verify(currentPassword, storedHash)
	if err != nil || !valid {
		return domain.ErrInvalidCredentials
	}
	hash, err := u.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	if err := u.users.UpdatePasswordHash(ctx, user.ID, hash); err != nil {
		return err
	}
	return u.sessions.RevokeAllForUser(ctx, user.ID)
}

func (u *AuthUsecase) verifyDummy(password string) {
	u.dummyOnce.Do(func() {
		hash, err := u.hasher.Hash("homelab-manager-unknown-user")
		if err == nil {
			u.dummyHash = hash
		}
	})
	if u.dummyHash != "" {
		_, _ = u.hasher.Verify(password, u.dummyHash)
	}
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func IsInvalidCredentials(err error) bool {
	return errors.Is(err, domain.ErrInvalidCredentials)
}
