package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// BackupSnapshot is a snapshot of the repository made by the Manager; the
// table is replaced after each successful backup.
type BackupSnapshot struct{ ent.Schema }

func (BackupSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").NotEmpty().Immutable(),
		field.String("short_id"),
		field.Time("taken_at"),
		field.String("hostname").Default(""),
		field.JSON("paths", []string{}),
		field.JSON("tags", []string{}),
		field.Int64("total_files").Default(0),
		field.Int64("total_bytes").Default(0),
		field.Int64("data_added").Default(0),
	}
}
