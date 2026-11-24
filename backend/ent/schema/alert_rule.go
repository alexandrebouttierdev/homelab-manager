package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// AlertRule is a rule of the alert engine; params is the JSON object of
// its parameters, decoded by the adapter.
type AlertRule struct{ ent.Schema }

func (AlertRule) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("name").NotEmpty(),
		field.String("type").NotEmpty(),
		field.String("severity").NotEmpty(),
		field.Bool("enabled").Default(true),
		field.Bool("is_default").Default(false),
		field.Text("params").Default("{}"),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow),
	}
}
