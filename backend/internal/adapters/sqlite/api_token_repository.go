package sqlite

import (
	"context"
	"fmt"
	"time"

	"monoops/backend/ent"
	entapitoken "monoops/backend/ent/apitoken"
	entuser "monoops/backend/ent/user"
	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"

	"github.com/google/uuid"
)

var _ ports.APITokenRepository = (*APITokenRepository)(nil)

// APITokenRepository stores the API tokens.
type APITokenRepository struct {
	client *ent.Client
}

func NewAPITokenRepository(client *ent.Client) *APITokenRepository {
	return &APITokenRepository{client: client}
}

func scopeStrings(scopes []domain.APITokenScope) []string {
	values := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		values = append(values, string(scope))
	}
	return values
}

func (r *APITokenRepository) CreateAPIToken(ctx context.Context, token domain.APIToken, hash string, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		count, err := tx.APIToken.Query().Where(entapitoken.UserID(token.UserID)).Count(ctx)
		if err != nil {
			return fmt.Errorf("count API tokens: %w", err)
		}
		if count >= domain.MaxAPITokensPerUser {
			return domain.ErrTooManyAPITokens
		}
		if err := tx.APIToken.Create().SetID(token.ID).SetUserID(token.UserID).SetName(token.Name).
			SetTokenHash(hash).SetPrefix(token.Prefix).SetScopes(scopeStrings(token.Scopes)).
			SetCreatedAt(token.CreatedAt.UTC()).SetExpiresAt(token.ExpiresAt.UTC()).Exec(ctx); err != nil {
			return fmt.Errorf("store API token: %w", err)
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *APITokenRepository) APITokens(ctx context.Context, userID *uuid.UUID) ([]domain.APIToken, error) {
	query := r.client.APIToken.Query().Order(ent.Desc(entapitoken.FieldCreatedAt))
	if userID != nil {
		query = query.Where(entapitoken.UserID(*userID))
	}
	records, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list API tokens: %w", err)
	}
	names, err := r.usernames(ctx, records)
	if err != nil {
		return nil, err
	}
	tokens := make([]domain.APIToken, 0, len(records))
	for _, record := range records {
		token := toAPIToken(record)
		token.Username = names[record.UserID]
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func (r *APITokenRepository) usernames(ctx context.Context, records []*ent.APIToken) (map[uuid.UUID]string, error) {
	ids := make([]uuid.UUID, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.UserID)
	}
	users, err := r.client.User.Query().Where(entuser.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read token owners: %w", err)
	}
	names := make(map[uuid.UUID]string, len(users))
	for _, user := range users {
		names[user.ID] = user.Username
	}
	return names, nil
}

func (r *APITokenRepository) APIToken(ctx context.Context, id uuid.UUID) (domain.APIToken, error) {
	record, err := r.client.APIToken.Get(ctx, id)
	if ent.IsNotFound(err) {
		return domain.APIToken{}, domain.ErrAPITokenNotFound
	}
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("read API token: %w", err)
	}
	return toAPIToken(record), nil
}

func (r *APITokenRepository) APITokenByHash(ctx context.Context, hash string) (domain.APIToken, error) {
	record, err := r.client.APIToken.Query().Where(entapitoken.TokenHash(hash)).Only(ctx)
	if ent.IsNotFound(err) {
		return domain.APIToken{}, domain.ErrAPITokenNotFound
	}
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("find API token: %w", err)
	}
	return toAPIToken(record), nil
}

func (r *APITokenRepository) DeleteAPIToken(ctx context.Context, id uuid.UUID, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		err := tx.APIToken.DeleteOneID(id).Exec(ctx)
		if ent.IsNotFound(err) {
			return domain.ErrAPITokenNotFound
		}
		if err != nil {
			return fmt.Errorf("delete API token: %w", err)
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *APITokenRepository) TouchAPIToken(ctx context.Context, id uuid.UUID, at time.Time) error {
	if err := r.client.APIToken.UpdateOneID(id).SetLastUsedAt(at.UTC()).Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("touch API token: %w", err)
	}
	return nil
}

func toAPIToken(record *ent.APIToken) domain.APIToken {
	scopes := make([]domain.APITokenScope, 0, len(record.Scopes))
	for _, scope := range record.Scopes {
		scopes = append(scopes, domain.APITokenScope(scope))
	}
	return domain.APIToken{
		ID: record.ID, UserID: record.UserID, Name: record.Name, Prefix: record.Prefix, Scopes: scopes,
		CreatedAt: record.CreatedAt.UTC(), ExpiresAt: record.ExpiresAt.UTC(), LastUsedAt: utcPointer(record.LastUsedAt),
	}
}
