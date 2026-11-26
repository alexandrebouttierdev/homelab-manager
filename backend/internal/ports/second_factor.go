package ports

import (
	"context"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

// SecondFactorRepository stores the TOTP secret and the recovery codes of
// each account (SPEC_ADMINISTRATION.md § 3.3).
type SecondFactorRepository interface {
	// TOTP reports whether the account has a TOTP secret, pending or
	// confirmed.
	TOTP(ctx context.Context, userID uuid.UUID) (domain.TOTPEnrollment, bool, error)
	// SavePendingTOTP stores a secret to confirm, replacing a pending one;
	// domain.ErrSecondFactorAlreadyEnabled when one is confirmed.
	SavePendingTOTP(ctx context.Context, userID uuid.UUID, sealed string) error
	// ConfirmTOTP enables the pending secret, records the step of the
	// confirmation code and replaces the recovery codes, atomically.
	ConfirmTOTP(ctx context.Context, userID uuid.UUID, step int64, codeHashes []string, audit domain.AuditEntry) error
	// UseTOTPStep records a step and reports false when it is not newer
	// than the last one used (anti-replay).
	UseTOTPStep(ctx context.Context, userID uuid.UUID, step int64) (bool, error)
	// DisableSecondFactor removes the secret and the recovery codes.
	DisableSecondFactor(ctx context.Context, userID uuid.UUID, audit domain.AuditEntry) error
	ReplaceRecoveryCodes(ctx context.Context, userID uuid.UUID, codeHashes []string, audit domain.AuditEntry) error
	// UseRecoveryCode marks an unused code as used and reports whether it
	// existed.
	UseRecoveryCode(ctx context.Context, userID uuid.UUID, codeHash string) (bool, error)
	RemainingRecoveryCodes(ctx context.Context, userID uuid.UUID) (int, error)
	// HasSecondFactor reports a confirmed TOTP secret.
	HasSecondFactor(ctx context.Context, userID uuid.UUID) (bool, error)
}
