package sqlite

import (
	"context"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

var _ ports.AuditRecorder = (*AuditRepository)(nil)

// AuditRepository records audit entries on their own, committed before the
// caller goes on.
type AuditRepository struct {
	client *ent.Client
}

func NewAuditRepository(client *ent.Client) *AuditRepository {
	return &AuditRepository{client: client}
}

func (r *AuditRepository) RecordAudit(ctx context.Context, entry domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		return writeAudit(ctx, tx, entry)
	})
}
