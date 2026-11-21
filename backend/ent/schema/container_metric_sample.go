package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type ContainerMetricSample struct{ ent.Schema }

func (ContainerMetricSample) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("container_id", uuid.UUID{}),
		field.String("name").NotEmpty(),
		field.Float("value"),
		field.String("unit").Optional(),
		field.JSON("labels", map[string]string{}).Optional(),
		field.Time("recorded_at").Default(timeNow),
	}
}

func (ContainerMetricSample) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("container", Container.Type).Ref("metrics").Unique().Required().Field("container_id"),
	}
}
