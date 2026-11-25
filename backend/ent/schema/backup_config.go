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
		field.Text("repository_host_key").Default(""),
		field.Text("sealed_credentials").Default("").Sensitive(),
		field.Time("initialized_at").Optional().Nillable(),
		field.JSON("sources", []string{}),
		field.JSON("excludes", []string{}),
		field.Bool("exclude_caches").Default(true),
		field.String("exclude_if_present").Default(""),
		field.JSON("sqlite_databases", []string{}).Default([]string{}),
		field.Bool("schedule_enabled").Default(false),
		field.String("schedule_frequency").Default("daily"),
		field.Int("schedule_weekday").Default(0),
		field.Int("schedule_hour").Default(3),
		field.Int("schedule_minute").Default(0),
		field.Bool("retention_enabled").Default(false),
		field.Int("keep_daily").Default(7),
		field.Int("keep_weekly").Default(4),
		field.Int("keep_monthly").Default(6),
		field.Int("keep_yearly").Default(0),
		field.Bool("check_enabled").Default(true),
		field.Int("check_subset_percent").Default(5),
		field.String("copy_repository_type").Optional().Nillable(),
		field.String("copy_repository_location").Optional().Nillable(),
		field.Text("copy_repository_host_key").Default(""),
		field.Text("copy_sealed_credentials").Default("").Sensitive(),
		field.Time("copy_initialized_at").Optional().Nillable(),
		field.Bool("copy_after_backup").Default(false),
		field.Text("ssh_public_key").Default(""),
		field.Text("ssh_sealed_private_key").Default("").Sensitive(),
		field.Time("ssh_key_created_at").Optional().Nillable(),
		field.Time("updated_at"),
		field.String("updated_by").Default(""),
	}
}
