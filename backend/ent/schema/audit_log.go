package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type AuditLog struct{ ent.Schema }

func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("actor").NotEmpty(),
		field.String("target_type").NotEmpty(),
		field.String("target_id").NotEmpty(),
		field.String("action").NotEmpty(),
		field.String("request_id").Optional(),
		field.String("outcome").NotEmpty(),
		field.Time("created_at").Default(timeNow),
	}
}
