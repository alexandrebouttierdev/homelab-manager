package sqlite

import (
	"context"
	"fmt"
	"time"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/ent/loginfailure"
	"homelab-manager/backend/ent/securitypolicy"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
)

var (
	_ ports.SecurityRepository     = (*SecurityRepository)(nil)
	_ ports.LoginFailureRepository = (*SecurityRepository)(nil)
	_ ports.AuditReader            = (*AuditRepository)(nil)
	_ ports.SessionRepository      = (*AuthRepository)(nil)
	_ ports.UserRepository         = (*AuthRepository)(nil)
)

// SecurityRepository stores the security policy and the consecutive login
// failures of each account.
type SecurityRepository struct {
	client *ent.Client
}

func NewSecurityRepository(client *ent.Client) *SecurityRepository {
	return &SecurityRepository{client: client}
}

// SecurityPolicy returns the stored policy, or the default one when an
// administrator never changed it.
func (r *SecurityRepository) SecurityPolicy(ctx context.Context) (domain.SecurityPolicy, error) {
	record, err := r.client.SecurityPolicy.Query().Where(securitypolicy.SingletonKey(settingsSingletonKey)).Only(ctx)
	if ent.IsNotFound(err) {
		return domain.DefaultSecurityPolicy(), nil
	}
	if err != nil {
		return domain.SecurityPolicy{}, fmt.Errorf("read security policy: %w", err)
	}
	policy, err := domain.NewSecurityPolicy(record.IdleTimeoutMinutes, record.AllowedNetworks)
	if err != nil {
		return domain.SecurityPolicy{}, fmt.Errorf("stored security policy: %w", err)
	}
	policy.RequireAdminSecondFactor = record.RequireAdminSecondFactor
	return policy, nil
}

// SetSecurityPolicy stores the policy and its audit entry atomically.
func (r *SecurityRepository) SetSecurityPolicy(ctx context.Context, policy domain.SecurityPolicy, audit domain.AuditEntry) error {
	networks := policy.Networks()
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		record, err := tx.SecurityPolicy.Query().Where(securitypolicy.SingletonKey(settingsSingletonKey)).Only(ctx)
		switch {
		case ent.IsNotFound(err):
			err = tx.SecurityPolicy.Create().SetSingletonKey(settingsSingletonKey).
				SetIdleTimeoutMinutes(policy.IdleTimeoutMinutes).SetAllowedNetworks(networks).
				SetRequireAdminSecondFactor(policy.RequireAdminSecondFactor).Exec(ctx)
		case err == nil:
			err = tx.SecurityPolicy.UpdateOneID(record.ID).
				SetIdleTimeoutMinutes(policy.IdleTimeoutMinutes).SetAllowedNetworks(networks).
				SetRequireAdminSecondFactor(policy.RequireAdminSecondFactor).Exec(ctx)
		}
		if err != nil {
			return fmt.Errorf("write security policy: %w", err)
		}
		return writeAudit(ctx, tx, audit)
	})
}

// LoginFailures returns zero failures for an account that has none.
func (r *SecurityRepository) LoginFailures(ctx context.Context, userID uuid.UUID) (domain.LoginFailures, error) {
	record, err := r.client.LoginFailure.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return domain.LoginFailures{}, nil
	}
	if err != nil {
		return domain.LoginFailures{}, fmt.Errorf("read login failures: %w", err)
	}
	return domain.LoginFailures{Count: record.ConsecutiveFailures, LockedUntil: record.LockedUntil}, nil
}

func (r *SecurityRepository) SaveLoginFailures(ctx context.Context, userID uuid.UUID, failures domain.LoginFailures, at time.Time) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		exists, err := tx.LoginFailure.Query().Where(loginfailure.ID(userID)).Exist(ctx)
		if err != nil {
			return err
		}
		if exists {
			update := tx.LoginFailure.UpdateOneID(userID).SetConsecutiveFailures(failures.Count).SetLastFailureAt(at.UTC())
			if failures.LockedUntil != nil {
				update.SetLockedUntil(failures.LockedUntil.UTC())
			} else {
				update.ClearLockedUntil()
			}
			return update.Exec(ctx)
		}
		create := tx.LoginFailure.Create().SetID(userID).SetConsecutiveFailures(failures.Count).SetLastFailureAt(at.UTC())
		if failures.LockedUntil != nil {
			create.SetLockedUntil(failures.LockedUntil.UTC())
		}
		return create.Exec(ctx)
	})
}

// ClearLoginFailures forgets the failures of an account; it succeeds when
// there are none.
func (r *SecurityRepository) ClearLoginFailures(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.client.LoginFailure.Delete().Where(loginfailure.ID(userID)).Exec(ctx); err != nil {
		return fmt.Errorf("clear login failures: %w", err)
	}
	return nil
}
