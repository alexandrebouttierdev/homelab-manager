package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Agent struct{ ent.Schema }

func (Agent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("name").NotEmpty(),
		field.String("hostname").Unique().NotEmpty(),
		field.String("agent_version").Optional(),
		field.String("os").Optional(),
		field.String("architecture").Optional(),
		field.JSON("capabilities", map[string]any{}).Optional(),
		field.Time("last_seen_at").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Time("revoked_at").Optional().Nillable(),
	}
}

func (Agent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("tokens", AgentToken.Type),
		edge.To("statuses", AgentStatus.Type),
		edge.To("metrics", Metric.Type),
	}
}
