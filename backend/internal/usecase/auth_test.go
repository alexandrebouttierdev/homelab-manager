package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

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
func (m *mockUserRepository) RecordLogin(_ context.Context, _ uuid.UUID, ip string, at time.Time) error {
	m.loginIP, m.loginAt = ip, at
	return nil
}

// mockSessionRepository finds active when it is set.
type mockSessionRepository struct {
	active *domain.Session
}

func (m *mockSessionRepository) Create(_ context.Context, userID uuid.UUID, _ string, _ string, _ string, expiresAt time.Time) (domain.Session, error) {
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

	token, user, expiresAt, err := usecase.Login(context.Background(), "admin", "a-valid-password", "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("expected a session token")
	}
	if user.ID != userID {
		t.Fatalf("unexpected user ID: %s", user.ID)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("session should expire in the future")
	}
}

func TestAuthUsecaseRejectsInvalidPassword(t *testing.T) {
	users := &mockUserRepository{
		user: domain.User{ID: uuid.New(), Username: "admin"},
		hash: "stored-hash",
	}
	usecase := NewAuthUsecase(users, &mockSessionRepository{}, &mockHasher{valid: false})

	_, _, _, err := usecase.Login(context.Background(), "admin", "wrong", "", "")
	if err != domain.ErrInvalidCredentials {
		t.Fatalf("got error %v, want %v", err, domain.ErrInvalidCredentials)
	}
}

func TestAuthUsecaseUnknownUserStillVerifiesAHash(t *testing.T) {
	hasher := &mockHasher{valid: true}
	usecase := NewAuthUsecase(&mockUserRepository{missing: true}, &mockSessionRepository{}, hasher)

	for range 2 {
		if _, _, _, err := usecase.Login(context.Background(), "nobody", "password", "", ""); err != domain.ErrInvalidCredentials {
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

	_, _, _, err := auth.Login(context.Background(), "bob", "a-valid-password", "192.0.2.1", "test")
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

	if _, _, _, err := auth.Login(context.Background(), "alice", "a-valid-password", "192.0.2.7", "test"); err != nil {
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
