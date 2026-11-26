package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"

	"github.com/google/uuid"
)

// totpIssuer names the Manager in authenticator apps.
const totpIssuer = "MonoOps"

// totpAssociatedData binds a sealed TOTP secret to its account.
func totpAssociatedData(userID uuid.UUID) string {
	return "totp:" + userID.String()
}

// TOTPEnrollmentStart is what an authenticator app needs to add the account.
type TOTPEnrollmentStart struct {
	Secret string
	URI    string
	QR     domain.QRMatrix
}

// SecondFactorUsecase lets each account manage its own second factor
// (SPEC_ADMINISTRATION.md § 3.3). Without the secrets key (box nil), TOTP
// cannot be enrolled; recovery codes keep working for login.
type SecondFactorUsecase struct {
	users   ports.UserRepository
	factors ports.SecondFactorRepository
	box     ports.SecretBox
	qr      ports.QREncoder
	hasher  ports.PasswordHasher
	now     func() time.Time

	// verifier and keys are nil without WebAuthn (no webauthn_rp_id).
	verifier ports.WebAuthnVerifier
	keys     ports.WebAuthnRepository

	// mu serialises the changes of second factors, so that a confirmation
	// cannot interleave with a restart of the enrollment.
	mu sync.Mutex
	// registrations holds the key registration started by each account,
	// for domain.WebAuthnCeremonyTTL.
	registrations map[uuid.UUID]pendingRegistration
}

type pendingRegistration struct {
	session   []byte
	expiresAt time.Time
}

// WithWebAuthn enables the passkeys and security keys.
func (u *SecondFactorUsecase) WithWebAuthn(verifier ports.WebAuthnVerifier, keys ports.WebAuthnRepository) *SecondFactorUsecase {
	u.verifier, u.keys = verifier, keys
	u.registrations = map[uuid.UUID]pendingRegistration{}
	return u
}

func NewSecondFactorUsecase(users ports.UserRepository, factors ports.SecondFactorRepository, box ports.SecretBox, qr ports.QREncoder, hasher ports.PasswordHasher) *SecondFactorUsecase {
	return &SecondFactorUsecase{users: users, factors: factors, box: box, qr: qr, hasher: hasher, now: time.Now}
}

func (u *SecondFactorUsecase) Status(ctx context.Context, user domain.User) (domain.SecondFactorStatus, error) {
	enrollment, found, err := u.factors.TOTP(ctx, user.ID)
	if err != nil {
		return domain.SecondFactorStatus{}, err
	}
	remaining, err := u.factors.RemainingRecoveryCodes(ctx, user.ID)
	if err != nil {
		return domain.SecondFactorStatus{}, err
	}
	status := domain.SecondFactorStatus{
		Enabled: found && enrollment.Confirmed, Pending: found && !enrollment.Confirmed,
		RecoveryCodesRemaining: remaining, Available: u.box != nil,
		Keys: []domain.WebAuthnCredential{}, WebAuthnAvailable: u.verifier != nil,
	}
	if u.keys != nil {
		keys, err := u.keys.WebAuthnCredentials(ctx, user.ID)
		if err != nil {
			return domain.SecondFactorStatus{}, err
		}
		for _, key := range keys {
			// The stored credential stays in the adapter.
			key.Data, key.CredentialID = nil, nil
			status.Keys = append(status.Keys, key)
		}
	}
	return status, nil
}

// BeginWebAuthn starts the registration of a passkey or a security key and
// returns the options for the browser. The current password is required.
func (u *SecondFactorUsecase) BeginWebAuthn(ctx context.Context, user domain.User, password string) ([]byte, error) {
	if u.verifier == nil || u.keys == nil {
		return nil, domain.ErrWebAuthnUnavailable
	}
	if err := u.verifyPassword(ctx, user, password); err != nil {
		return nil, err
	}
	existing, err := u.keys.WebAuthnCredentials(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if len(existing) >= domain.MaxWebAuthnCredentials {
		return nil, domain.ErrTooManyWebAuthnKeys
	}
	options, session, err := u.verifier.BeginRegistration(user, existing)
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	now := u.now()
	for id, pending := range u.registrations {
		if !now.Before(pending.expiresAt) {
			delete(u.registrations, id)
		}
	}
	u.registrations[user.ID] = pendingRegistration{session: session, expiresAt: now.Add(domain.WebAuthnCeremonyTTL)}
	return options, nil
}

// FinishWebAuthn checks the answer of the browser and stores the key under
// its name. The first second factor of an account also gets recovery
// codes, returned this once; otherwise codes is nil.
func (u *SecondFactorUsecase) FinishWebAuthn(ctx context.Context, user domain.User, name string, response []byte, requestID string) (domain.WebAuthnCredential, []string, error) {
	if u.verifier == nil || u.keys == nil {
		return domain.WebAuthnCredential{}, nil, domain.ErrWebAuthnUnavailable
	}
	name, err := domain.ValidWebAuthnName(name)
	if err != nil {
		return domain.WebAuthnCredential{}, nil, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	pending, ok := u.registrations[user.ID]
	// A ceremony answers once, right or wrong.
	delete(u.registrations, user.ID)
	if !ok || !u.now().Before(pending.expiresAt) {
		return domain.WebAuthnCredential{}, nil, domain.ErrNoWebAuthnCeremony
	}
	existing, err := u.keys.WebAuthnCredentials(ctx, user.ID)
	if err != nil {
		return domain.WebAuthnCredential{}, nil, err
	}
	credentialID, data, err := u.verifier.FinishRegistration(user, existing, pending.session, response)
	if err != nil {
		return domain.WebAuthnCredential{}, nil, err
	}
	var codes, hashes []string
	remaining, err := u.factors.RemainingRecoveryCodes(ctx, user.ID)
	if err != nil {
		return domain.WebAuthnCredential{}, nil, err
	}
	if remaining == 0 {
		if codes, hashes, err = newRecoveryCodes(); err != nil {
			return domain.WebAuthnCredential{}, nil, err
		}
	}
	key := domain.WebAuthnCredential{
		ID: uuid.New(), UserID: user.ID, Name: name, CredentialID: credentialID, Data: data, CreatedAt: u.now().UTC(),
	}
	if err := u.keys.AddWebAuthnCredential(ctx, key, hashes, selfAudit(user, "webauthn_add", requestID)); err != nil {
		return domain.WebAuthnCredential{}, nil, err
	}
	key.Data, key.CredentialID = nil, nil
	return key, codes, nil
}

// DeleteWebAuthn removes a key of the account. The current password is
// required.
func (u *SecondFactorUsecase) DeleteWebAuthn(ctx context.Context, user domain.User, password string, id uuid.UUID, requestID string) error {
	if u.keys == nil {
		return domain.ErrWebAuthnUnavailable
	}
	if err := u.verifyPassword(ctx, user, password); err != nil {
		return err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.keys.DeleteWebAuthnCredential(ctx, user.ID, id, selfAudit(user, "webauthn_remove", requestID))
}

// BeginTOTP stores a new secret to confirm, replacing a pending one. The
// current password is required.
func (u *SecondFactorUsecase) BeginTOTP(ctx context.Context, user domain.User, password string) (TOTPEnrollmentStart, error) {
	if u.box == nil {
		return TOTPEnrollmentStart{}, domain.ErrSecondFactorUnavailable
	}
	if err := u.verifyPassword(ctx, user, password); err != nil {
		return TOTPEnrollmentStart{}, err
	}
	secret, err := domain.NewTOTPSecret()
	if err != nil {
		return TOTPEnrollmentStart{}, err
	}
	sealed, err := u.box.Seal(secret, totpAssociatedData(user.ID))
	if err != nil {
		return TOTPEnrollmentStart{}, fmt.Errorf("seal TOTP secret: %w", err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.factors.SavePendingTOTP(ctx, user.ID, sealed); err != nil {
		return TOTPEnrollmentStart{}, err
	}
	uri := domain.TOTPURI(totpIssuer, user.Username, secret)
	qr, err := u.qr.Encode(uri)
	if err != nil {
		return TOTPEnrollmentStart{}, err
	}
	return TOTPEnrollmentStart{Secret: domain.TOTPSecretText(secret), URI: uri, QR: qr}, nil
}

// ConfirmTOTP enables the pending secret with a first valid code and
// returns the recovery codes, shown only this once.
func (u *SecondFactorUsecase) ConfirmTOTP(ctx context.Context, user domain.User, code, requestID string) ([]string, error) {
	if u.box == nil {
		return nil, domain.ErrSecondFactorUnavailable
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	enrollment, found, err := u.factors.TOTP(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if !found || enrollment.Confirmed {
		return nil, domain.ErrNoPendingEnrollment
	}
	secret, err := u.box.Open(enrollment.Sealed, totpAssociatedData(user.ID))
	if err != nil {
		return nil, fmt.Errorf("open TOTP secret: %w", err)
	}
	step, ok := domain.MatchTOTP(secret, code, u.now())
	if !ok {
		return nil, domain.ErrInvalidSecondFactor
	}
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := u.factors.ConfirmTOTP(ctx, user.ID, step, hashes, selfAudit(user, "second_factor_enable", requestID)); err != nil {
		return nil, err
	}
	return codes, nil
}

// RegenerateRecoveryCodes replaces the recovery codes of an enabled second
// factor and returns the new ones, shown only this once.
func (u *SecondFactorUsecase) RegenerateRecoveryCodes(ctx context.Context, user domain.User, password, requestID string) ([]string, error) {
	if err := u.verifyPassword(ctx, user, password); err != nil {
		return nil, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	enabled, err := u.factors.HasSecondFactor(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, domain.ErrSecondFactorNotEnabled
	}
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := u.factors.ReplaceRecoveryCodes(ctx, user.ID, hashes, selfAudit(user, "recovery_codes_regenerate", requestID)); err != nil {
		return nil, err
	}
	return codes, nil
}

// Disable removes every second factor: the TOTP secret, pending or
// enabled, the security keys and the recovery codes.
func (u *SecondFactorUsecase) Disable(ctx context.Context, user domain.User, password, requestID string) error {
	if err := u.verifyPassword(ctx, user, password); err != nil {
		return err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	_, found, err := u.factors.TOTP(ctx, user.ID)
	if err != nil {
		return err
	}
	enabled, err := u.factors.HasSecondFactor(ctx, user.ID)
	if err != nil {
		return err
	}
	if !found && !enabled {
		return domain.ErrSecondFactorNotEnabled
	}
	return u.factors.DisableSecondFactor(ctx, user.ID, selfAudit(user, "second_factor_disable", requestID))
}

func (u *SecondFactorUsecase) verifyPassword(ctx context.Context, user domain.User, password string) error {
	_, storedHash, err := u.users.FindByUsername(ctx, user.Username)
	if err != nil {
		return err
	}
	valid, err := u.hasher.Verify(password, storedHash)
	if err != nil || !valid {
		return domain.ErrInvalidCredentials
	}
	return nil
}

func newRecoveryCodes() (codes, hashes []string, err error) {
	codes, err = domain.NewRecoveryCodes()
	if err != nil {
		return nil, nil, err
	}
	hashes = make([]string, len(codes))
	for i, code := range codes {
		hashes[i] = domain.RecoveryCodeHash(code)
	}
	return codes, hashes, nil
}

func selfAudit(user domain.User, action, requestID string) domain.AuditEntry {
	return domain.AuditEntry{
		Actor: user.Username, TargetType: auditTargetUser, TargetID: user.ID.String(),
		Action: action, RequestID: requestID, Outcome: domain.AuditSuccess,
	}
}
