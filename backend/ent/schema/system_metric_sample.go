package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type SystemMetricSample struct{ ent.Schema }

func (SystemMetricSample) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("name").NotEmpty(),
		field.Float("value"),
		field.String("unit").Optional(),
		field.JSON("labels", map[string]string{}).Optional(),
		field.Time("recorded_at").Default(timeNow),
	}
}
