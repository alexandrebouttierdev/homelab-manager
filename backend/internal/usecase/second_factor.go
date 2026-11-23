package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
)

// totpIssuer names the Manager in authenticator apps.
const totpIssuer = "Homelab Manager"

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

	// mu serialises the changes of second factors, so that a confirmation
	// cannot interleave with a restart of the enrollment.
	mu sync.Mutex
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
	return domain.SecondFactorStatus{
		Enabled: found && enrollment.Confirmed, Pending: found && !enrollment.Confirmed,
		RecoveryCodesRemaining: remaining, Available: u.box != nil,
	}, nil
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

// Disable removes the TOTP secret, pending or enabled, and the recovery
// codes.
func (u *SecondFactorUsecase) Disable(ctx context.Context, user domain.User, password, requestID string) error {
	if err := u.verifyPassword(ctx, user, password); err != nil {
		return err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, found, err := u.factors.TOTP(ctx, user.ID); err != nil {
		return err
	} else if !found {
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
