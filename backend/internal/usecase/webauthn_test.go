package usecase

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"

	"github.com/google/uuid"
)

func (m *memoryFactors) WebAuthnCredentials(_ context.Context, userID uuid.UUID) ([]domain.WebAuthnCredential, error) {
	return slices.Clone(m.keys[userID]), nil
}
func (m *memoryFactors) AddWebAuthnCredential(_ context.Context, key domain.WebAuthnCredential, hashes []string, audit domain.AuditEntry) error {
	if len(m.keys[key.UserID]) >= domain.MaxWebAuthnCredentials {
		return domain.ErrTooManyWebAuthnKeys
	}
	m.keys[key.UserID] = append(m.keys[key.UserID], key)
	if hashes != nil {
		m.setCodes(key.UserID, hashes)
	}
	m.audits = append(m.audits, audit)
	return nil
}
func (m *memoryFactors) UseWebAuthnCredential(_ context.Context, id uuid.UUID, data []byte, at time.Time) error {
	for userID, keys := range m.keys {
		for i := range keys {
			if keys[i].ID == id {
				m.keys[userID][i].Data, m.keys[userID][i].LastUsedAt = data, &at
				return nil
			}
		}
	}
	return domain.ErrWebAuthnKeyNotFound
}
func (m *memoryFactors) DeleteWebAuthnCredential(_ context.Context, userID, id uuid.UUID, audit domain.AuditEntry) error {
	keys := m.keys[userID]
	i := slices.IndexFunc(keys, func(key domain.WebAuthnCredential) bool { return key.ID == id })
	if i < 0 {
		return domain.ErrWebAuthnKeyNotFound
	}
	m.keys[userID] = slices.Delete(keys, i, i+1)
	if len(m.keys[userID]) == 0 && !m.totp[userID].Confirmed {
		delete(m.recovery, userID)
	}
	m.audits = append(m.audits, audit)
	return nil
}

// fakeVerifier accepts the answer "ok:<session>" to a registration, which
// creates the credential "cred-<session>", and "ok:<session>:<credential>"
// to a login.
type fakeVerifier struct{ ceremonies int }

func (f *fakeVerifier) next(kind string) ([]byte, []byte) {
	f.ceremonies++
	session := []byte(kind + "-" + string(rune('a'+f.ceremonies)))
	return []byte(`{"publicKey":{"challenge":"` + string(session) + `"}}`), session
}
func (f *fakeVerifier) BeginRegistration(domain.User, []domain.WebAuthnCredential) ([]byte, []byte, error) {
	options, session := f.next("reg")
	return options, session, nil
}
func (f *fakeVerifier) FinishRegistration(_ domain.User, _ []domain.WebAuthnCredential, session, response []byte) ([]byte, []byte, error) {
	if string(response) != "ok:"+string(session) {
		return nil, nil, domain.ErrInvalidWebAuthnAnswer
	}
	return []byte("cred-" + string(session)), []byte("data"), nil
}
func (f *fakeVerifier) BeginLogin(_ domain.User, existing []domain.WebAuthnCredential) ([]byte, []byte, error) {
	if len(existing) == 0 {
		return nil, nil, domain.ErrWebAuthnKeyNotFound
	}
	options, session := f.next("login")
	return options, session, nil
}
func (f *fakeVerifier) FinishLogin(_ domain.User, existing []domain.WebAuthnCredential, session, response []byte) ([]byte, []byte, error) {
	prefix := "ok:" + string(session) + ":"
	if !strings.HasPrefix(string(response), prefix) {
		return nil, nil, domain.ErrInvalidWebAuthnAnswer
	}
	id := []byte(strings.TrimPrefix(string(response), prefix))
	for _, key := range existing {
		if bytes.Equal(key.CredentialID, id) {
			return id, []byte("data-used"), nil
		}
	}
	return nil, nil, domain.ErrInvalidWebAuthnAnswer
}

func (f *fakeVerifier) BeginPasskeyLogin() ([]byte, []byte, error) {
	options, session := f.next("passkey")
	return options, session, nil
}

// FinishPasskeyLogin accepts "ok:<session>:<user UUID>:<credential>".
func (f *fakeVerifier) FinishPasskeyLogin(session, response []byte, lookup ports.PasskeyLookup) (domain.User, []byte, []byte, error) {
	rest, ok := strings.CutPrefix(string(response), "ok:"+string(session)+":")
	handle, credential, found := strings.Cut(rest, ":")
	id, err := uuid.Parse(handle)
	if !ok || !found || err != nil {
		return domain.User{}, nil, nil, domain.ErrInvalidWebAuthnAnswer
	}
	user, existing, err := lookup(id[:])
	if errors.Is(err, domain.ErrWebAuthnKeyNotFound) {
		return domain.User{}, nil, nil, domain.ErrInvalidWebAuthnAnswer
	}
	if err != nil {
		return domain.User{}, nil, nil, err
	}
	for _, key := range existing {
		if bytes.Equal(key.CredentialID, []byte(credential)) {
			return user, key.CredentialID, []byte("data-passkey"), nil
		}
	}
	return domain.User{}, nil, nil, domain.ErrInvalidWebAuthnAnswer
}

func sessionOf(t *testing.T, options []byte) string {
	t.Helper()
	_, rest, ok := strings.Cut(string(options), `"challenge":"`)
	if !ok {
		t.Fatalf("options = %s", options)
	}
	return strings.TrimSuffix(rest, `"}}`)
}

func TestWebAuthnKeyLifecycle(t *testing.T) {
	factors := newMemoryFactors()
	usecase := newFactorUsecase(factors, true).WithWebAuthn(&fakeVerifier{}, factors)
	clock := factorClock
	usecase.now = func() time.Time { return clock }
	ctx := context.Background()

	if status, _ := usecase.Status(ctx, securityAlice); !status.WebAuthnAvailable || len(status.Keys) != 0 {
		t.Fatalf("status before = %+v", status)
	}
	if _, err := usecase.BeginWebAuthn(ctx, securityAlice, "wrong"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("begin with a wrong password = %v", err)
	}
	if _, _, err := usecase.FinishWebAuthn(ctx, securityAlice, "Clé", []byte("ok:x"), ""); !errors.Is(err, domain.ErrNoWebAuthnCeremony) {
		t.Fatalf("finish without ceremony = %v", err)
	}
	options, err := usecase.BeginWebAuthn(ctx, securityAlice, "right-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := usecase.FinishWebAuthn(ctx, securityAlice, "  ", []byte("ok:"+sessionOf(t, options)), ""); !errors.Is(err, domain.ErrInvalidWebAuthnName) {
		t.Fatalf("empty name = %v", err)
	}
	if _, _, err := usecase.FinishWebAuthn(ctx, securityAlice, "Clé", []byte("bad"), ""); !errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
		t.Fatalf("invalid answer = %v", err)
	}
	// The ceremony answered once: the right answer now comes too late.
	if _, _, err := usecase.FinishWebAuthn(ctx, securityAlice, "Clé", []byte("ok:"+sessionOf(t, options)), ""); !errors.Is(err, domain.ErrNoWebAuthnCeremony) {
		t.Fatalf("second answer = %v", err)
	}

	options, _ = usecase.BeginWebAuthn(ctx, securityAlice, "right-password")
	key, codes, err := usecase.FinishWebAuthn(ctx, securityAlice, " YubiKey ", []byte("ok:"+sessionOf(t, options)), "req")
	if err != nil || key.Name != "YubiKey" || len(codes) != domain.RecoveryCodeCount || key.Data != nil {
		t.Fatalf("first key = (%+v, %d codes, %v)", key, len(codes), err)
	}
	options, _ = usecase.BeginWebAuthn(ctx, securityAlice, "right-password")
	_, again, err := usecase.FinishWebAuthn(ctx, securityAlice, "Téléphone", []byte("ok:"+sessionOf(t, options)), "req")
	if err != nil || again != nil {
		t.Fatalf("second key = (%d codes, %v), want no new codes", len(again), err)
	}
	status, _ := usecase.Status(ctx, securityAlice)
	if len(status.Keys) != 2 || status.Keys[0].Data != nil || status.Keys[0].CredentialID != nil || status.Enabled {
		t.Fatalf("status = %+v", status)
	}

	clock = factorClock.Add(domain.WebAuthnCeremonyTTL)
	options, _ = usecase.BeginWebAuthn(ctx, securityAlice, "right-password")
	clock = factorClock.Add(2 * domain.WebAuthnCeremonyTTL)
	if _, _, err := usecase.FinishWebAuthn(ctx, securityAlice, "Tard", []byte("ok:"+sessionOf(t, options)), ""); !errors.Is(err, domain.ErrNoWebAuthnCeremony) {
		t.Fatalf("expired ceremony = %v", err)
	}

	if err := usecase.DeleteWebAuthn(ctx, securityAlice, "wrong", key.ID, ""); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("delete with a wrong password = %v", err)
	}
	if err := usecase.DeleteWebAuthn(ctx, securityAlice, "right-password", key.ID, ""); err != nil {
		t.Fatal(err)
	}
	if status, _ := usecase.Status(ctx, securityAlice); len(status.Keys) != 1 || status.RecoveryCodesRemaining != domain.RecoveryCodeCount {
		t.Fatalf("status after delete = %+v", status)
	}
	// An account with keys only can regenerate its codes and disable all.
	if _, err := usecase.RegenerateRecoveryCodes(ctx, securityAlice, "right-password", ""); err != nil {
		t.Fatalf("regenerate with keys only = %v", err)
	}
	if err := usecase.Disable(ctx, securityAlice, "right-password", ""); err != nil {
		t.Fatalf("disable with keys only = %v", err)
	}
	if status, _ := usecase.Status(ctx, securityAlice); len(status.Keys) != 0 || status.RecoveryCodesRemaining != 0 {
		t.Fatalf("status after disable = %+v", status)
	}
	actions := []string{}
	for _, audit := range factors.audits {
		actions = append(actions, audit.Action)
	}
	if strings.Join(actions, ",") != "webauthn_add,webauthn_add,webauthn_remove,recovery_codes_regenerate,second_factor_disable" {
		t.Fatalf("audited actions = %v", actions)
	}
}

func TestWebAuthnNeedsTheVerifier(t *testing.T) {
	factors := newMemoryFactors()
	usecase := newFactorUsecase(factors, true)
	if _, err := usecase.BeginWebAuthn(context.Background(), securityAlice, "right-password"); !errors.Is(err, domain.ErrWebAuthnUnavailable) {
		t.Fatalf("begin without WebAuthn = %v", err)
	}
	if status, _ := usecase.Status(context.Background(), securityAlice); status.WebAuthnAvailable {
		t.Fatal("WebAuthn reported available")
	}
}

func TestLoginWithASecurityKey(t *testing.T) {
	factors := newMemoryFactors()
	factors.keys[securityAlice.ID] = []domain.WebAuthnCredential{{ID: uuid.New(), UserID: securityAlice.ID, Name: "YubiKey", CredentialID: []byte("cred-1"), Data: []byte("data")}}
	factors.setCodes(securityAlice.ID, []string{domain.RecoveryCodeHash("aaaa-bbbb-cccc")})
	auth, sessions, failures, _ := newTwoStepAuth(factors, true)
	auth.WithWebAuthn(&fakeVerifier{}, factors)
	ctx := context.Background()

	result, err := auth.Login(ctx, "alice", "right-password", "", "")
	if err != nil || result.Challenge == "" {
		t.Fatalf("password step = (%+v, %v)", result, err)
	}
	if !slices.Equal(result.Methods, []domain.SecondFactorMethod{domain.SecondFactorWebAuthn, domain.SecondFactorRecovery}) {
		t.Fatalf("methods = %v", result.Methods)
	}
	if _, err := auth.BeginWebAuthnLogin(ctx, "unknown"); !errors.Is(err, domain.ErrLoginChallengeExpired) {
		t.Fatalf("unknown challenge = %v", err)
	}
	// Without a ceremony, an answer is refused and counts as a failure.
	if _, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorWebAuthn, "ok:x:cred-1", "", ""); !errors.Is(err, domain.ErrInvalidSecondFactor) {
		t.Fatalf("answer without ceremony = %v", err)
	}
	options, err := auth.BeginWebAuthnLogin(ctx, result.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	session := sessionOf(t, options)
	if _, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorWebAuthn, "ok:"+session+":cred-2", "", ""); !errors.Is(err, domain.ErrInvalidSecondFactor) {
		t.Fatalf("unknown credential = %v", err)
	}
	if failures.failures[securityAlice.ID].Count != 2 {
		t.Fatalf("failures = %d", failures.failures[securityAlice.ID].Count)
	}
	// The ceremony ended with the wrong answer.
	if _, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorWebAuthn, "ok:"+session+":cred-1", "", ""); !errors.Is(err, domain.ErrInvalidSecondFactor) {
		t.Fatalf("answer to an ended ceremony = %v", err)
	}
	options, _ = auth.BeginWebAuthnLogin(ctx, result.Challenge)
	opened, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorWebAuthn, "ok:"+sessionOf(t, options)+":cred-1", "", "")
	if err != nil || opened.Token == "" {
		t.Fatalf("right answer = (%+v, %v)", opened, err)
	}
	stored, _ := sessions.FindActiveByTokenHash(ctx, sha256Hex(opened.Token))
	if stored.AuthMethod != "password+webauthn" {
		t.Fatalf("auth method = %q", stored.AuthMethod)
	}
	if key := factors.keys[securityAlice.ID][0]; string(key.Data) != "data-used" || key.LastUsedAt == nil {
		t.Fatalf("key after login = %+v", key)
	}
}

func TestLoginMethodsWithoutWebAuthn(t *testing.T) {
	factors := newMemoryFactors()
	enrolledAlice(factors)
	factors.keys[securityAlice.ID] = []domain.WebAuthnCredential{{ID: uuid.New(), UserID: securityAlice.ID, CredentialID: []byte("cred-1")}}
	auth, _, _, _ := newTwoStepAuth(factors, true)
	result, _ := auth.Login(context.Background(), "alice", "right-password", "", "")
	if !slices.Equal(result.Methods, []domain.SecondFactorMethod{domain.SecondFactorTOTP, domain.SecondFactorRecovery}) {
		t.Fatalf("methods = %v", result.Methods)
	}
	if _, err := auth.BeginWebAuthnLogin(context.Background(), result.Challenge); !errors.Is(err, domain.ErrWebAuthnUnavailable) {
		t.Fatalf("key login without WebAuthn = %v", err)
	}
}
