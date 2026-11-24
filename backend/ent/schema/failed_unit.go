package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// FailedUnit is a systemd unit recorded in the failed state; its id is the
// unit name.
type FailedUnit struct{ ent.Schema }

func (FailedUnit) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").NotEmpty(),
		field.Time("failed_at"),
	}
}
