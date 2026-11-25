package usecase

import (
	"context"
	"fmt"
	"sync"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

// SecurityUsecase keeps the security policy in memory, so that every
// request can check it without reading SQLite (SPEC_ADMINISTRATION.md
// § 3.4). Callers of UpdatePolicy must hold domain.PermManageUsers.
type SecurityUsecase struct {
	repository ports.SecurityRepository
	factors    ports.SecondFactorRepository

	mu     sync.RWMutex
	policy domain.SecurityPolicy
}

func NewSecurityUsecase(repository ports.SecurityRepository) *SecurityUsecase {
	return &SecurityUsecase{repository: repository, policy: domain.DefaultSecurityPolicy()}
}

// WithSecondFactors lets the policy require a second factor from
// administrators.
func (u *SecurityUsecase) WithSecondFactors(factors ports.SecondFactorRepository) *SecurityUsecase {
	u.factors = factors
	return u
}

// Load reads the stored policy; it is called once at startup.
func (u *SecurityUsecase) Load(ctx context.Context) error {
	policy, err := u.repository.SecurityPolicy(ctx)
	if err != nil {
		return fmt.Errorf("load security policy: %w", err)
	}
	u.mu.Lock()
	u.policy = policy
	u.mu.Unlock()
	return nil
}

func (u *SecurityUsecase) Policy() domain.SecurityPolicy {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.policy
}

// AllowsAddress reports whether a client address may use the interface.
func (u *SecurityUsecase) AllowsAddress(address string) bool {
	return u.Policy().Allows(address)
}

// UpdatePolicy stores a new policy, refusing one that would exclude the
// address of the administrator making the change.
// Requiring a second factor from administrators needs one on the account of
// the administrator making the change, so that nobody is locked out.
func (u *SecurityUsecase) UpdatePolicy(ctx context.Context, actor domain.User, currentAddress string, idleTimeoutMinutes int, networks []string, requireAdminSecondFactor, allowPasskeyLogin bool, requestID string) (domain.SecurityPolicy, error) {
	policy, err := domain.NewSecurityPolicy(idleTimeoutMinutes, networks)
	if err != nil {
		return domain.SecurityPolicy{}, err
	}
	if !policy.Allows(currentAddress) {
		return domain.SecurityPolicy{}, domain.ErrPolicyExcludesCurrentAddress
	}
	if requireAdminSecondFactor {
		if u.factors == nil {
			return domain.SecurityPolicy{}, domain.ErrSecondFactorUnavailable
		}
		enrolled, err := u.factors.HasSecondFactor(ctx, actor.ID)
		if err != nil {
			return domain.SecurityPolicy{}, err
		}
		if !enrolled {
			return domain.SecurityPolicy{}, domain.ErrSecondFactorNotEnrolled
		}
	}
	policy.RequireAdminSecondFactor = requireAdminSecondFactor
	policy.AllowPasskeyLogin = allowPasskeyLogin
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: "security_policy", TargetID: "local",
		Action: "update", RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.repository.SetSecurityPolicy(ctx, policy, audit); err != nil {
		return domain.SecurityPolicy{}, err
	}
	u.policy = policy
	return policy, nil
}
