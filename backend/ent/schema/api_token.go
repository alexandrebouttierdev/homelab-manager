package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// APIToken is a read-only API token, stored as its SHA-256.
type APIToken struct{ ent.Schema }

func (APIToken) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("user_id", uuid.UUID{}),
		field.String("name").NotEmpty(),
		field.String("token_hash").Unique().Sensitive(),
		field.String("prefix"),
		field.JSON("scopes", []string{}),
		field.Time("created_at"),
		field.Time("expires_at"),
		field.Time("last_used_at").Optional().Nillable(),
	}
}
