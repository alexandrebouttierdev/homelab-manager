package sqlite

import (
	"context"
	"fmt"
	"time"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/ent/loginfailure"
	"homelab-manager/backend/ent/session"
	"homelab-manager/backend/ent/user"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
)

var _ ports.AccountRepository = (*AccountRepository)(nil)

// AccountRepository stores the Manager accounts managed by administrators.
// Every change writes its audit entry in the same transaction; the
// repository fills AuditEntry.TargetID with the account UUID.
type AccountRepository struct {
	client *ent.Client
}

func NewAccountRepository(client *ent.Client) *AccountRepository {
	return &AccountRepository{client: client}
}

// ListUsers returns at most limit accounts ordered by username.
func (r *AccountRepository) ListUsers(ctx context.Context, limit int) ([]domain.User, error) {
	records, err := r.client.User.Query().Order(ent.Asc(user.FieldUsername)).Limit(limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	users := make([]domain.User, 0, len(records))
	for _, record := range records {
		users = append(users, toDomainUser(record))
	}
	return users, nil
}

func (r *AccountRepository) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	record, err := r.client.User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}
	return toDomainUser(record), nil
}

func (r *AccountRepository) CountActiveAdmins(ctx context.Context) (int, error) {
	count, err := r.client.User.Query().Where(user.RoleEQ(user.RoleAdmin), user.DisabledAtIsNil()).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count active administrators: %w", err)
	}
	return count, nil
}

// CreateUser stores an account that must change its password at first login.
func (r *AccountRepository) CreateUser(ctx context.Context, input domain.NewUser, passwordHash string, audit domain.AuditEntry) (domain.User, error) {
	var created *ent.User
	err := withTx(ctx, r.client, func(tx *ent.Tx) error {
		record, err := tx.User.Create().
			SetUsername(input.Username).
			SetDisplayName(input.DisplayName).
			SetRole(user.Role(input.Role)).
			SetPasswordHash(passwordHash).
			SetMustChangePassword(true).
			Save(ctx)
		if ent.IsConstraintError(err) {
			return domain.ErrUsernameTaken
		}
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		created = record
		audit.TargetID = record.ID.String()
		return writeAudit(ctx, tx, audit)
	})
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(created), nil
}

func (r *AccountRepository) UpdateUser(ctx context.Context, id uuid.UUID, change domain.UserChange, audit domain.AuditEntry) (domain.User, error) {
	var updated *ent.User
	err := withTx(ctx, r.client, func(tx *ent.Tx) error {
		update := tx.User.UpdateOneID(id)
		if change.DisplayName != nil {
			update.SetDisplayName(*change.DisplayName)
		}
		if change.Role != nil {
			update.SetRole(user.Role(*change.Role))
		}
		if change.Disabled != nil {
			if *change.Disabled {
				update.SetDisabledAt(time.Now().UTC())
			} else {
				update.ClearDisabledAt()
			}
		}
		record, err := update.Save(ctx)
		if ent.IsNotFound(err) {
			return domain.ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		updated = record
		audit.TargetID = id.String()
		return writeAudit(ctx, tx, audit)
	})
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(updated), nil
}

// ResetPassword stores a new initial password, forces its change at the
// next login and lifts a lockout, with its audit entry, in one transaction.
func (r *AccountRepository) ResetPassword(ctx context.Context, id uuid.UUID, passwordHash string, audit domain.AuditEntry) (domain.User, error) {
	var updated *ent.User
	err := withTx(ctx, r.client, func(tx *ent.Tx) error {
		record, err := tx.User.UpdateOneID(id).SetPasswordHash(passwordHash).SetMustChangePassword(true).Save(ctx)
		if ent.IsNotFound(err) {
			return domain.ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("reset password: %w", err)
		}
		if _, err := tx.LoginFailure.Delete().Where(loginfailure.ID(id)).Exec(ctx); err != nil {
			return fmt.Errorf("clear login failures: %w", err)
		}
		updated = record
		audit.TargetID = id.String()
		return writeAudit(ctx, tx, audit)
	})
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(updated), nil
}

// DeleteUser removes the account and its sessions; audit rows keep the
// actor username as text and therefore survive the account.
func (r *AccountRepository) DeleteUser(ctx context.Context, id uuid.UUID, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		if _, err := tx.Session.Delete().Where(session.UserID(id)).Exec(ctx); err != nil {
			return fmt.Errorf("delete sessions: %w", err)
		}
		err := tx.User.DeleteOneID(id).Exec(ctx)
		if ent.IsNotFound(err) {
			return domain.ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("delete user: %w", err)
		}
		audit.TargetID = id.String()
		return writeAudit(ctx, tx, audit)
	})
}

func toDomainUser(record *ent.User) domain.User {
	return domain.User{
		ID:                 record.ID,
		Username:           record.Username,
		DisplayName:        record.DisplayName,
		Role:               domain.Role(record.Role),
		Disabled:           record.DisabledAt != nil,
		MustChangePassword: record.MustChangePassword,
		LastLoginAt:        record.LastLoginAt,
		LastLoginIP:        record.LastLoginIP,
		CreatedAt:          record.CreatedAt,
	}
}
