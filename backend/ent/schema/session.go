package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Session struct{ ent.Schema }

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("user_id", uuid.UUID{}),
		field.String("token_hash").Unique().Sensitive(),
		field.Time("expires_at"),
		field.Time("created_at").Default(timeNow),
		field.Time("revoked_at").Optional().Nillable(),
		field.String("ip_address").Optional(),
		field.String("user_agent").Optional(),
	}
}

func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("sessions").Unique().Required().Field("user_id"),
	}
}
