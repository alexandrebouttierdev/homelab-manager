package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"

	"github.com/google/uuid"
)

// maxAccounts bounds the account list; one server has a handful of accounts.
const maxAccounts = 500

const auditTargetUser = "user"

// AccountUsecase manages the Manager accounts (SPEC_ADMINISTRATION.md § 3.1).
// Callers must hold domain.PermManageUsers; the HTTP layer enforces it.
type AccountUsecase struct {
	accounts ports.AccountRepository
	sessions ports.SessionRepository
	hasher   ports.PasswordHasher
	// audit, when set, traces the refusals that protect the last
	// administrator (SPEC_ADMINISTRATION.md § 2.2).
	audit ports.AuditRecorder

	// mu serialises account changes: the last-administrator check and the
	// write that follows must not interleave with another change. The
	// Manager is the only writer of the accounts.
	mu sync.Mutex
}

func NewAccountUsecase(accounts ports.AccountRepository, sessions ports.SessionRepository, hasher ports.PasswordHasher) *AccountUsecase {
	return &AccountUsecase{accounts: accounts, sessions: sessions, hasher: hasher}
}

// WithAudit traces the refused changes with outcome "denied".
func (u *AccountUsecase) WithAudit(audit ports.AuditRecorder) *AccountUsecase {
	u.audit = audit
	return u
}

func (u *AccountUsecase) List(ctx context.Context) ([]domain.User, error) {
	return u.accounts.ListUsers(ctx, maxAccounts)
}

// Create stores an account whose initial password must be changed at
// first login.
func (u *AccountUsecase) Create(ctx context.Context, actor domain.User, input domain.NewUser, password, requestID string) (domain.User, error) {
	normalized, err := input.Normalize()
	if err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidateNewPassword(password); err != nil {
		return domain.User{}, err
	}
	// Hashing is slow on purpose: do it before taking the lock.
	hash, err := u.hasher.Hash(password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.accounts.CreateUser(ctx, normalized, hash, auditEntry(actor, "create", requestID))
}

// Update changes an account. Disabling it revokes its sessions; the last
// active administrator can be neither demoted nor disabled.
func (u *AccountUsecase) Update(ctx context.Context, actor domain.User, id uuid.UUID, change domain.UserChange, requestID string) (domain.User, error) {
	change, err := change.Normalize()
	if err != nil {
		return domain.User{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	target, err := u.accounts.GetUser(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	if target.IsActiveAdmin() && !change.Apply(target).IsActiveAdmin() {
		if err := u.requireAnotherAdmin(ctx, actor, id, "update", requestID); err != nil {
			return domain.User{}, err
		}
	}
	updated, err := u.accounts.UpdateUser(ctx, id, change, auditEntry(actor, "update", requestID))
	if err != nil {
		return domain.User{}, err
	}
	if updated.Disabled && !target.Disabled {
		if err := u.sessions.RevokeAllForUser(ctx, id); err != nil {
			return domain.User{}, fmt.Errorf("revoke sessions: %w", err)
		}
	}
	return updated, nil
}

// ResetPassword gives another account a new initial password to change at
// its next login, revokes its sessions and lifts its lockout. Administrators
// change their own password through the normal password change.
func (u *AccountUsecase) ResetPassword(ctx context.Context, actor domain.User, id uuid.UUID, password, requestID string) (domain.User, error) {
	if err := domain.ValidateNewPassword(password); err != nil {
		return domain.User{}, err
	}
	if id == actor.ID {
		return domain.User{}, domain.ErrOwnPasswordReset
	}
	hash, err := u.hasher.Hash(password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	reset, err := u.accounts.ResetPassword(ctx, id, hash, auditEntry(actor, "reset_password", requestID))
	if err != nil {
		return domain.User{}, err
	}
	if err := u.sessions.RevokeAllForUser(ctx, id); err != nil {
		return domain.User{}, fmt.Errorf("revoke sessions: %w", err)
	}
	return reset, nil
}

// Delete removes an account and its sessions, keeping its audit entries.
func (u *AccountUsecase) Delete(ctx context.Context, actor domain.User, id uuid.UUID, requestID string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	target, err := u.accounts.GetUser(ctx, id)
	if err != nil {
		return err
	}
	if target.IsActiveAdmin() {
		if err := u.requireAnotherAdmin(ctx, actor, id, "delete", requestID); err != nil {
			return err
		}
	}
	return u.accounts.DeleteUser(ctx, id, auditEntry(actor, "delete", requestID))
}

// requireAnotherAdmin refuses a change that would leave no active
// administrator, and traces the refusal.
func (u *AccountUsecase) requireAnotherAdmin(ctx context.Context, actor domain.User, id uuid.UUID, action, requestID string) error {
	count, err := u.accounts.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if count > 1 {
		return nil
	}
	if u.audit != nil {
		entry := auditEntry(actor, action, requestID)
		entry.TargetID, entry.Outcome = id.String(), domain.AuditDenied
		if err := u.audit.RecordAudit(ctx, entry); err != nil {
			return errors.Join(domain.ErrLastAdmin, fmt.Errorf("record audit: %w", err))
		}
	}
	return domain.ErrLastAdmin
}

func auditEntry(actor domain.User, action, requestID string) domain.AuditEntry {
	return domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetUser, Action: action,
		RequestID: requestID, Outcome: "success",
	}
}
