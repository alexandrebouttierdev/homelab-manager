package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
	"time"
)

type User struct{ ent.Schema }

func timeNow() time.Time { return time.Now().UTC() }

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("username").Unique().NotEmpty(),
		field.String("password_hash").Sensitive(),
		field.String("display_name").Default(""),
		field.Enum("role").Values("viewer", "operator", "admin").Default("viewer"),
		field.Time("disabled_at").Optional().Nillable(),
		field.Bool("must_change_password").Default(false),
		field.Time("last_login_at").Optional().Nillable(),
		field.String("last_login_ip").Optional(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{edge.To("sessions", Session.Type)}
}
