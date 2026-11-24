package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// AlertSilence suspends the notifications of the alerts it covers.
type AlertSilence struct{ ent.Schema }

func (AlertSilence) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("scope").NotEmpty(),
		field.UUID("rule_id", uuid.UUID{}).Optional().Nillable(),
		field.String("target_key").Optional().Nillable(),
		field.String("reason").NotEmpty(),
		field.String("created_by").NotEmpty(),
		field.Time("starts_at"),
		field.Time("ends_at"),
		field.Time("lifted_at").Optional().Nillable(),
		field.String("lifted_by").Optional().Nillable(),
	}
}
