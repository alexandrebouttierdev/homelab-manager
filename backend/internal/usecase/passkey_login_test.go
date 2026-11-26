package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

// newPasskeyAuth builds an auth usecase whose policy allows (or not) the
// passkey login, with one key "cred-<name>" per account of users.
func newPasskeyAuth(users usersByName, allow bool) (*AuthUsecase, *memorySessions, *memoryFailures, *memoryFactors, *time.Time) {
	sessions := newMemorySessions()
	factors := newMemoryFactors()
	for name, user := range users {
		factors.keys[user.ID] = []domain.WebAuthnCredential{{ID: uuid.New(), UserID: user.ID, CredentialID: []byte("cred-" + name), Data: []byte("data")}}
	}
	failures := &memoryFailures{failures: map[uuid.UUID]domain.LoginFailures{}}
	policy := domain.DefaultSecurityPolicy()
	policy.AllowPasskeyLogin = allow
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	auth := NewAuthUsecase(users, sessions, &checkingHasher{}).
		WithSecurity(failures, &fakeAudit{}, fixedPolicy{policy}).
		WithSecondFactor(factors, fakeBox{}).
		WithWebAuthn(&fakeVerifier{}, factors)
	auth.now = func() time.Time { return now }
	return auth, sessions, failures, factors, &now
}

// passkeyAnswer begins a passkey login and returns its token and the
// answer of the key of name for the account id.
func passkeyAnswer(t *testing.T, auth *AuthUsecase, id uuid.UUID, name string) (string, []byte) {
	t.Helper()
	token, options, _, err := auth.BeginPasskeyLogin()
	if err != nil {
		t.Fatal(err)
	}
	return token, []byte("ok:" + sessionOf(t, options) + ":" + id.String() + ":cred-" + name)
}

func TestPasskeyLoginOpensASession(t *testing.T) {
	auth, sessions, failures, factors, now := newPasskeyAuth(usersByName{"alice": securityAlice}, true)
	ctx := context.Background()
	if !auth.PasskeyLoginAvailable() {
		t.Fatal("the passkey login is not offered")
	}
	failures.failures[securityAlice.ID] = domain.LoginFailures{Count: 2}

	token, answer := passkeyAnswer(t, auth, securityAlice.ID, "alice")
	result, err := auth.CompletePasskeyLogin(ctx, token, answer, "192.0.2.1", "test")
	if err != nil || result.Token == "" || result.User.ID != securityAlice.ID {
		t.Fatalf("passkey login = (%+v, %v)", result, err)
	}
	stored, _ := sessions.FindActiveByTokenHash(ctx, sha256Hex(result.Token))
	if stored.AuthMethod != "passkey" {
		t.Fatalf("auth method = %q", stored.AuthMethod)
	}
	if key := factors.keys[securityAlice.ID][0]; string(key.Data) != "data-passkey" || key.LastUsedAt == nil || !key.LastUsedAt.Equal(*now) {
		t.Fatalf("key after login = %+v", key)
	}
	if failures.failures[securityAlice.ID].Count != 0 {
		t.Fatal("a passkey login clears the failures")
	}
	// A ceremony answers once.
	if _, err := auth.CompletePasskeyLogin(ctx, token, answer, "", ""); !errors.Is(err, domain.ErrLoginChallengeExpired) {
		t.Fatalf("second answer = %v", err)
	}
}

func TestPasskeyLoginRefusals(t *testing.T) {
	disabled, mustChange := securityBob, securityAdmin
	disabled.Disabled = true
	mustChange.MustChangePassword = true
	users := usersByName{"alice": securityAlice, "bob": disabled, "admin": mustChange}
	auth, _, failures, _, now := newPasskeyAuth(users, true)
	ctx := context.Background()

	cases := []struct {
		name   string
		answer func() (string, []byte)
		want   error
	}{
		{"key of another account", func() (string, []byte) { return passkeyAnswer(t, auth, securityAlice.ID, "bob") }, domain.ErrInvalidPasskey},
		{"unknown account", func() (string, []byte) { return passkeyAnswer(t, auth, uuid.New(), "alice") }, domain.ErrInvalidPasskey},
		{"wrong answer", func() (string, []byte) {
			token, _ := passkeyAnswer(t, auth, securityAlice.ID, "alice")
			return token, []byte("no")
		}, domain.ErrInvalidPasskey},
		{"disabled account", func() (string, []byte) { return passkeyAnswer(t, auth, disabled.ID, "bob") }, domain.ErrInvalidPasskey},
		{"password to change", func() (string, []byte) { return passkeyAnswer(t, auth, mustChange.ID, "admin") }, domain.ErrPasskeyPasswordChange},
		{"unknown ceremony", func() (string, []byte) {
			_, answer := passkeyAnswer(t, auth, securityAlice.ID, "alice")
			return "other", answer
		}, domain.ErrLoginChallengeExpired},
	}
	for _, test := range cases {
		token, answer := test.answer()
		if _, err := auth.CompletePasskeyLogin(ctx, token, answer, "", ""); !errors.Is(err, test.want) {
			t.Fatalf("%s = %v, want %v", test.name, err, test.want)
		}
	}
	if failures.failures[securityAlice.ID].Count != 0 {
		t.Fatal("a refused passkey does not count as a password failure")
	}

	locked := now.Add(time.Minute)
	failures.failures[securityAlice.ID] = domain.LoginFailures{Count: domain.MaxConsecutiveFailures, LockedUntil: &locked}
	token, answer := passkeyAnswer(t, auth, securityAlice.ID, "alice")
	if _, err := auth.CompletePasskeyLogin(ctx, token, answer, "", ""); !errors.Is(err, domain.ErrInvalidPasskey) {
		t.Fatalf("locked account = %v", err)
	}

	// The ceremony lasts 5 minutes.
	failures.failures[securityAlice.ID] = domain.LoginFailures{}
	token, answer = passkeyAnswer(t, auth, securityAlice.ID, "alice")
	*now = now.Add(domain.WebAuthnCeremonyTTL)
	if _, err := auth.CompletePasskeyLogin(ctx, token, answer, "", ""); !errors.Is(err, domain.ErrLoginChallengeExpired) {
		t.Fatalf("expired ceremony = %v", err)
	}
}

func TestPasskeyLoginFollowsThePolicy(t *testing.T) {
	auth, _, _, _, _ := newPasskeyAuth(usersByName{"alice": securityAlice}, false)
	if auth.PasskeyLoginAvailable() {
		t.Fatal("the passkey login is offered against the policy")
	}
	if _, _, _, err := auth.BeginPasskeyLogin(); !errors.Is(err, domain.ErrPasskeyLoginDisabled) {
		t.Fatalf("begin while disabled = %v", err)
	}
	withoutWebAuthn, _, _, _, _ := newSecureAuth(newMemorySessions(), domain.SecurityPolicy{IdleTimeoutMinutes: 60, AllowPasskeyLogin: true})
	if withoutWebAuthn.PasskeyLoginAvailable() {
		t.Fatal("the passkey login is offered without WebAuthn")
	}
	if _, _, _, err := withoutWebAuthn.BeginPasskeyLogin(); !errors.Is(err, domain.ErrWebAuthnUnavailable) {
		t.Fatalf("begin without WebAuthn = %v", err)
	}
}
