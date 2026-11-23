package sqlite

import (
	"context"
	"testing"
	"time"

	"homelab-manager/backend/ent/auditlog"
	"homelab-manager/backend/internal/domain"
)

func TestAuditRepositoryRecordsAnEntry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)

	entry := domain.AuditEntry{Actor: "admin", TargetType: "host", TargetID: "homelab", Action: "reboot", RequestID: "req-9", Outcome: "requested"}
	if err := NewAuditRepository(database.Client()).RecordAudit(ctx, entry); err != nil {
		t.Fatal(err)
	}
	record, err := database.Client().AuditLog.Query().Where(auditlog.Action("reboot")).Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if record.Actor != "admin" || record.TargetType != "host" || record.TargetID != "homelab" || record.Outcome != "requested" || record.RequestID != "req-9" {
		t.Fatalf("audit row = %+v", record)
	}
}
