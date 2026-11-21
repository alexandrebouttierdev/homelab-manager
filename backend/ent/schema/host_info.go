package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type HostInfo struct{ ent.Schema }

func (HostInfo) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("singleton_key").NotEmpty().Unique(),
		field.String("hostname").NotEmpty(),
		field.String("distribution").Optional(),
		field.String("os_version").Optional(),
		field.String("kernel_version").Optional(),
		field.Time("boot_time").Optional().Nillable(),
		field.Time("updated_at").Default(timeNow),
	}
}
