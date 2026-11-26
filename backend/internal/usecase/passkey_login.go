package usecase

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

// maxPasskeyCeremonies bounds the passkey logins started and not finished.
const maxPasskeyCeremonies = 1000

// passkeyCeremonies keeps the passkey logins in progress in memory, keyed
// by the hash of their token. Each one answers once and lasts 5 minutes.
type passkeyCeremonies struct {
	mu      sync.Mutex
	pending map[string]passkeyCeremony
}

type passkeyCeremony struct {
	session   []byte
	expiresAt time.Time
}

func newPasskeyCeremonies() *passkeyCeremonies {
	return &passkeyCeremonies{pending: make(map[string]passkeyCeremony)}
}

func (c *passkeyCeremonies) create(session []byte, now time.Time) (string, time.Time, error) {
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, ceremony := range c.pending {
		if !now.Before(ceremony.expiresAt) {
			delete(c.pending, key)
		}
	}
	if len(c.pending) >= maxPasskeyCeremonies {
		return "", time.Time{}, ErrTooManyLoginChallenges
	}
	expiresAt := now.Add(domain.WebAuthnCeremonyTTL)
	c.pending[sha256Hex(token)] = passkeyCeremony{session: session, expiresAt: expiresAt}
	return token, expiresAt, nil
}

// take removes the ceremony of a token and returns its session if it has
// not expired.
func (c *passkeyCeremonies) take(token string, now time.Time) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := sha256Hex(token)
	ceremony, ok := c.pending[key]
	delete(c.pending, key)
	if !ok || !now.Before(ceremony.expiresAt) {
		return nil, false
	}
	return ceremony.session, true
}

// PasskeyLoginAvailable reports whether the login page may offer the
// login with a passkey alone (decision D-14).
func (u *AuthUsecase) PasskeyLoginAvailable() bool {
	return u.verifier != nil && u.keys != nil && u.policy != nil && u.policy.Policy().AllowPasskeyLogin
}

func (u *AuthUsecase) passkeyLoginAllowed() error {
	if u.verifier == nil || u.keys == nil {
		return domain.ErrWebAuthnUnavailable
	}
	if u.policy == nil || !u.policy.Policy().AllowPasskeyLogin {
		return domain.ErrPasskeyLoginDisabled
	}
	return nil
}

// BeginPasskeyLogin starts a login without user name nor password: the
// browser offers the passkeys it holds for the site. token identifies the
// ceremony for CompletePasskeyLogin.
func (u *AuthUsecase) BeginPasskeyLogin() (token string, options []byte, expiresAt time.Time, err error) {
	if err := u.passkeyLoginAllowed(); err != nil {
		return "", nil, time.Time{}, err
	}
	options, session, err := u.verifier.BeginPasskeyLogin()
	if err != nil {
		return "", nil, time.Time{}, err
	}
	token, expiresAt, err = u.passkeys.create(session, u.now().UTC())
	if err != nil {
		return "", nil, time.Time{}, err
	}
	return token, options, expiresAt, nil
}

// CompletePasskeyLogin checks the answer of the browser and opens a
// session for the account of the passkey. The passkey itself proves two
// factors: the device and its PIN or biometrics (user verification). A
// disabled or locked account, or one that must change its password, is
// refused.
func (u *AuthUsecase) CompletePasskeyLogin(ctx context.Context, token string, response []byte, ip, userAgent string) (LoginResult, error) {
	now := u.now().UTC()
	// A ceremony answers once, right or wrong.
	session, ok := u.passkeys.take(token, now)
	if !ok {
		return LoginResult{}, domain.ErrLoginChallengeExpired
	}
	if err := u.passkeyLoginAllowed(); err != nil {
		return LoginResult{}, err
	}
	if len(response) == 0 || len(response) > domain.MaxWebAuthnResponseBytes {
		return LoginResult{}, domain.ErrInvalidPasskey
	}
	var keys []domain.WebAuthnCredential
	lookup := func(handle []byte) (domain.User, []domain.WebAuthnCredential, error) {
		id, err := uuid.FromBytes(handle)
		if err != nil {
			return domain.User{}, nil, domain.ErrWebAuthnKeyNotFound
		}
		user, err := u.users.FindByID(ctx, id)
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, nil, domain.ErrWebAuthnKeyNotFound
		}
		if err != nil {
			return domain.User{}, nil, err
		}
		keys, err = u.keys.WebAuthnCredentials(ctx, id)
		if err != nil {
			return domain.User{}, nil, err
		}
		if len(keys) == 0 {
			return domain.User{}, nil, domain.ErrWebAuthnKeyNotFound
		}
		return user, keys, nil
	}
	user, credentialID, data, err := u.verifier.FinishPasskeyLogin(session, response, lookup)
	if errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
		return LoginResult{}, domain.ErrInvalidPasskey
	}
	if err != nil {
		return LoginResult{}, err
	}
	failures, err := u.loginFailures(ctx, user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if user.Disabled || failures.Locked(now) {
		return LoginResult{}, domain.ErrInvalidPasskey
	}
	if user.MustChangePassword {
		return LoginResult{}, domain.ErrPasskeyPasswordChange
	}
	used := false
	for _, key := range keys {
		if bytes.Equal(key.CredentialID, credentialID) {
			if err := u.keys.UseWebAuthnCredential(ctx, key.ID, data, now); err != nil {
				return LoginResult{}, err
			}
			used = true
			break
		}
	}
	if !used {
		return LoginResult{}, domain.ErrInvalidPasskey
	}
	return u.openSession(ctx, user, failures, ip, userAgent, "passkey", now)
}
