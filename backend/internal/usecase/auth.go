package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

type AuthUsecase struct {
	users    ports.UserRepository
	sessions ports.SessionRepository
	hasher   ports.PasswordHasher
	now      func() time.Time

	// dummyHash is verified when the username is unknown, so that a missing
	// account costs the same Argon2id work as a wrong password.
	dummyOnce sync.Once
	dummyHash string
}

func NewAuthUsecase(users ports.UserRepository, sessions ports.SessionRepository, hasher ports.PasswordHasher) *AuthUsecase {
	return &AuthUsecase{users: users, sessions: sessions, hasher: hasher, now: time.Now}
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
	valid, err := u.hasher.Verify(password, storedHash)
	if err != nil || !valid {
		return "", domain.User{}, time.Time{}, domain.ErrInvalidCredentials
	}

	token, err := randomToken()
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	expiresAt := u.now().UTC().Add(12 * time.Hour)
	tokenHash := sha256Hex(token)
	session, err := u.sessions.Create(ctx, user.ID, tokenHash, ip, userAgent, expiresAt)
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	return token, user, session.ExpiresAt, nil
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
	if err != nil {
		return domain.User{}, domain.Session{}, err
	}
	return user, session, nil
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
