package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// AppSetting is the singleton row of settings changed from the interface.
// A nil field means "use the value from the service configuration".
type AppSetting struct{ ent.Schema }

func (AppSetting) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("singleton_key").NotEmpty().Unique(),
		field.Int("metrics_retention_days").Optional().Nillable(),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}
