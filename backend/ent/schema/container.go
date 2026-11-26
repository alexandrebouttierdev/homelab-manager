package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Container struct{ ent.Schema }

func (Container) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("docker_id").NotEmpty().Unique(),
		field.String("name").NotEmpty(),
		field.String("image").NotEmpty(),
		field.String("state").NotEmpty(),
		field.String("status").Optional(),
		field.Time("created_at").Optional().Nillable(),
		field.Time("started_at").Optional().Nillable(),
		field.Time("first_seen_at").Default(timeNow),
		field.Time("last_seen_at").Default(timeNow),
		field.Time("removed_at").Optional().Nillable(),
		field.Uint64("restart_count").Default(0),
		field.String("restart_policy").Optional(),
		field.JSON("ports", []map[string]any{}).Optional(),
		field.JSON("networks", []string{}).Optional(),
		field.JSON("mounts", []map[string]any{}).Optional(),
		field.Time("updated_at").Default(timeNow),
	}
}

func (Container) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("events", Event.Type),
	}
}
