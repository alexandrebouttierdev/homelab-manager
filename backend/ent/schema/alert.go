package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Alert is one firing of a rule on a target. rule_id is not an edge: the
// history outlives a deleted rule.
type Alert struct{ ent.Schema }

func (Alert) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("rule_id", uuid.UUID{}),
		field.String("rule_name").NotEmpty(),
		field.String("rule_type").NotEmpty(),
		field.String("severity").NotEmpty(),
		field.String("target_key").NotEmpty(),
		field.Text("target").Default("{}"),
		field.String("state").NotEmpty(),
		field.Float("value").Optional().Nillable(),
		field.Float("threshold").Optional().Nillable(),
		field.String("observed").Default(""),
		field.Time("fired_at"),
		field.Time("acknowledged_at").Optional().Nillable(),
		field.String("acknowledged_by").Optional().Nillable(),
		field.Time("resolved_at").Optional().Nillable(),
		field.String("resolution").Optional().Nillable(),
		field.Time("last_notified_at").Optional().Nillable(),
	}
}
