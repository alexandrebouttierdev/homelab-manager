package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type AgentStatus struct{ ent.Schema }

func (AgentStatus) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.Enum("status").Values("unknown", "connected", "disconnected"),
		field.Time("observed_at").Default(timeNow),
		field.JSON("details", map[string]any{}).Optional(),
	}
}

func (AgentStatus) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("agent", Agent.Type).Ref("statuses").Unique(),
	}
}
