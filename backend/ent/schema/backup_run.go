package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// BackupRun is one run of homelab-backup@<operation>.service.
type BackupRun struct{ ent.Schema }

func (BackupRun) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("operation").NotEmpty(),
		field.String("origin").NotEmpty(),
		field.String("requested_by").Optional().Nillable(),
		field.String("state").NotEmpty(),
		field.String("failure").Optional().Nillable(),
		field.Text("message").Default(""),
		field.Bool("warning").Default(false),
		field.String("snapshot_id").Default(""),
		field.Int64("files_new").Optional().Nillable(),
		field.Int64("files_changed").Optional().Nillable(),
		field.Int64("files_unmodified").Optional().Nillable(),
		field.Int64("dirs_new").Optional().Nillable(),
		field.Int64("dirs_changed").Optional().Nillable(),
		field.Int64("dirs_unmodified").Optional().Nillable(),
		field.Int64("data_added").Optional().Nillable(),
		field.Int64("total_files_processed").Optional().Nillable(),
		field.Int64("total_bytes_processed").Optional().Nillable(),
		field.Float("duration_seconds").Optional().Nillable(),
		field.Text("details").Optional().Nillable(),
		field.Time("started_at"),
		field.Time("finished_at").Optional().Nillable(),
	}
}
