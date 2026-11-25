package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"homelab-manager/backend/ent/auditlog"
	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

func TestSecondFactorLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	accounts := NewAccountRepository(database.Client())
	factors := NewSecondFactorRepository(database.Client())
	audit := domain.AuditEntry{Actor: "admin", TargetType: "user", Action: "create", Outcome: domain.AuditSuccess}
	alice, err := accounts.CreateUser(ctx, domain.NewUser{Username: "alice", DisplayName: "Alice", Role: domain.RoleViewer}, "hash", audit)
	if err != nil {
		t.Fatal(err)
	}

	if _, found, err := factors.TOTP(ctx, alice.ID); err != nil || found {
		t.Fatalf("TOTP before enrollment = (%v, %v)", found, err)
	}
	if has, err := factors.HasSecondFactor(ctx, alice.ID); err != nil || has {
		t.Fatalf("HasSecondFactor before enrollment = (%v, %v)", has, err)
	}
	for _, sealed := range []string{"v1.first", "v1.second"} { // a new start replaces the pending one
		if err := factors.SavePendingTOTP(ctx, alice.ID, sealed); err != nil {
			t.Fatal(err)
		}
	}
	pending, found, err := factors.TOTP(ctx, alice.ID)
	if err != nil || !found || pending.Confirmed || pending.Sealed != "v1.second" {
		t.Fatalf("pending = (%+v, %v, %v)", pending, found, err)
	}
	if has, _ := factors.HasSecondFactor(ctx, alice.ID); has {
		t.Fatal("a pending enrollment is not a second factor")
	}

	hashes := []string{domain.RecoveryCodeHash("aaaa-bbbb-cccc"), domain.RecoveryCodeHash("dddd-eeee-ffff")}
	audit.Action = "second_factor_enable"
	if err := factors.ConfirmTOTP(ctx, alice.ID, 100, hashes, audit); err != nil {
		t.Fatal(err)
	}
	confirmed, _, err := factors.TOTP(ctx, alice.ID)
	if err != nil || !confirmed.Confirmed || confirmed.LastUsedStep != 100 {
		t.Fatalf("confirmed = (%+v, %v)", confirmed, err)
	}
	if has, _ := factors.HasSecondFactor(ctx, alice.ID); !has {
		t.Fatal("a confirmed TOTP is a second factor")
	}
	if err := factors.SavePendingTOTP(ctx, alice.ID, "v1.third"); !errors.Is(err, domain.ErrSecondFactorAlreadyEnabled) {
		t.Fatalf("restarting an enabled TOTP = %v, want ErrSecondFactorAlreadyEnabled", err)
	}

	// Anti-replay: a step is accepted once, and never an older one.
	for _, c := range []struct {
		step int64
		want bool
	}{{100, false}, {101, true}, {101, false}, {99, false}, {103, true}} {
		used, err := factors.UseTOTPStep(ctx, alice.ID, c.step)
		if err != nil || used != c.want {
			t.Fatalf("UseTOTPStep(%d) = (%v, %v), want %v", c.step, used, err, c.want)
		}
	}

	if remaining, err := factors.RemainingRecoveryCodes(ctx, alice.ID); err != nil || remaining != 2 {
		t.Fatalf("remaining = (%d, %v), want 2", remaining, err)
	}
	if used, err := factors.UseRecoveryCode(ctx, alice.ID, hashes[0]); err != nil || !used {
		t.Fatalf("first use = (%v, %v)", used, err)
	}
	if used, err := factors.UseRecoveryCode(ctx, alice.ID, hashes[0]); err != nil || used {
		t.Fatalf("second use = (%v, %v), want refused", used, err)
	}
	if remaining, _ := factors.RemainingRecoveryCodes(ctx, alice.ID); remaining != 1 {
		t.Fatalf("remaining after a use = %d, want 1", remaining)
	}
	audit.Action = "recovery_codes_regenerate"
	if err := factors.ReplaceRecoveryCodes(ctx, alice.ID, []string{domain.RecoveryCodeHash("gggg-hhhh-jjjj")}, audit); err != nil {
		t.Fatal(err)
	}
	if used, _ := factors.UseRecoveryCode(ctx, alice.ID, hashes[1]); used {
		t.Fatal("replaced codes no longer work")
	}

	audit.Action = "second_factor_disable"
	if err := factors.DisableSecondFactor(ctx, alice.ID, audit); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := factors.TOTP(ctx, alice.ID); found {
		t.Fatal("disabling removes the TOTP secret")
	}
	if remaining, _ := factors.RemainingRecoveryCodes(ctx, alice.ID); remaining != 0 {
		t.Fatal("disabling removes the recovery codes")
	}
	if count, err := database.Client().AuditLog.Query().Where(auditlog.TargetID(alice.ID.String())).Count(ctx); err != nil || count != 4 {
		t.Fatalf("audit rows = (%d, %v), want create, enable, regenerate, disable", count, err)
	}

	if err := factors.SavePendingTOTP(ctx, alice.ID, "v1.again"); err != nil {
		t.Fatal(err)
	}
	if err := factors.ReplaceRecoveryCodes(ctx, alice.ID, hashes, audit); err != nil {
		t.Fatal(err)
	}
	audit.Action = "delete"
	if err := accounts.DeleteUser(ctx, alice.ID, audit); err != nil {
		t.Fatalf("deleting an account with a second factor: %v", err)
	}
	if count, _ := database.Client().RecoveryCode.Query().Count(ctx); count != 0 {
		t.Fatal("recovery codes are deleted with the account")
	}
	if count, _ := database.Client().TotpSecret.Query().Count(ctx); count != 0 {
		t.Fatal("the TOTP secret is deleted with the account")
	}
}

func TestSecurityPolicyStoresTheSecondFactorRequirement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	repository := NewSecurityRepository(database.Client())
	policy := domain.DefaultSecurityPolicy()
	policy.RequireAdminSecondFactor = true
	if err := repository.SetSecurityPolicy(ctx, policy, domain.AuditEntry{Actor: "admin", TargetType: "security_policy", TargetID: "local", Action: "update", Outcome: domain.AuditSuccess}); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.SecurityPolicy(ctx)
	if err != nil || !stored.RequireAdminSecondFactor {
		t.Fatalf("stored = (%+v, %v)", stored, err)
	}
}

func TestWebAuthnCredentials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	accounts := NewAccountRepository(database.Client())
	factors := NewSecondFactorRepository(database.Client())
	audit := domain.AuditEntry{Actor: "admin", TargetType: "user", Action: "create", Outcome: domain.AuditSuccess}
	alice, err := accounts.CreateUser(ctx, domain.NewUser{Username: "alice", DisplayName: "Alice", Role: domain.RoleViewer}, "hash", audit)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := accounts.CreateUser(ctx, domain.NewUser{Username: "bob", DisplayName: "Bob", Role: domain.RoleViewer}, "hash", audit)
	if err != nil {
		t.Fatal(err)
	}
	key := func(owner uuid.UUID, id string) domain.WebAuthnCredential {
		return domain.WebAuthnCredential{
			ID: uuid.New(), UserID: owner, Name: "key " + id, CredentialID: []byte(id), Data: []byte(`{"id":"` + id + `"}`),
			CreatedAt: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
		}
	}

	audit.Action, audit.TargetID = "webauthn_add", alice.ID.String()
	first := key(alice.ID, "k0")
	codes := []string{domain.RecoveryCodeHash("aaaa-bbbb-cccc")}
	if err := factors.AddWebAuthnCredential(ctx, first, codes, audit); err != nil {
		t.Fatal(err)
	}
	if has, _ := factors.HasSecondFactor(ctx, alice.ID); !has {
		t.Fatal("a key is a second factor")
	}
	if remaining, _ := factors.RemainingRecoveryCodes(ctx, alice.ID); remaining != 1 {
		t.Fatalf("recovery codes = %d, want those stored with the first key", remaining)
	}
	if err := factors.AddWebAuthnCredential(ctx, key(bob.ID, "k0"), nil, audit); !errors.Is(err, domain.ErrWebAuthnKeyExists) {
		t.Fatalf("same key on another account = %v, want ErrWebAuthnKeyExists", err)
	}
	for i := 1; i < domain.MaxWebAuthnCredentials; i++ {
		if err := factors.AddWebAuthnCredential(ctx, key(alice.ID, fmt.Sprintf("k%d", i)), nil, audit); err != nil {
			t.Fatal(err)
		}
	}
	if err := factors.AddWebAuthnCredential(ctx, key(alice.ID, "extra"), nil, audit); !errors.Is(err, domain.ErrTooManyWebAuthnKeys) {
		t.Fatalf("eleventh key = %v", err)
	}
	if remaining, _ := factors.RemainingRecoveryCodes(ctx, alice.ID); remaining != 1 {
		t.Fatal("adding a key with nil hashes keeps the recovery codes")
	}

	used := time.Date(2026, 11, 25, 11, 0, 0, 0, time.UTC)
	if err := factors.UseWebAuthnCredential(ctx, first.ID, []byte(`{"count":1}`), used); err != nil {
		t.Fatal(err)
	}
	keys, err := factors.WebAuthnCredentials(ctx, alice.ID)
	if err != nil || len(keys) != domain.MaxWebAuthnCredentials {
		t.Fatalf("keys = (%d, %v)", len(keys), err)
	}
	var stored domain.WebAuthnCredential
	for _, candidate := range keys {
		if candidate.ID == first.ID {
			stored = candidate
		}
	}
	if stored.ID != first.ID || string(stored.Data) != `{"count":1}` || stored.LastUsedAt == nil || !stored.LastUsedAt.Equal(used) || string(stored.CredentialID) != "k0" {
		t.Fatalf("first key = %+v", stored)
	}

	audit.Action = "webauthn_remove"
	if err := factors.DeleteWebAuthnCredential(ctx, bob.ID, first.ID, audit); !errors.Is(err, domain.ErrWebAuthnKeyNotFound) {
		t.Fatalf("delete the key of another account = %v", err)
	}
	for _, stored := range keys {
		if err := factors.DeleteWebAuthnCredential(ctx, alice.ID, stored.ID, audit); err != nil {
			t.Fatal(err)
		}
	}
	if has, _ := factors.HasSecondFactor(ctx, alice.ID); has {
		t.Fatal("no key left, no second factor")
	}
	if remaining, _ := factors.RemainingRecoveryCodes(ctx, alice.ID); remaining != 0 {
		t.Fatal("the recovery codes go with the last second factor")
	}

	// Disabling the second factor removes the keys.
	if err := factors.AddWebAuthnCredential(ctx, key(alice.ID, "again"), codes, audit); err != nil {
		t.Fatal(err)
	}
	if err := factors.DisableSecondFactor(ctx, alice.ID, audit); err != nil {
		t.Fatal(err)
	}
	if keys, _ := factors.WebAuthnCredentials(ctx, alice.ID); len(keys) != 0 {
		t.Fatalf("keys after disable = %d", len(keys))
	}
}
