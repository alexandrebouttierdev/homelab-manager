package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// RecoveryCode is a single-use second factor, stored hashed.
type RecoveryCode struct{ ent.Schema }

func (RecoveryCode) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("user_id", uuid.UUID{}),
		field.String("code_hash").Unique().Sensitive(),
		field.Time("used_at").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
	}
}
