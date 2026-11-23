package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// SecurityPolicy is the singleton row of the security policy set by an
// administrator (SPEC_ADMINISTRATION.md § 3.4).
type SecurityPolicy struct{ ent.Schema }

func (SecurityPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("singleton_key").NotEmpty().Unique(),
		field.Int("idle_timeout_minutes"),
		field.JSON("allowed_networks", []string{}),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}
