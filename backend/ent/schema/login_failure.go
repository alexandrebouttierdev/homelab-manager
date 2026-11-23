package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// LoginFailure counts the consecutive failed passwords of an account; its
// id is the account UUID.
type LoginFailure struct{ ent.Schema }

func (LoginFailure) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.Int("consecutive_failures").Default(0),
		field.Time("last_failure_at"),
		field.Time("locked_until").Optional().Nillable(),
	}
}
