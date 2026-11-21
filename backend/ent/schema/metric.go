package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Metric struct{ ent.Schema }

func (Metric) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("name").NotEmpty(),
		field.Float("value"),
		field.String("unit").Optional(),
		field.JSON("labels", map[string]string{}).Optional(),
		field.Time("recorded_at").Default(timeNow),
	}
}

func (Metric) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("agent", Agent.Type).Ref("metrics").Unique().Required(),
	}
}
