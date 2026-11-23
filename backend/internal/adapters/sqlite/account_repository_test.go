package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"homelab-manager/backend/ent/auditlog"
	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

func TestAccountRepositoryLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	auth := NewAuthRepository(database.Client())
	accounts := NewAccountRepository(database.Client())

	if _, err := auth.EnsureAdmin(ctx, "admin", "admin-hash"); err != nil {
		t.Fatal(err)
	}
	admin, _, err := auth.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if admin.Role != domain.RoleAdmin || admin.DisplayName != "admin" || admin.MustChangePassword || admin.Disabled {
		t.Fatalf("bootstrap administrator = %+v", admin)
	}

	audit := domain.AuditEntry{Actor: "admin", TargetType: "user", Action: "create", RequestID: "req-1", Outcome: "success"}
	alice, err := accounts.CreateUser(ctx, domain.NewUser{Username: "alice", DisplayName: "Alice", Role: domain.RoleViewer}, "alice-hash", audit)
	if err != nil {
		t.Fatal(err)
	}
	if !alice.MustChangePassword || alice.Role != domain.RoleViewer || alice.DisplayName != "Alice" {
		t.Fatalf("created account = %+v", alice)
	}
	if _, err := accounts.CreateUser(ctx, domain.NewUser{Username: "alice", DisplayName: "Other", Role: domain.RoleViewer}, "hash", audit); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatalf("duplicate username error = %v, want ErrUsernameTaken", err)
	}
	if count, err := accounts.CountActiveAdmins(ctx); err != nil || count != 1 {
		t.Fatalf("CountActiveAdmins = (%d, %v), want 1", count, err)
	}

	if err := auth.UpdatePasswordHash(ctx, alice.ID, "alice-own-hash"); err != nil {
		t.Fatal(err)
	}
	if reloaded, err := accounts.GetUser(ctx, alice.ID); err != nil || reloaded.MustChangePassword {
		t.Fatalf("after a password change = (%+v, %v), want MustChangePassword false", reloaded, err)
	}

	loginAt := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	if err := auth.RecordLogin(ctx, alice.ID, "192.0.2.10", loginAt); err != nil {
		t.Fatal(err)
	}
	operator, disabled := domain.RoleOperator, true
	audit.Action = "update"
	updated, err := accounts.UpdateUser(ctx, alice.ID, domain.UserChange{Role: &operator, Disabled: &disabled}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != domain.RoleOperator || !updated.Disabled || updated.LastLoginIP != "192.0.2.10" || updated.LastLoginAt == nil || !updated.LastLoginAt.Equal(loginAt) {
		t.Fatalf("updated account = %+v", updated)
	}
	if _, err := accounts.UpdateUser(ctx, uuid.New(), domain.UserChange{Role: &operator}, audit); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("update of an unknown account error = %v, want ErrUserNotFound", err)
	}

	if _, err := auth.Create(ctx, alice.ID, "alice-token", "127.0.0.1", "test", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	audit.Action = "delete"
	if err := accounts.DeleteUser(ctx, alice.ID, audit); err != nil {
		t.Fatalf("delete an account with sessions: %v", err)
	}
	if _, err := accounts.GetUser(ctx, alice.ID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("deleted account error = %v, want ErrUserNotFound", err)
	}
	if err := accounts.DeleteUser(ctx, alice.ID, audit); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("second delete error = %v, want ErrUserNotFound", err)
	}

	users, err := accounts.ListUsers(ctx, 10)
	if err != nil || len(users) != 1 || users[0].Username != "admin" {
		t.Fatalf("ListUsers = (%+v, %v), want only admin", users, err)
	}
	// create, update and delete committed; the refused calls rolled back.
	audits, err := database.Client().AuditLog.Query().Where(auditlog.TargetType("user"), auditlog.TargetID(alice.ID.String())).Count(ctx)
	if err != nil || audits != 3 {
		t.Fatalf("audit rows for alice = (%d, %v), want 3", audits, err)
	}
}

func TestMigration003PromotesExistingAccountsToAdministrators(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	if err := prepareFile(path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_initial.sql", "002_app_settings.sql"} {
		if err := applyMigration(ctx, db, name); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (?, 'admin', 'hash', ?, ?)`, uuid.NewString(), now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	user, _, err := NewAuthRepository(database.Client()).FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != domain.RoleAdmin || user.DisplayName != "admin" || user.Disabled || user.MustChangePassword {
		t.Fatalf("upgraded account = %+v, want an active admin", user)
	}
}

func TestResetPasswordForcesAChangeAndUnlocks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	auth := NewAuthRepository(database.Client())
	accounts := NewAccountRepository(database.Client())
	security := NewSecurityRepository(database.Client())
	audit := domain.AuditEntry{Actor: "admin", TargetType: "user", Action: "create", Outcome: domain.AuditSuccess}
	alice, err := accounts.CreateUser(ctx, domain.NewUser{Username: "alice", DisplayName: "Alice", Role: domain.RoleViewer}, "initial-hash", audit)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.UpdatePasswordHash(ctx, alice.ID, "own-hash"); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(domain.LockoutDuration)
	if err := security.SaveLoginFailures(ctx, alice.ID, domain.LoginFailures{LockedUntil: &until}, time.Now()); err != nil {
		t.Fatal(err)
	}

	audit.Action = "reset_password"
	reset, err := accounts.ResetPassword(ctx, alice.ID, "reset-hash", audit)
	if err != nil || !reset.MustChangePassword {
		t.Fatalf("ResetPassword = (%+v, %v), want a forced change", reset, err)
	}
	if _, hash, err := auth.FindByUsername(ctx, "alice"); err != nil || hash != "reset-hash" {
		t.Fatalf("stored hash = (%q, %v)", hash, err)
	}
	if failures, err := security.LoginFailures(ctx, alice.ID); err != nil || failures.LockedUntil != nil {
		t.Fatalf("failures after a reset = (%+v, %v), want unlocked", failures, err)
	}
	if count, err := database.Client().AuditLog.Query().Where(auditlog.Action("reset_password"), auditlog.TargetID(alice.ID.String())).Count(ctx); err != nil || count != 1 {
		t.Fatalf("reset audit rows = (%d, %v), want 1", count, err)
	}
	if _, err := accounts.ResetPassword(ctx, uuid.New(), "hash", audit); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("reset of an unknown account = %v, want ErrUserNotFound", err)
	}

	if err := auth.UpdatePasswordHash(ctx, alice.ID, "own-hash-2"); err != nil {
		t.Fatal(err)
	}
	if err := auth.RequirePasswordChange(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if user, err := accounts.GetUser(ctx, alice.ID); err != nil || !user.MustChangePassword {
		t.Fatalf("RequirePasswordChange = (%+v, %v)", user, err)
	}
}
