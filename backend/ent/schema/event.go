package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Event struct{ ent.Schema }

func (Event) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("container_id", uuid.UUID{}).Optional().Nillable(),
		field.String("type").NotEmpty(),
		field.Enum("severity").Values("info", "warning", "error", "critical").Default("info"),
		field.String("message").NotEmpty(),
		field.JSON("metadata", map[string]any{}).Optional(),
		field.Time("occurred_at").Default(timeNow),
	}
}

func (Event) Edges() []ent.Edge {
	return []ent.Edge{edge.From("container", Container.Type).Ref("events").Unique().Field("container_id")}
}
