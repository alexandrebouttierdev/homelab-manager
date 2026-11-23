package sqlite

import (
	"context"
	"fmt"

	"homelab-manager/backend/ent"
	entevent "homelab-manager/backend/ent/event"
	"homelab-manager/backend/internal/domain"
)

// withTx runs fn in a transaction, committed only when fn succeeds. fn must
// only use tx: the database has a single connection.
func withTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// writeAudit records an administrative change in the transaction that
// makes it, so that the change and its trace commit together.
func writeAudit(ctx context.Context, tx *ent.Tx, audit domain.AuditEntry) error {
	create := tx.AuditLog.Create().
		SetActor(audit.Actor).SetTargetType(audit.TargetType).SetTargetID(audit.TargetID).
		SetAction(audit.Action).SetOutcome(audit.Outcome)
	if audit.RequestID != "" {
		create.SetRequestID(audit.RequestID)
	}
	if _, err := create.Save(ctx); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

// writeEvent records a monitoring event in the transaction that detects
// it.
func writeEvent(ctx context.Context, tx *ent.Tx, event domain.MonitorEvent) error {
	create := tx.Event.Create().
		SetID(event.ID).SetType(event.Type).SetSeverity(entevent.Severity(event.Severity)).
		SetMessage(event.Message).SetOccurredAt(event.OccurredAt)
	if event.ContainerID != nil {
		create.SetContainerID(*event.ContainerID)
	}
	if event.Metadata != nil {
		create.SetMetadata(event.Metadata)
	}
	if _, err := create.Save(ctx); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}
