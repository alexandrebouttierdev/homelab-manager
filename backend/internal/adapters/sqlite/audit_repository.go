package sqlite

import (
	"context"
	"fmt"

	"monoops/backend/ent"
	"monoops/backend/ent/auditlog"
	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"
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

// ListAudit returns the records matching the filter, newest first. The
// filter must have been validated (bounded limit).
func (r *AuditRepository) ListAudit(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, error) {
	query := r.client.AuditLog.Query()
	if filter.Actor != "" {
		query.Where(auditlog.Actor(filter.Actor))
	}
	if filter.TargetType != "" {
		query.Where(auditlog.TargetType(filter.TargetType))
	}
	if filter.Action != "" {
		query.Where(auditlog.Action(filter.Action))
	}
	if filter.Outcome != "" {
		query.Where(auditlog.Outcome(filter.Outcome))
	}
	// Timestamps are compared as UTC text: convert before binding.
	if filter.From != nil {
		query.Where(auditlog.CreatedAtGTE(filter.From.UTC()))
	}
	if filter.To != nil {
		query.Where(auditlog.CreatedAtLT(filter.To.UTC()))
	}
	records, err := query.Order(ent.Desc(auditlog.FieldCreatedAt), ent.Desc(auditlog.FieldID)).Limit(filter.Limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list audit records: %w", err)
	}
	result := make([]domain.AuditRecord, 0, len(records))
	for _, record := range records {
		result = append(result, domain.AuditRecord{
			ID: record.ID,
			AuditEntry: domain.AuditEntry{
				Actor: record.Actor, TargetType: record.TargetType, TargetID: record.TargetID,
				Action: record.Action, RequestID: record.RequestID, Outcome: record.Outcome,
			},
			CreatedAt: record.CreatedAt,
		})
	}
	return result, nil
}
