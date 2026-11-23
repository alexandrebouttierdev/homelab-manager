package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// TotpSecret is the sealed TOTP secret of an account; its id is the
// account UUID.
type TotpSecret struct{ ent.Schema }

func (TotpSecret) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}),
		field.String("secret_sealed").Sensitive(),
		field.Time("confirmed_at").Optional().Nillable(),
		field.Int64("last_used_step").Default(0),
		field.Time("created_at").Default(timeNow),
	}
}
