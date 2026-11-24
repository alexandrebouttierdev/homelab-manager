package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// BackupConfig is the singleton configuration of the backups module
// (SPEC_ADMINISTRATION.md § 7).
type BackupConfig struct{ ent.Schema }

func (BackupConfig) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("singleton_key").NotEmpty().Unique(),
		field.String("repository_type").Optional().Nillable(),
		field.String("repository_location").Optional().Nillable(),
		field.Text("sealed_credentials").Default("").Sensitive(),
		field.Time("initialized_at").Optional().Nillable(),
		field.JSON("sources", []string{}),
		field.JSON("excludes", []string{}),
		field.Bool("exclude_caches").Default(true),
		field.String("exclude_if_present").Default(""),
		field.Bool("schedule_enabled").Default(false),
		field.String("schedule_frequency").Default("daily"),
		field.Int("schedule_weekday").Default(0),
		field.Int("schedule_hour").Default(3),
		field.Int("schedule_minute").Default(0),
		field.Time("updated_at"),
		field.String("updated_by").Default(""),
	}
}
