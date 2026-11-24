package sqlite

import (
	"context"
	"fmt"
	"time"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/ent/recoverycode"
	"homelab-manager/backend/ent/totpsecret"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
)

var _ ports.SecondFactorRepository = (*SecondFactorRepository)(nil)

// SecondFactorRepository stores TOTP secrets (sealed) and recovery codes
// (hashed).
type SecondFactorRepository struct {
	client *ent.Client
}

func NewSecondFactorRepository(client *ent.Client) *SecondFactorRepository {
	return &SecondFactorRepository{client: client}
}

func (r *SecondFactorRepository) TOTP(ctx context.Context, userID uuid.UUID) (domain.TOTPEnrollment, bool, error) {
	record, err := r.client.TotpSecret.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return domain.TOTPEnrollment{}, false, nil
	}
	if err != nil {
		return domain.TOTPEnrollment{}, false, fmt.Errorf("read TOTP secret: %w", err)
	}
	return domain.TOTPEnrollment{
		Sealed: record.SecretSealed, Confirmed: record.ConfirmedAt != nil, LastUsedStep: record.LastUsedStep,
	}, true, nil
}

func (r *SecondFactorRepository) SavePendingTOTP(ctx context.Context, userID uuid.UUID, sealed string) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		record, err := tx.TotpSecret.Get(ctx, userID)
		switch {
		case ent.IsNotFound(err):
			return tx.TotpSecret.Create().SetID(userID).SetSecretSealed(sealed).Exec(ctx)
		case err != nil:
			return fmt.Errorf("read TOTP secret: %w", err)
		case record.ConfirmedAt != nil:
			return domain.ErrSecondFactorAlreadyEnabled
		default:
			return tx.TotpSecret.UpdateOneID(userID).SetSecretSealed(sealed).SetLastUsedStep(0).Exec(ctx)
		}
	})
}

func (r *SecondFactorRepository) ConfirmTOTP(ctx context.Context, userID uuid.UUID, step int64, codeHashes []string, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		count, err := tx.TotpSecret.Update().
			Where(totpsecret.ID(userID), totpsecret.ConfirmedAtIsNil()).
			SetConfirmedAt(time.Now().UTC()).SetLastUsedStep(step).Save(ctx)
		if err != nil {
			return fmt.Errorf("confirm TOTP secret: %w", err)
		}
		if count == 0 {
			return domain.ErrNoPendingEnrollment
		}
		if err := replaceRecoveryCodes(ctx, tx, userID, codeHashes); err != nil {
			return err
		}
		audit.TargetID = userID.String()
		return writeAudit(ctx, tx, audit)
	})
}

func (r *SecondFactorRepository) UseTOTPStep(ctx context.Context, userID uuid.UUID, step int64) (bool, error) {
	// One statement: two concurrent logins cannot both use the same step.
	count, err := r.client.TotpSecret.Update().
		Where(totpsecret.ID(userID), totpsecret.ConfirmedAtNotNil(), totpsecret.LastUsedStepLT(step)).
		SetLastUsedStep(step).Save(ctx)
	if err != nil {
		return false, fmt.Errorf("record TOTP step: %w", err)
	}
	return count == 1, nil
}

func (r *SecondFactorRepository) DisableSecondFactor(ctx context.Context, userID uuid.UUID, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		if _, err := tx.TotpSecret.Delete().Where(totpsecret.ID(userID)).Exec(ctx); err != nil {
			return fmt.Errorf("delete TOTP secret: %w", err)
		}
		if _, err := tx.RecoveryCode.Delete().Where(recoverycode.UserID(userID)).Exec(ctx); err != nil {
			return fmt.Errorf("delete recovery codes: %w", err)
		}
		audit.TargetID = userID.String()
		return writeAudit(ctx, tx, audit)
	})
}

func (r *SecondFactorRepository) ReplaceRecoveryCodes(ctx context.Context, userID uuid.UUID, codeHashes []string, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		if err := replaceRecoveryCodes(ctx, tx, userID, codeHashes); err != nil {
			return err
		}
		audit.TargetID = userID.String()
		return writeAudit(ctx, tx, audit)
	})
}

func replaceRecoveryCodes(ctx context.Context, tx *ent.Tx, userID uuid.UUID, codeHashes []string) error {
	if _, err := tx.RecoveryCode.Delete().Where(recoverycode.UserID(userID)).Exec(ctx); err != nil {
		return fmt.Errorf("delete recovery codes: %w", err)
	}
	if len(codeHashes) == 0 {
		return nil
	}
	creates := make([]*ent.RecoveryCodeCreate, 0, len(codeHashes))
	for _, hash := range codeHashes {
		creates = append(creates, tx.RecoveryCode.Create().SetUserID(userID).SetCodeHash(hash))
	}
	if err := tx.RecoveryCode.CreateBulk(creates...).Exec(ctx); err != nil {
		return fmt.Errorf("store recovery codes: %w", err)
	}
	return nil
}

func (r *SecondFactorRepository) UseRecoveryCode(ctx context.Context, userID uuid.UUID, codeHash string) (bool, error) {
	count, err := r.client.RecoveryCode.Update().
		Where(recoverycode.UserID(userID), recoverycode.CodeHash(codeHash), recoverycode.UsedAtIsNil()).
		SetUsedAt(time.Now().UTC()).Save(ctx)
	if err != nil {
		return false, fmt.Errorf("use recovery code: %w", err)
	}
	return count == 1, nil
}

func (r *SecondFactorRepository) RemainingRecoveryCodes(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := r.client.RecoveryCode.Query().Where(recoverycode.UserID(userID), recoverycode.UsedAtIsNil()).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count recovery codes: %w", err)
	}
	return count, nil
}

func (r *SecondFactorRepository) HasSecondFactor(ctx context.Context, userID uuid.UUID) (bool, error) {
	exists, err := r.client.TotpSecret.Query().Where(totpsecret.ID(userID), totpsecret.ConfirmedAtNotNil()).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("check second factor: %w", err)
	}
	return exists, nil
}
