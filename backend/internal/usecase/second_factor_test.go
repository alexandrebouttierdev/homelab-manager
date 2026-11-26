package usecase

import (
	"context"
	"encoding/base32"
	"errors"
	"strings"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

// fakeBox seals by tagging with the associated data; Open checks it.
type fakeBox struct{}

func (fakeBox) Seal(plaintext []byte, associated string) (string, error) {
	return associated + "|" + string(plaintext), nil
}
func (fakeBox) Open(sealed, associated string) ([]byte, error) {
	value, ok := strings.CutPrefix(sealed, associated+"|")
	if !ok {
		return nil, errors.New("wrong associated data")
	}
	return []byte(value), nil
}

type fakeQR struct{}

func (fakeQR) Encode(string) (domain.QRMatrix, error) {
	return domain.QRMatrix{Size: 21, Rows: make([]string, 21)}, nil
}

// memoryFactors is an in-memory ports.SecondFactorRepository.
type memoryFactors struct {
	totp     map[uuid.UUID]domain.TOTPEnrollment
	recovery map[uuid.UUID]map[string]bool // hash → used
	audits   []domain.AuditEntry
	keys     map[uuid.UUID][]domain.WebAuthnCredential
}

func newMemoryFactors() *memoryFactors {
	return &memoryFactors{totp: map[uuid.UUID]domain.TOTPEnrollment{}, recovery: map[uuid.UUID]map[string]bool{}, keys: map[uuid.UUID][]domain.WebAuthnCredential{}}
}

func (m *memoryFactors) TOTP(_ context.Context, userID uuid.UUID) (domain.TOTPEnrollment, bool, error) {
	enrollment, ok := m.totp[userID]
	return enrollment, ok, nil
}
func (m *memoryFactors) SavePendingTOTP(_ context.Context, userID uuid.UUID, sealed string) error {
	if m.totp[userID].Confirmed {
		return domain.ErrSecondFactorAlreadyEnabled
	}
	m.totp[userID] = domain.TOTPEnrollment{Sealed: sealed}
	return nil
}
func (m *memoryFactors) ConfirmTOTP(_ context.Context, userID uuid.UUID, step int64, hashes []string, audit domain.AuditEntry) error {
	enrollment, ok := m.totp[userID]
	if !ok || enrollment.Confirmed {
		return domain.ErrNoPendingEnrollment
	}
	enrollment.Confirmed, enrollment.LastUsedStep = true, step
	m.totp[userID] = enrollment
	m.setCodes(userID, hashes)
	m.audits = append(m.audits, audit)
	return nil
}
func (m *memoryFactors) UseTOTPStep(_ context.Context, userID uuid.UUID, step int64) (bool, error) {
	enrollment := m.totp[userID]
	if !enrollment.Confirmed || step <= enrollment.LastUsedStep {
		return false, nil
	}
	enrollment.LastUsedStep = step
	m.totp[userID] = enrollment
	return true, nil
}
func (m *memoryFactors) DisableSecondFactor(_ context.Context, userID uuid.UUID, audit domain.AuditEntry) error {
	delete(m.totp, userID)
	delete(m.recovery, userID)
	delete(m.keys, userID)
	m.audits = append(m.audits, audit)
	return nil
}
func (m *memoryFactors) ReplaceRecoveryCodes(_ context.Context, userID uuid.UUID, hashes []string, audit domain.AuditEntry) error {
	m.setCodes(userID, hashes)
	m.audits = append(m.audits, audit)
	return nil
}
func (m *memoryFactors) setCodes(userID uuid.UUID, hashes []string) {
	m.recovery[userID] = map[string]bool{}
	for _, hash := range hashes {
		m.recovery[userID][hash] = false
	}
}
func (m *memoryFactors) UseRecoveryCode(_ context.Context, userID uuid.UUID, hash string) (bool, error) {
	used, ok := m.recovery[userID][hash]
	if !ok || used {
		return false, nil
	}
	m.recovery[userID][hash] = true
	return true, nil
}
func (m *memoryFactors) RemainingRecoveryCodes(_ context.Context, userID uuid.UUID) (int, error) {
	count := 0
	for _, used := range m.recovery[userID] {
		if !used {
			count++
		}
	}
	return count, nil
}
func (m *memoryFactors) HasSecondFactor(_ context.Context, userID uuid.UUID) (bool, error) {
	return m.totp[userID].Confirmed || len(m.keys[userID]) > 0, nil
}

var factorClock = time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

func newFactorUsecase(factors *memoryFactors, box bool) *SecondFactorUsecase {
	users := usersByName{"admin": securityAdmin, "alice": securityAlice, "bob": securityBob}
	var secrets fakeBox
	usecase := NewSecondFactorUsecase(users, factors, nil, fakeQR{}, &checkingHasher{})
	if box {
		usecase = NewSecondFactorUsecase(users, factors, secrets, fakeQR{}, &checkingHasher{})
	}
	usecase.now = func() time.Time { return factorClock }
	return usecase
}

func TestTOTPEnrollmentLifecycle(t *testing.T) {
	factors := newMemoryFactors()
	usecase := newFactorUsecase(factors, true)
	ctx := context.Background()

	if status, err := usecase.Status(ctx, securityAlice); err != nil || status.Enabled || !status.Available {
		t.Fatalf("status before enrollment = (%+v, %v)", status, err)
	}
	if _, err := usecase.BeginTOTP(ctx, securityAlice, "wrong"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("begin with a wrong password = %v", err)
	}
	start, err := usecase.BeginTOTP(ctx, securityAlice, "right-password")
	if err != nil || len(start.Secret) != 32 || !strings.Contains(start.URI, "alice") || start.QR.Size != 21 {
		t.Fatalf("BeginTOTP = (%+v, %v)", start, err)
	}
	if status, _ := usecase.Status(ctx, securityAlice); !status.Pending || status.Enabled {
		t.Fatalf("status while pending = %+v", status)
	}
	if _, err := usecase.ConfirmTOTP(ctx, securityAlice, "000000", ""); !errors.Is(err, domain.ErrInvalidSecondFactor) {
		t.Fatalf("confirm with a wrong code = %v", err)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(start.Secret)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := usecase.ConfirmTOTP(ctx, securityAlice, domain.TOTPCode(secret, domain.TOTPStep(factorClock)), "req-1")
	if err != nil || len(codes) != domain.RecoveryCodeCount {
		t.Fatalf("ConfirmTOTP = (%d codes, %v)", len(codes), err)
	}
	if status, _ := usecase.Status(ctx, securityAlice); !status.Enabled || status.RecoveryCodesRemaining != domain.RecoveryCodeCount {
		t.Fatalf("status after confirmation = %+v", status)
	}
	if _, err := usecase.BeginTOTP(ctx, securityAlice, "right-password"); !errors.Is(err, domain.ErrSecondFactorAlreadyEnabled) {
		t.Fatalf("second enrollment = %v", err)
	}
	fresh, err := usecase.RegenerateRecoveryCodes(ctx, securityAlice, "right-password", "")
	if err != nil || len(fresh) != domain.RecoveryCodeCount || fresh[0] == codes[0] {
		t.Fatalf("RegenerateRecoveryCodes = (%v, %v)", fresh, err)
	}
	if err := usecase.Disable(ctx, securityAlice, "wrong", ""); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("disable with a wrong password = %v", err)
	}
	if err := usecase.Disable(ctx, securityAlice, "right-password", ""); err != nil {
		t.Fatal(err)
	}
	if err := usecase.Disable(ctx, securityAlice, "right-password", ""); !errors.Is(err, domain.ErrSecondFactorNotEnabled) {
		t.Fatalf("disable twice = %v", err)
	}
	actions := []string{}
	for _, audit := range factors.audits {
		actions = append(actions, audit.Action)
	}
	if strings.Join(actions, ",") != "second_factor_enable,recovery_codes_regenerate,second_factor_disable" {
		t.Fatalf("audited actions = %v", actions)
	}
}

func TestEnrollmentNeedsTheSecretsKey(t *testing.T) {
	usecase := newFactorUsecase(newMemoryFactors(), false)
	if status, err := usecase.Status(context.Background(), securityAlice); err != nil || status.Available {
		t.Fatalf("status without key = (%+v, %v)", status, err)
	}
	if _, err := usecase.BeginTOTP(context.Background(), securityAlice, "right-password"); !errors.Is(err, domain.ErrSecondFactorUnavailable) {
		t.Fatalf("begin without key = %v", err)
	}
}

// enrolledAlice gives alice a confirmed TOTP secret and recovery codes.
func enrolledAlice(factors *memoryFactors) ([]byte, string) {
	secret := []byte("12345678901234567890")
	sealed, _ := fakeBox{}.Seal(secret, totpAssociatedData(securityAlice.ID))
	factors.totp[securityAlice.ID] = domain.TOTPEnrollment{Sealed: sealed, Confirmed: true}
	factors.setCodes(securityAlice.ID, []string{domain.RecoveryCodeHash("aaaa-bbbb-cccc")})
	return secret, "aaaa-bbbb-cccc"
}

func newTwoStepAuth(factors *memoryFactors, box bool) (*AuthUsecase, *memorySessions, *memoryFailures, *time.Time) {
	sessions := newMemorySessions()
	auth, failures, _, _, now := newSecureAuth(sessions, domain.DefaultSecurityPolicy())
	if box {
		auth.WithSecondFactor(factors, fakeBox{})
	} else {
		auth.WithSecondFactor(factors, nil)
	}
	return auth, sessions, failures, now
}

func TestLoginRequiresTheSecondFactor(t *testing.T) {
	factors := newMemoryFactors()
	secret, recovery := enrolledAlice(factors)
	auth, sessions, failures, now := newTwoStepAuth(factors, true)
	ctx := context.Background()

	result, err := auth.Login(ctx, "alice", "right-password", "192.0.2.1", "test")
	if err != nil || result.Challenge == "" || result.Token != "" {
		t.Fatalf("password step = (%+v, %v), want a challenge and no session", result, err)
	}
	if _, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorTOTP, "000000", "192.0.2.1", "test"); !errors.Is(err, domain.ErrInvalidSecondFactor) {
		t.Fatalf("wrong code = %v", err)
	}
	if failures.failures[securityAlice.ID].Count != 1 {
		t.Fatal("a wrong code counts as a login failure")
	}
	code := domain.TOTPCode(secret, domain.TOTPStep(*now))
	session, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorTOTP, code, "192.0.2.1", "test")
	if err != nil || session.Token == "" || session.User.ID != securityAlice.ID {
		t.Fatalf("right code = (%+v, %v)", session, err)
	}
	if _, ok := failures.failures[securityAlice.ID]; ok {
		t.Fatal("a complete login clears the failures")
	}
	opened, _ := sessions.FindActiveByTokenHash(ctx, sha256Hex(session.Token))
	if opened.AuthMethod != "password+totp" {
		t.Fatalf("auth method = %q", opened.AuthMethod)
	}
	if _, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorTOTP, code, "", ""); !errors.Is(err, domain.ErrLoginChallengeExpired) {
		t.Fatalf("reused challenge = %v", err)
	}

	again, _ := auth.Login(ctx, "alice", "right-password", "", "")
	if _, err := auth.CompleteSecondFactor(ctx, again.Challenge, domain.SecondFactorTOTP, code, "", ""); !errors.Is(err, domain.ErrInvalidSecondFactor) {
		t.Fatalf("replayed code = %v, want refused", err)
	}
	if _, err := auth.CompleteSecondFactor(ctx, again.Challenge, domain.SecondFactorRecovery, strings.ToUpper(recovery), "", ""); err != nil {
		t.Fatalf("recovery code = %v", err)
	}
	third, _ := auth.Login(ctx, "alice", "right-password", "", "")
	if _, err := auth.CompleteSecondFactor(ctx, third.Challenge, domain.SecondFactorRecovery, recovery, "", ""); !errors.Is(err, domain.ErrInvalidSecondFactor) {
		t.Fatalf("reused recovery code = %v", err)
	}

	*now = now.Add(loginChallengeTTL + time.Second)
	if _, err := auth.CompleteSecondFactor(ctx, third.Challenge, domain.SecondFactorTOTP, domain.TOTPCode(secret, domain.TOTPStep(*now)), "", ""); !errors.Is(err, domain.ErrLoginChallengeExpired) {
		t.Fatalf("expired challenge = %v", err)
	}
}

func TestWrongCodesEndTheChallengeAndLockTheAccount(t *testing.T) {
	factors := newMemoryFactors()
	enrolledAlice(factors)
	auth, _, failures, now := newTwoStepAuth(factors, true)
	ctx := context.Background()
	result, _ := auth.Login(ctx, "alice", "right-password", "", "")
	for range maxChallengeAttempts {
		if _, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorTOTP, "000000", "", ""); !errors.Is(err, domain.ErrInvalidSecondFactor) {
			t.Fatalf("wrong code = %v", err)
		}
	}
	if _, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorTOTP, "000000", "", ""); !errors.Is(err, domain.ErrLoginChallengeExpired) {
		t.Fatalf("challenge after %d failures = %v, want expired", maxChallengeAttempts, err)
	}
	if !failures.failures[securityAlice.ID].Locked(*now) {
		t.Fatal("five wrong codes lock the account")
	}
}

func TestRecoveryCodesWorkWithoutTheSecretsKey(t *testing.T) {
	factors := newMemoryFactors()
	secret, recovery := enrolledAlice(factors)
	auth, _, _, now := newTwoStepAuth(factors, false)
	ctx := context.Background()
	result, _ := auth.Login(ctx, "alice", "right-password", "", "")
	if _, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorTOTP, domain.TOTPCode(secret, domain.TOTPStep(*now)), "", ""); !errors.Is(err, domain.ErrSecondFactorUnavailable) {
		t.Fatalf("TOTP without key = %v", err)
	}
	if _, err := auth.CompleteSecondFactor(ctx, result.Challenge, domain.SecondFactorRecovery, recovery, "", ""); err != nil {
		t.Fatalf("recovery code without key = %v", err)
	}
}

func TestAdministratorsMustEnrollWhenRequired(t *testing.T) {
	factors := newMemoryFactors()
	enrolledAlice(factors)
	policy := domain.DefaultSecurityPolicy()
	policy.RequireAdminSecondFactor = true
	sessions := newMemorySessions()
	auth, _, _, _, now := newSecureAuth(sessions, policy)
	auth.WithSecondFactor(factors, fakeBox{})
	sessions.add(securityAdmin, "admin-token", *now)
	sessions.add(securityAlice, "alice-token", *now)

	user, _, err := auth.Session(context.Background(), "admin-token")
	if err != nil || !user.MustEnrollSecondFactor {
		t.Fatalf("admin without factor = (%+v, %v), want MustEnrollSecondFactor", user, err)
	}
	if user, _, err := auth.Session(context.Background(), "alice-token"); err != nil || user.MustEnrollSecondFactor {
		t.Fatalf("viewer = (%+v, %v), want no obligation", user, err)
	}
	// The login response tells the interface too, before any session check.
	result, err := auth.Login(context.Background(), "admin", "right-password", "192.0.2.1", "test")
	if err != nil || result.Token == "" || !result.User.MustEnrollSecondFactor {
		t.Fatalf("admin login = (%+v, %v), want a session and MustEnrollSecondFactor", result.User, err)
	}
	factors.totp[securityAdmin.ID] = domain.TOTPEnrollment{Confirmed: true}
	if user, _, err := auth.Session(context.Background(), "admin-token"); err != nil || user.MustEnrollSecondFactor {
		t.Fatalf("admin with a factor = (%+v, %v)", user, err)
	}
}

func TestRequiringTheSecondFactorNeedsOneOwnFactor(t *testing.T) {
	factors := newMemoryFactors()
	security := NewSecurityUsecase(&memorySecurity{}).WithSecondFactors(factors)
	ctx := context.Background()
	if _, err := security.UpdatePolicy(ctx, securityAdmin, "10.0.0.1", 60, nil, true, false, ""); !errors.Is(err, domain.ErrSecondFactorNotEnrolled) {
		t.Fatalf("requiring without an own factor = %v", err)
	}
	factors.totp[securityAdmin.ID] = domain.TOTPEnrollment{Confirmed: true}
	policy, err := security.UpdatePolicy(ctx, securityAdmin, "10.0.0.1", 60, nil, true, false, "")
	if err != nil || !policy.RequireAdminSecondFactor || !security.Policy().RequireAdminSecondFactor {
		t.Fatalf("UpdatePolicy = (%+v, %v)", policy, err)
	}
}
