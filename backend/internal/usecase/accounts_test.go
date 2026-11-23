package usecase

import (
	"context"
	"errors"
	"sort"
	"testing"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// fakeAccounts is an in-memory ports.AccountRepository.
type fakeAccounts struct {
	users  map[uuid.UUID]domain.User
	hashes map[uuid.UUID]string
	audits []domain.AuditEntry
}

func newFakeAccounts(users ...domain.User) *fakeAccounts {
	fake := &fakeAccounts{users: map[uuid.UUID]domain.User{}, hashes: map[uuid.UUID]string{}}
	for _, user := range users {
		fake.users[user.ID] = user
	}
	return fake
}

func (f *fakeAccounts) ListUsers(_ context.Context, limit int) ([]domain.User, error) {
	users := make([]domain.User, 0, len(f.users))
	for _, user := range f.users {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	if len(users) > limit {
		users = users[:limit]
	}
	return users, nil
}

func (f *fakeAccounts) GetUser(_ context.Context, id uuid.UUID) (domain.User, error) {
	user, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, nil
}

func (f *fakeAccounts) CountActiveAdmins(context.Context) (int, error) {
	count := 0
	for _, user := range f.users {
		if user.IsActiveAdmin() {
			count++
		}
	}
	return count, nil
}

func (f *fakeAccounts) CreateUser(_ context.Context, input domain.NewUser, passwordHash string, audit domain.AuditEntry) (domain.User, error) {
	for _, user := range f.users {
		if user.Username == input.Username {
			return domain.User{}, domain.ErrUsernameTaken
		}
	}
	user := domain.User{ID: uuid.New(), Username: input.Username, DisplayName: input.DisplayName, Role: input.Role, MustChangePassword: true}
	f.users[user.ID] = user
	f.hashes[user.ID] = passwordHash
	audit.TargetID = user.ID.String()
	f.audits = append(f.audits, audit)
	return user, nil
}

func (f *fakeAccounts) UpdateUser(_ context.Context, id uuid.UUID, change domain.UserChange, audit domain.AuditEntry) (domain.User, error) {
	user, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	user = change.Apply(user)
	f.users[id] = user
	audit.TargetID = id.String()
	f.audits = append(f.audits, audit)
	return user, nil
}

func (f *fakeAccounts) DeleteUser(_ context.Context, id uuid.UUID, audit domain.AuditEntry) error {
	if _, ok := f.users[id]; !ok {
		return domain.ErrUserNotFound
	}
	delete(f.users, id)
	audit.TargetID = id.String()
	f.audits = append(f.audits, audit)
	return nil
}

// revokingSessions records the accounts whose sessions were revoked.
type revokingSessions struct {
	mockSessionRepository
	revoked []uuid.UUID
}

func (r *revokingSessions) RevokeAllForUser(_ context.Context, userID uuid.UUID) error {
	r.revoked = append(r.revoked, userID)
	return nil
}

var testAdmin = domain.User{ID: uuid.New(), Username: "admin", DisplayName: "admin", Role: domain.RoleAdmin}

func TestCreateAccountNormalisesHashesAndAudits(t *testing.T) {
	accounts := newFakeAccounts(testAdmin)
	usecase := NewAccountUsecase(accounts, &revokingSessions{}, &mockHasher{})

	created, err := usecase.Create(context.Background(), testAdmin, domain.NewUser{Username: "alice", DisplayName: " ", Role: domain.RoleViewer}, "initial-password-1", "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if created.DisplayName != "alice" || !created.MustChangePassword {
		t.Fatalf("created = %+v", created)
	}
	if accounts.hashes[created.ID] != "hash:initial-password-1" {
		t.Fatal("the stored password must be the hasher output")
	}
	want := domain.AuditEntry{Actor: "admin", TargetType: "user", TargetID: created.ID.String(), Action: "create", RequestID: "req-1", Outcome: "success"}
	if len(accounts.audits) != 1 || accounts.audits[0] != want {
		t.Fatalf("audits = %+v, want [%+v]", accounts.audits, want)
	}
}

func TestCreateAccountRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name     string
		input    domain.NewUser
		password string
	}{
		{"username", domain.NewUser{Username: "Alice", Role: domain.RoleViewer}, "initial-password-1"},
		{"role", domain.NewUser{Username: "alice", Role: "root"}, "initial-password-1"},
		{"password", domain.NewUser{Username: "alice", Role: domain.RoleViewer}, "short"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			accounts := newFakeAccounts(testAdmin)
			_, err := NewAccountUsecase(accounts, &revokingSessions{}, &mockHasher{}).Create(context.Background(), testAdmin, c.input, c.password, "")
			if !domain.IsValidationError(err) {
				t.Fatalf("Create error = %v, want a validation error", err)
			}
			if len(accounts.users) != 1 || len(accounts.audits) != 0 {
				t.Fatal("an invalid account must not be stored")
			}
		})
	}
}

func TestTheLastActiveAdminIsProtected(t *testing.T) {
	viewer := domain.User{ID: uuid.New(), Username: "viewer", Role: domain.RoleViewer}
	accounts := newFakeAccounts(testAdmin, viewer)
	usecase := NewAccountUsecase(accounts, &revokingSessions{}, &mockHasher{})
	ctx := context.Background()

	demote, disable := domain.RoleOperator, true
	if _, err := usecase.Update(ctx, testAdmin, testAdmin.ID, domain.UserChange{Role: &demote}, ""); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("demote error = %v, want ErrLastAdmin", err)
	}
	if _, err := usecase.Update(ctx, testAdmin, testAdmin.ID, domain.UserChange{Disabled: &disable}, ""); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("disable error = %v, want ErrLastAdmin", err)
	}
	if err := usecase.Delete(ctx, testAdmin, testAdmin.ID, ""); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("delete error = %v, want ErrLastAdmin", err)
	}
	name := "Root"
	if _, err := usecase.Update(ctx, testAdmin, testAdmin.ID, domain.UserChange{DisplayName: &name}, ""); err != nil {
		t.Fatalf("renaming the last admin must be allowed: %v", err)
	}
	if len(accounts.audits) != 1 {
		t.Fatalf("only the rename is audited, got %d entries", len(accounts.audits))
	}
}

func TestAnAdminCanBeDemotedWhenAnotherRemains(t *testing.T) {
	second := domain.User{ID: uuid.New(), Username: "second", Role: domain.RoleAdmin}
	accounts := newFakeAccounts(testAdmin, second)
	viewer := domain.RoleViewer

	updated, err := NewAccountUsecase(accounts, &revokingSessions{}, &mockHasher{}).Update(context.Background(), testAdmin, second.ID, domain.UserChange{Role: &viewer}, "")
	if err != nil || updated.Role != domain.RoleViewer {
		t.Fatalf("Update = (%+v, %v), want a viewer", updated, err)
	}
}

func TestDisablingAnAccountRevokesItsSessions(t *testing.T) {
	viewer := domain.User{ID: uuid.New(), Username: "viewer", Role: domain.RoleViewer}
	accounts := newFakeAccounts(testAdmin, viewer)
	sessions := &revokingSessions{}
	usecase := NewAccountUsecase(accounts, sessions, &mockHasher{})
	ctx := context.Background()

	disable, enable := true, false
	if _, err := usecase.Update(ctx, testAdmin, viewer.ID, domain.UserChange{Disabled: &disable}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := usecase.Update(ctx, testAdmin, viewer.ID, domain.UserChange{Disabled: &enable}, ""); err != nil {
		t.Fatal(err)
	}
	if len(sessions.revoked) != 1 || sessions.revoked[0] != viewer.ID {
		t.Fatalf("revoked = %v, want only the disabling to revoke", sessions.revoked)
	}
}

func TestUpdateAndDeleteReportUnknownAccounts(t *testing.T) {
	usecase := NewAccountUsecase(newFakeAccounts(testAdmin), &revokingSessions{}, &mockHasher{})
	viewer := domain.RoleViewer
	if _, err := usecase.Update(context.Background(), testAdmin, uuid.New(), domain.UserChange{Role: &viewer}, ""); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("Update error = %v, want ErrUserNotFound", err)
	}
	if err := usecase.Delete(context.Background(), testAdmin, uuid.New(), ""); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("Delete error = %v, want ErrUserNotFound", err)
	}
}
