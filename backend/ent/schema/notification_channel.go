package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// NotificationChannel is a destination of the alert notifications;
// secrets holds its sealed secrets.
type NotificationChannel struct{ ent.Schema }

func (NotificationChannel) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("name").NotEmpty(),
		field.String("type").NotEmpty(),
		field.Bool("enabled").Default(true),
		field.String("min_severity").NotEmpty(),
		field.Int("renotify_minutes").Default(30),
		field.Text("settings").Default("{}"),
		field.Text("secrets").Default("").Sensitive(),
		field.Text("secret_names").Default("[]"),
		field.Time("last_sent_at").Optional().Nillable(),
		field.Time("last_error_at").Optional().Nillable(),
		field.String("last_error").Optional().Nillable(),
		field.Int("last_error_status").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow),
	}
}
