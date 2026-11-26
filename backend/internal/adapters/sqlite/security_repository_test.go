package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"monoops/backend/ent/auditlog"
	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

func TestSecurityPolicyIsStoredWithItsAudit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	repository := NewSecurityRepository(database.Client())

	policy, err := repository.SecurityPolicy(ctx)
	if err != nil || policy.IdleTimeoutMinutes != domain.DefaultIdleTimeoutMinutes || len(policy.AllowedNetworks) != 0 {
		t.Fatalf("policy before any change = (%+v, %v), want the default", policy, err)
	}
	wanted, err := domain.NewSecurityPolicy(30, []string{"192.168.1.0/24", "fd00::/8"})
	if err != nil {
		t.Fatal(err)
	}
	audit := domain.AuditEntry{Actor: "admin", TargetType: "security_policy", TargetID: "local", Action: "update", Outcome: domain.AuditSuccess}
	for range 2 { // create, then update the singleton
		if err := repository.SetSecurityPolicy(ctx, wanted, audit); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := repository.SecurityPolicy(ctx)
	if err != nil || stored.IdleTimeoutMinutes != 30 || len(stored.Networks()) != 2 || stored.Networks()[1] != "fd00::/8" {
		t.Fatalf("stored policy = (%+v, %v)", stored, err)
	}
	if count, err := database.Client().AuditLog.Query().Where(auditlog.TargetType("security_policy")).Count(ctx); err != nil || count != 2 {
		t.Fatalf("policy audit rows = (%d, %v), want 2", count, err)
	}
}

func TestLoginFailuresAreCountedAndCascade(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	repository := NewSecurityRepository(database.Client())
	accounts := NewAccountRepository(database.Client())
	audit := domain.AuditEntry{Actor: "admin", TargetType: "user", Action: "create", Outcome: domain.AuditSuccess}
	bob, err := accounts.CreateUser(ctx, domain.NewUser{Username: "bob", DisplayName: "Bob", Role: domain.RoleViewer}, "hash", audit)
	if err != nil {
		t.Fatal(err)
	}

	if failures, err := repository.LoginFailures(ctx, bob.ID); err != nil || failures.Count != 0 || failures.LockedUntil != nil {
		t.Fatalf("failures of a new account = (%+v, %v)", failures, err)
	}
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	until := at.Add(domain.LockoutDuration)
	for _, failures := range []domain.LoginFailures{{Count: 2}, {LockedUntil: &until}} {
		if err := repository.SaveLoginFailures(ctx, bob.ID, failures, at); err != nil {
			t.Fatal(err)
		}
	}
	failures, err := repository.LoginFailures(ctx, bob.ID)
	if err != nil || failures.Count != 0 || failures.LockedUntil == nil || !failures.LockedUntil.Equal(until) {
		t.Fatalf("stored failures = (%+v, %v)", failures, err)
	}
	if err := repository.ClearLoginFailures(ctx, bob.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.ClearLoginFailures(ctx, bob.ID); err != nil {
		t.Fatalf("clearing twice must succeed: %v", err)
	}
	if err := repository.SaveLoginFailures(ctx, bob.ID, domain.LoginFailures{Count: 1}, at); err != nil {
		t.Fatal(err)
	}
	audit.Action = "delete"
	if err := accounts.DeleteUser(ctx, bob.ID, audit); err != nil {
		t.Fatalf("deleting an account with failures: %v", err)
	}
	if count, err := database.Client().LoginFailure.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("failure rows after deletion = (%d, %v), want 0", count, err)
	}
}

func TestSessionsAreListedTouchedAndRevoked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	auth := NewAuthRepository(database.Client())
	accounts := NewAccountRepository(database.Client())
	if _, err := auth.EnsureAdmin(ctx, "admin", "hash"); err != nil {
		t.Fatal(err)
	}
	admin, _, err := auth.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := accounts.CreateUser(ctx, domain.NewUser{Username: "alice", DisplayName: "Alice", Role: domain.RoleViewer}, "hash",
		domain.AuditEntry{Actor: "admin", TargetType: "user", Action: "create", Outcome: domain.AuditSuccess})
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour)
	first, err := auth.Create(ctx, admin.ID, "token-1", "192.0.2.1", "Firefox", "password", expires)
	if err != nil {
		t.Fatal(err)
	}
	if first.AuthMethod != "password" || first.LastSeenAt.IsZero() || first.IPAddress != "192.0.2.1" || first.Username != "admin" {
		t.Fatalf("created session = %+v", first)
	}
	second, err := auth.Create(ctx, admin.ID, "token-2", "192.0.2.2", "Chrome", "password", expires)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Create(ctx, alice.ID, "token-3", "192.0.2.3", "Safari", "password", expires); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Create(ctx, alice.ID, "token-old", "192.0.2.3", "Safari", "password", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	all, err := auth.ListActive(ctx, nil, now, 10)
	if err != nil || len(all) != 3 {
		t.Fatalf("active sessions = (%d, %v), want 3 (the expired one is hidden)", len(all), err)
	}
	mine, err := auth.ListActive(ctx, &alice.ID, now, 10)
	if err != nil || len(mine) != 1 || mine[0].Username != "alice" {
		t.Fatalf("alice sessions = (%+v, %v)", mine, err)
	}

	later := now.Add(2 * time.Minute).Truncate(time.Microsecond)
	if err := auth.Touch(ctx, first.ID, later); err != nil {
		t.Fatal(err)
	}
	found, err := auth.FindActiveByID(ctx, first.ID)
	if err != nil || !found.LastSeenAt.Equal(later) {
		t.Fatalf("touched session = (%+v, %v), want last seen %s", found, err, later)
	}
	if byToken, err := auth.FindActiveByTokenHash(ctx, "token-1"); err != nil || byToken.Username != "admin" || !byToken.LastSeenAt.Equal(later) {
		t.Fatalf("session by token = (%+v, %v)", byToken, err)
	}

	if err := auth.RevokeByID(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := auth.RevokeByID(ctx, second.ID); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("revoking twice = %v, want ErrSessionNotFound", err)
	}
	if _, err := auth.FindActiveByID(ctx, uuid.New()); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("unknown session = %v, want ErrSessionNotFound", err)
	}
	if _, err := auth.Create(ctx, admin.ID, "token-4", "192.0.2.4", "Edge", "password", expires); err != nil {
		t.Fatal(err)
	}
	revoked, err := auth.RevokeAllForUserExcept(ctx, admin.ID, first.ID)
	if err != nil || revoked != 1 {
		t.Fatalf("revoke others = (%d, %v), want 1", revoked, err)
	}
	if _, err := auth.FindActiveByID(ctx, first.ID); err != nil {
		t.Fatalf("the kept session must stay active: %v", err)
	}
}

func TestAuditRecordsAreFiltered(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	audits := NewAuditRepository(database.Client())
	entries := []domain.AuditEntry{
		{Actor: "admin", TargetType: "user", TargetID: "u1", Action: "create", Outcome: domain.AuditSuccess},
		{Actor: "alice", TargetType: "route", TargetID: "GET /api/v1/users", Action: "access", Outcome: domain.AuditDenied},
		{Actor: "admin", TargetType: "host", TargetID: "vm", Action: "reboot", Outcome: domain.AuditRequested},
	}
	for _, entry := range entries {
		if err := audits.RecordAudit(ctx, entry); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // distinct timestamps for the ordering
	}

	all, err := audits.ListAudit(ctx, domain.AuditFilter{Limit: 10})
	if err != nil || len(all) != 3 || all[0].Action != "reboot" || all[2].Action != "create" || all[0].CreatedAt.IsZero() {
		t.Fatalf("all records = (%+v, %v), want newest first", all, err)
	}
	admin, err := audits.ListAudit(ctx, domain.AuditFilter{Actor: "admin", Limit: 10})
	if err != nil || len(admin) != 2 {
		t.Fatalf("admin records = (%d, %v), want 2", len(admin), err)
	}
	denied, err := audits.ListAudit(ctx, domain.AuditFilter{Outcome: domain.AuditDenied, TargetType: "route", Limit: 10})
	if err != nil || len(denied) != 1 || denied[0].Actor != "alice" {
		t.Fatalf("denied records = (%+v, %v)", denied, err)
	}
	limited, err := audits.ListAudit(ctx, domain.AuditFilter{Limit: 1})
	if err != nil || len(limited) != 1 {
		t.Fatalf("limited = (%d, %v), want 1", len(limited), err)
	}
	// A range in another time zone must select the same rows.
	paris := time.FixedZone("CET", 3600)
	from := all[1].CreatedAt.In(paris)
	to := all[0].CreatedAt.In(paris)
	ranged, err := audits.ListAudit(ctx, domain.AuditFilter{From: &from, To: &to, Limit: 10})
	if err != nil || len(ranged) != 1 || ranged[0].Action != "access" {
		t.Fatalf("ranged = (%+v, %v), want only the middle record", ranged, err)
	}
}

func TestMigration004KeepsExistingSessionsActive(t *testing.T) {
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
	for _, name := range []string{"001_initial.sql", "002_app_settings.sql", "003_user_accounts.sql"} {
		if err := applyMigration(ctx, db, name); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	userID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role, display_name, created_at, updated_at) VALUES (?, 'admin', 'hash', 'admin', 'admin', ?, ?)`, userID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, 'old-token', ?, ?)`, uuid.NewString(), userID, now.Add(time.Hour), now); err != nil {
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
	session, err := NewAuthRepository(database.Client()).FindActiveByTokenHash(ctx, "old-token")
	if err != nil || session.AuthMethod != "password" || session.LastSeenAt.IsZero() {
		t.Fatalf("upgraded session = (%+v, %v), want last_seen_at filled and password method", session, err)
	}
}
