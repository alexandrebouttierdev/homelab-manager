package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

type mockUserRepository struct {
	user    domain.User
	hash    string
	missing bool
	loginIP string
	loginAt time.Time
}

func (m *mockUserRepository) FindByUsername(context.Context, string) (domain.User, string, error) {
	if m.missing {
		return domain.User{}, "", domain.ErrInvalidCredentials
	}
	return m.user, m.hash, nil
}
func (m *mockUserRepository) FindByID(context.Context, uuid.UUID) (domain.User, error) {
	return m.user, nil
}
func (m *mockUserRepository) UpdatePasswordHash(context.Context, uuid.UUID, string) error {
	return nil
}
func (m *mockUserRepository) EnsureAdmin(context.Context, string, string) (bool, error) {
	return false, nil
}
func (m *mockUserRepository) RequirePasswordChange(context.Context, uuid.UUID) error {
	return nil
}
func (m *mockUserRepository) RecordLogin(_ context.Context, _ uuid.UUID, ip string, at time.Time) error {
	m.loginIP, m.loginAt = ip, at
	return nil
}

// mockSessionRepository finds active when it is set.
type mockSessionRepository struct {
	active     *domain.Session
	authMethod string
}

func (m *mockSessionRepository) Create(_ context.Context, userID uuid.UUID, _, _, _, authMethod string, expiresAt time.Time) (domain.Session, error) {
	m.authMethod = authMethod
	return domain.Session{ID: uuid.New(), UserID: userID, ExpiresAt: expiresAt}, nil
}
func (m *mockSessionRepository) FindActiveByTokenHash(context.Context, string) (domain.Session, error) {
	if m.active != nil {
		return *m.active, nil
	}
	return domain.Session{}, domain.ErrSessionNotFound
}
func (m *mockSessionRepository) RevokeByTokenHash(context.Context, string) error {
	return nil
}
func (m *mockSessionRepository) RevokeAllForUser(context.Context, uuid.UUID) error { return nil }
func (m *mockSessionRepository) FindActiveByID(context.Context, uuid.UUID) (domain.Session, error) {
	return domain.Session{}, domain.ErrSessionNotFound
}
func (m *mockSessionRepository) ListActive(context.Context, *uuid.UUID, time.Time, int) ([]domain.Session, error) {
	return nil, nil
}
func (m *mockSessionRepository) Touch(context.Context, uuid.UUID, time.Time) error { return nil }
func (m *mockSessionRepository) RevokeByID(context.Context, uuid.UUID) error       { return nil }
func (m *mockSessionRepository) RevokeAllForUserExcept(context.Context, uuid.UUID, uuid.UUID) (int, error) {
	return 0, nil
}

type mockHasher struct {
	valid    bool
	verified []string
}

func (m *mockHasher) Hash(password string) (string, error) {
	return "hash:" + password, nil
}
func (m *mockHasher) Verify(password, encodedHash string) (bool, error) {
	m.verified = append(m.verified, encodedHash)
	return m.valid && password != "", nil
}

func TestAuthUsecaseLogin(t *testing.T) {
	userID := uuid.New()
	users := &mockUserRepository{
		user: domain.User{ID: userID, Username: "admin"},
		hash: "stored-hash",
	}
	usecase := NewAuthUsecase(users, &mockSessionRepository{}, &mockHasher{valid: true})

	result, err := usecase.Login(context.Background(), "admin", "a-valid-password", "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.Challenge != "" {
		t.Fatal("expected a session token and no second factor challenge")
	}
	if result.User.ID != userID {
		t.Fatalf("unexpected user ID: %s", result.User.ID)
	}
	if !result.ExpiresAt.After(time.Now()) {
		t.Fatal("session should expire in the future")
	}
}

func TestAuthUsecaseRejectsInvalidPassword(t *testing.T) {
	users := &mockUserRepository{
		user: domain.User{ID: uuid.New(), Username: "admin"},
		hash: "stored-hash",
	}
	usecase := NewAuthUsecase(users, &mockSessionRepository{}, &mockHasher{valid: false})

	_, err := usecase.Login(context.Background(), "admin", "wrong", "", "")
	if err != domain.ErrInvalidCredentials {
		t.Fatalf("got error %v, want %v", err, domain.ErrInvalidCredentials)
	}
}

func TestAuthUsecaseUnknownUserStillVerifiesAHash(t *testing.T) {
	hasher := &mockHasher{valid: true}
	usecase := NewAuthUsecase(&mockUserRepository{missing: true}, &mockSessionRepository{}, hasher)

	for range 2 {
		if _, err := usecase.Login(context.Background(), "nobody", "password", "", ""); err != domain.ErrInvalidCredentials {
			t.Fatalf("got error %v, want %v", err, domain.ErrInvalidCredentials)
		}
	}
	if len(hasher.verified) != 2 || hasher.verified[0] == "" || hasher.verified[0] != hasher.verified[1] {
		t.Fatalf("verified hashes = %q, want the same dummy hash verified on each attempt", hasher.verified)
	}
}

func TestLoginRejectsADisabledAccount(t *testing.T) {
	users := &mockUserRepository{
		user: domain.User{ID: uuid.New(), Username: "bob", Role: domain.RoleViewer, Disabled: true},
		hash: "stored-hash",
	}
	hasher := &mockHasher{valid: true}
	auth := NewAuthUsecase(users, &mockSessionRepository{}, hasher)

	_, err := auth.Login(context.Background(), "bob", "a-valid-password", "192.0.2.1", "test")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Login error = %v, want ErrInvalidCredentials", err)
	}
	if len(hasher.verified) != 1 {
		t.Fatal("the password must still be verified so that the response time does not reveal the state")
	}
	if users.loginIP != "" {
		t.Fatal("a refused login must not be recorded")
	}
}

func TestLoginRecordsTheLastLogin(t *testing.T) {
	users := &mockUserRepository{
		user: domain.User{ID: uuid.New(), Username: "alice", Role: domain.RoleViewer},
		hash: "stored-hash",
	}
	auth := NewAuthUsecase(users, &mockSessionRepository{}, &mockHasher{valid: true})
	fixed := time.Date(2026, 11, 23, 9, 30, 0, 0, time.UTC)
	auth.now = func() time.Time { return fixed }

	if _, err := auth.Login(context.Background(), "alice", "a-valid-password", "192.0.2.7", "test"); err != nil {
		t.Fatal(err)
	}
	if users.loginIP != "192.0.2.7" || !users.loginAt.Equal(fixed) {
		t.Fatalf("recorded login = (%q, %s), want (192.0.2.7, %s)", users.loginIP, users.loginAt, fixed)
	}
}

func TestSessionRejectsADisabledAccount(t *testing.T) {
	userID := uuid.New()
	sessions := &mockSessionRepository{active: &domain.Session{ID: uuid.New(), UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}}
	users := &mockUserRepository{user: domain.User{ID: userID, Username: "bob", Role: domain.RoleAdmin, Disabled: true}}

	_, _, err := NewAuthUsecase(users, sessions, &mockHasher{}).Session(context.Background(), "token")
	if !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("Session error = %v, want ErrSessionNotFound", err)
	}
}

// bootstrapUsers is a ports.UserRepository for the administrator bootstrap.
type bootstrapUsers struct {
	mockUserRepository
	exists  bool
	created bool
	forced  bool
}

func (b *bootstrapUsers) EnsureAdmin(_ context.Context, _ string, passwordHash string) (bool, error) {
	if b.exists {
		return false, nil
	}
	b.exists, b.created, b.hash = true, true, passwordHash
	return true, nil
}
func (b *bootstrapUsers) RequirePasswordChange(context.Context, uuid.UUID) error {
	b.forced = true
	return nil
}

func TestEnsureAdministratorForcesTheDefaultPasswordToChange(t *testing.T) {
	cases := []struct {
		name       string
		existing   string // stored hash of an existing account, "" for a fresh database
		isDefault  bool
		wantForced bool
	}{
		{"fresh database, default password", "", true, true},
		{"fresh database, configured password", "", false, false},
		{"existing account still on the default", "hash:admin", true, true},
		{"existing account with its own password", "hash:own-password", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			users := &bootstrapUsers{mockUserRepository: mockUserRepository{user: domain.User{ID: uuid.New(), Username: "admin", Role: domain.RoleAdmin}}}
			if c.existing != "" {
				users.exists, users.hash = true, c.existing
			}
			password := "admin"
			if !c.isDefault {
				password = "configured-password-1"
			}
			created, forced, err := NewAuthUsecase(users, &mockSessionRepository{}, &checkingHasher{}).EnsureAdministrator(context.Background(), "admin", password, c.isDefault)
			if err != nil {
				t.Fatal(err)
			}
			if created != (c.existing == "") || forced != c.wantForced || users.forced != c.wantForced {
				t.Fatalf("EnsureAdministrator = (created %v, forced %v), stored forced %v", created, forced, users.forced)
			}
		})
	}
}
