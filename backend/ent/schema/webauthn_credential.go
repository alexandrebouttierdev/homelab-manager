package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// WebauthnCredential is a passkey or a security key of an account.
type WebauthnCredential struct{ ent.Schema }

func (WebauthnCredential) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("user_id", uuid.UUID{}),
		field.String("name").NotEmpty(),
		field.Bytes("credential_id").Unique(),
		field.Bytes("data"),
		field.Time("created_at"),
		field.Time("last_used_at").Optional().Nillable(),
	}
}
