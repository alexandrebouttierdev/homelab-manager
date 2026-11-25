package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// MaintenanceTask is one task of the catalog the Manager schedules itself
// (decision D-1), keyed by its kind: no command is stored.
type MaintenanceTask struct{ ent.Schema }

func (MaintenanceTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").NotEmpty().Immutable(),
		field.Bool("enabled").Default(false),
		field.String("frequency").Default("weekly"),
		field.Int("weekday").Default(0),
		field.Int("hour").Default(4),
		field.Int("minute").Default(0),
		field.Int("max_size_mib").Default(512),
		field.Time("last_run_at").Optional().Nillable(),
		field.Bool("last_succeeded").Optional().Nillable(),
		field.String("last_message").Default(""),
		field.String("last_origin").Default(""),
		field.Time("updated_at").Optional().Nillable(),
		field.String("updated_by").Default(""),
	}
}
