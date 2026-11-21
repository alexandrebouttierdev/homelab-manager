package usecase

import (
	"context"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

type mockUserRepository struct {
	user domain.User
	hash string
}

func (m *mockUserRepository) FindByUsername(context.Context, string) (domain.User, string, error) {
	return m.user, m.hash, nil
}
func (m *mockUserRepository) FindByID(context.Context, uuid.UUID) (domain.User, error) {
	return m.user, nil
}
func (m *mockUserRepository) UpdatePasswordHash(context.Context, uuid.UUID, string) error {
	return nil
}
func (m *mockUserRepository) EnsureAdmin(context.Context, string, string) error {
	return nil
}

type mockSessionRepository struct{}

func (m *mockSessionRepository) Create(_ context.Context, userID uuid.UUID, _ string, _ string, _ string, expiresAt time.Time) (domain.Session, error) {
	return domain.Session{ID: uuid.New(), UserID: userID, ExpiresAt: expiresAt}, nil
}
func (m *mockSessionRepository) FindActiveByTokenHash(context.Context, string) (domain.Session, error) {
	return domain.Session{}, domain.ErrSessionNotFound
}
func (m *mockSessionRepository) RevokeByTokenHash(context.Context, string) error {
	return nil
}
func (m *mockSessionRepository) RevokeAllForUser(context.Context, uuid.UUID) error { return nil }

type mockHasher struct {
	valid bool
}

func (m *mockHasher) Hash(password string) (string, error) {
	return "hash:" + password, nil
}
func (m *mockHasher) Verify(password, encodedHash string) (bool, error) {
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
