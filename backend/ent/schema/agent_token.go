package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type AgentToken struct{ ent.Schema }

func (AgentToken) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("token_hash").Unique().Sensitive(),
		field.Time("expires_at"),
		field.Time("used_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
	}
}

func (AgentToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("agent", Agent.Type).Ref("tokens").Unique(),
	}
}
