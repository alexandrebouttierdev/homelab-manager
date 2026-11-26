package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type memoryTokens struct {
	tokens  map[uuid.UUID]domain.APIToken
	hashes  map[string]uuid.UUID
	touches int
}

func (m *memoryTokens) CreateAPIToken(_ context.Context, token domain.APIToken, hash string, _ domain.AuditEntry) error {
	m.tokens[token.ID], m.hashes[hash] = token, token.ID
	return nil
}
func (m *memoryTokens) APITokens(context.Context, *uuid.UUID) ([]domain.APIToken, error) {
	return nil, nil
}
func (m *memoryTokens) APIToken(_ context.Context, id uuid.UUID) (domain.APIToken, error) {
	token, ok := m.tokens[id]
	if !ok {
		return domain.APIToken{}, domain.ErrAPITokenNotFound
	}
	return token, nil
}
func (m *memoryTokens) APITokenByHash(_ context.Context, hash string) (domain.APIToken, error) {
	id, ok := m.hashes[hash]
	if !ok {
		return domain.APIToken{}, domain.ErrAPITokenNotFound
	}
	return m.tokens[id], nil
}
func (m *memoryTokens) DeleteAPIToken(_ context.Context, id uuid.UUID, _ domain.AuditEntry) error {
	delete(m.tokens, id)
	return nil
}
func (m *memoryTokens) TouchAPIToken(_ context.Context, id uuid.UUID, at time.Time) error {
	token := m.tokens[id]
	token.LastUsedAt = &at
	m.tokens[id] = token
	m.touches++
	return nil
}

// tokenUsers stands for the user repository: FindByID only.
type tokenUsers struct {
	users map[uuid.UUID]domain.User
}

func (u tokenUsers) FindByUsername(context.Context, string) (domain.User, string, error) {
	return domain.User{}, "", errors.New("unused")
}
func (u tokenUsers) FindByID(_ context.Context, id uuid.UUID) (domain.User, error) {
	user, ok := u.users[id]
	if !ok {
		return domain.User{}, errors.New("no such user")
	}
	return user, nil
}
func (u tokenUsers) UpdatePasswordHash(context.Context, uuid.UUID, string) error { return nil }
func (u tokenUsers) EnsureAdmin(context.Context, string, string) (bool, error)   { return false, nil }
func (u tokenUsers) RequirePasswordChange(context.Context, uuid.UUID) error      { return nil }
func (u tokenUsers) RecordLogin(context.Context, uuid.UUID, string, time.Time) error {
	return nil
}

func TestAPITokenAuthenticate(t *testing.T) {
	owner := domain.User{ID: uuid.New(), Username: "vera", Role: domain.RoleViewer}
	store := &memoryTokens{tokens: map[uuid.UUID]domain.APIToken{}, hashes: map[string]uuid.UUID{}}
	users := tokenUsers{users: map[uuid.UUID]domain.User{owner.ID: owner}}
	tokens := NewAPITokenUsecase(store, users, zap.NewNop())
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	tokens.now = func() time.Time { return now }

	secret, created, err := tokens.Create(context.Background(), owner, domain.APITokenRequest{Name: "ha", Scopes: []domain.APITokenScope{domain.ScopeMetricsRead}}, "r")
	if err != nil || !created.ExpiresAt.Equal(now.AddDate(0, 0, 90)) || created.Prefix != secret[:12] {
		t.Fatalf("create = %+v, %v", created, err)
	}
	user, token, err := tokens.Authenticate(context.Background(), secret)
	if err != nil || user.ID != owner.ID || !token.Allows(domain.ScopeMetricsRead) || store.touches != 1 {
		t.Fatalf("authenticate = %+v %+v %v (touches %d)", user, token, err, store.touches)
	}
	now = now.Add(30 * time.Second)
	if _, _, err := tokens.Authenticate(context.Background(), secret); err != nil || store.touches != 1 {
		t.Fatalf("second use within a minute: %v, touches %d", err, store.touches)
	}
	now = now.AddDate(0, 0, 90)
	if _, _, err := tokens.Authenticate(context.Background(), secret); !errors.Is(err, domain.ErrAPITokenUnauthorized) {
		t.Fatalf("expired token = %v", err)
	}
	if _, _, err := tokens.Authenticate(context.Background(), "mop_nope"); !errors.Is(err, domain.ErrAPITokenUnauthorized) {
		t.Fatalf("malformed token = %v", err)
	}
	other := domain.User{ID: uuid.New(), Username: "olivia", Role: domain.RoleOperator}
	if err := tokens.Revoke(context.Background(), other, created.ID, "r"); !errors.Is(err, domain.ErrAPITokenNotFound) {
		t.Fatalf("revoke by another account = %v", err)
	}
}
