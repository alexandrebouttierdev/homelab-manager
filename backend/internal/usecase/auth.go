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

	// Set by WithSecondFactor; box is nil without the secrets key, which
	// leaves only recovery codes for the second step.
	secondFactors ports.SecondFactorRepository
	box           ports.SecretBox
	challenges    *loginChallenges

	// dummyHash is verified when the username is unknown, so that a missing
	// account costs the same Argon2id work as a wrong password.
	dummyOnce sync.Once
	dummyHash string
}

func NewAuthUsecase(users ports.UserRepository, sessions ports.SessionRepository, hasher ports.PasswordHasher) *AuthUsecase {
	return &AuthUsecase{users: users, sessions: sessions, hasher: hasher, now: time.Now, challenges: newLoginChallenges()}
}

// sessionLifetime is the fixed maximum duration of a session.
const sessionLifetime = 12 * time.Hour

// LoginResult is either an open session (Token) or, for an account with a
// second factor, a pending login to complete (Challenge).
type LoginResult struct {
	Token     string
	Challenge string
	User      domain.User
	// ExpiresAt is the end of the session, or of the challenge.
	ExpiresAt time.Time
}

// WithSecondFactor asks accounts that have a second factor for it at login
// (SPEC_ADMINISTRATION.md § 3.3).
func (u *AuthUsecase) WithSecondFactor(factors ports.SecondFactorRepository, box ports.SecretBox) *AuthUsecase {
	u.secondFactors, u.box = factors, box
	return u
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

// Login checks the password. An account with a second factor gets a
// challenge to complete with CompleteSecondFactor; the others get a session.
func (u *AuthUsecase) Login(ctx context.Context, username, password, ip, userAgent string) (LoginResult, error) {
	user, storedHash, err := u.users.FindByUsername(ctx, username)
	if errors.Is(err, domain.ErrInvalidCredentials) {
		u.verifyDummy(password)
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	now := u.now().UTC()
	failures, err := u.loginFailures(ctx, user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	// The password is always verified, so that a disabled or locked account
	// costs the same Argon2id work as any other refusal and answers alike.
	valid, err := u.hasher.Verify(password, storedHash)
	if failures.Locked(now) || user.Disabled {
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	if err != nil || !valid {
		if err := u.countFailure(ctx, user, failures, now); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	if u.secondFactors != nil {
		enrolled, err := u.secondFactors.HasSecondFactor(ctx, user.ID)
		if err != nil {
			return LoginResult{}, err
		}
		if enrolled {
			// The failure counter is kept until the second step succeeds, so
			// that a known password does not reset it.
			token, expiresAt, err := u.challenges.create(user.ID, now)
			if err != nil {
				return LoginResult{}, err
			}
			return LoginResult{Challenge: token, User: user, ExpiresAt: expiresAt}, nil
		}
		user.MustEnrollSecondFactor = u.mustEnroll(user, false)
	}
	return u.openSession(ctx, user, failures, ip, userAgent, "password", now)
}

// CompleteSecondFactor ends a login started by Login with a TOTP code or a
// recovery code. Wrong codes count as login failures; the challenge ends
// after maxChallengeAttempts.
func (u *AuthUsecase) CompleteSecondFactor(ctx context.Context, challenge string, method domain.SecondFactorMethod, code, ip, userAgent string) (LoginResult, error) {
	now := u.now().UTC()
	pending, ok := u.challenges.find(challenge, now)
	if !ok || u.secondFactors == nil {
		return LoginResult{}, domain.ErrLoginChallengeExpired
	}
	user, err := u.users.FindByID(ctx, pending.userID)
	if errors.Is(err, domain.ErrUserNotFound) {
		u.challenges.remove(challenge)
		return LoginResult{}, domain.ErrLoginChallengeExpired
	}
	if err != nil {
		return LoginResult{}, err
	}
	failures, err := u.loginFailures(ctx, user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if user.Disabled || failures.Locked(now) {
		u.challenges.remove(challenge)
		return LoginResult{}, domain.ErrLoginChallengeExpired
	}
	valid, err := u.checkSecondFactor(ctx, user, method, code, now)
	if err != nil {
		return LoginResult{}, err
	}
	if !valid {
		u.challenges.fail(challenge)
		if err := u.countFailure(ctx, user, failures, now); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, domain.ErrInvalidSecondFactor
	}
	u.challenges.remove(challenge)
	return u.openSession(ctx, user, failures, ip, userAgent, "password+"+string(method), now)
}

func (u *AuthUsecase) checkSecondFactor(ctx context.Context, user domain.User, method domain.SecondFactorMethod, code string, now time.Time) (bool, error) {
	switch method {
	case domain.SecondFactorRecovery:
		return u.secondFactors.UseRecoveryCode(ctx, user.ID, domain.RecoveryCodeHash(code))
	case domain.SecondFactorTOTP:
		if u.box == nil {
			return false, domain.ErrSecondFactorUnavailable
		}
		enrollment, found, err := u.secondFactors.TOTP(ctx, user.ID)
		if err != nil || !found || !enrollment.Confirmed {
			return false, err
		}
		secret, err := u.box.Open(enrollment.Sealed, totpAssociatedData(user.ID))
		if err != nil {
			return false, fmt.Errorf("open TOTP secret: %w", err)
		}
		step, ok := domain.MatchTOTP(secret, code, now)
		if !ok {
			return false, nil
		}
		// Anti-replay: a step already used is refused.
		return u.secondFactors.UseTOTPStep(ctx, user.ID, step)
	default:
		return false, nil
	}
}

// openSession ends a successful login: failures cleared, login recorded,
// session created.
func (u *AuthUsecase) openSession(ctx context.Context, user domain.User, failures domain.LoginFailures, ip, userAgent, authMethod string, now time.Time) (LoginResult, error) {
	if u.failures != nil && (failures.Count > 0 || failures.LockedUntil != nil) {
		if err := u.failures.ClearLoginFailures(ctx, user.ID); err != nil {
			return LoginResult{}, fmt.Errorf("clear login failures: %w", err)
		}
	}
	token, err := randomToken()
	if err != nil {
		return LoginResult{}, err
	}
	if err := u.users.RecordLogin(ctx, user.ID, ip, now); err != nil {
		return LoginResult{}, fmt.Errorf("record login: %w", err)
	}
	session, err := u.sessions.Create(ctx, user.ID, sha256Hex(token), ip, userAgent, authMethod, now.Add(sessionLifetime))
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, User: user, ExpiresAt: session.ExpiresAt}, nil
}

func (u *AuthUsecase) loginFailures(ctx context.Context, userID uuid.UUID) (domain.LoginFailures, error) {
	if u.failures == nil {
		return domain.LoginFailures{}, nil
	}
	failures, err := u.failures.LoginFailures(ctx, userID)
	if err != nil {
		return domain.LoginFailures{}, fmt.Errorf("read login failures: %w", err)
	}
	return failures, nil
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
		policy := u.policy.Policy()
		if session.IdleExpired(now, policy) {
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
		if u.mustEnroll(user, false) {
			enrolled, err := u.secondFactors.HasSecondFactor(ctx, user.ID)
			if err != nil {
				return domain.User{}, domain.Session{}, fmt.Errorf("check second factor: %w", err)
			}
			user.MustEnrollSecondFactor = !enrolled
		}
	}
	return user, session, nil
}

// mustEnroll tells whether the policy restricts this account to the
// second factor enrollment, given whether it already has one.
func (u *AuthUsecase) mustEnroll(user domain.User, enrolled bool) bool {
	return !enrolled && user.Role == domain.RoleAdmin && u.secondFactors != nil &&
		u.policy != nil && u.policy.Policy().RequireAdminSecondFactor
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
