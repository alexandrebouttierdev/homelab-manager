package usecase

import (
	"context"
	"errors"
	"testing"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

type fakeAudit struct {
	entries []domain.AuditEntry
	err     error
}

func (f *fakeAudit) RecordAudit(_ context.Context, entry domain.AuditEntry) error {
	if f.err != nil {
		return f.err
	}
	f.entries = append(f.entries, entry)
	return nil
}

// fakePower records requests and how many audit entries existed then.
type fakePower struct {
	audit        *fakeAudit
	requested    []domain.PowerAction
	auditedFirst []int
	err          error
}

func (f *fakePower) Request(_ context.Context, action domain.PowerAction) error {
	f.requested = append(f.requested, action)
	f.auditedFirst = append(f.auditedFirst, len(f.audit.entries))
	return f.err
}

func newPowerFixture(enabled bool) (*PowerUsecase, *fakePower, *fakeAudit) {
	audit := &fakeAudit{}
	power := &fakePower{audit: audit}
	hostname := func() (string, error) { return "homelab", nil }
	return NewPowerUsecase(power, audit, hostname, enabled), power, audit
}

var powerAdmin = domain.User{ID: uuid.New(), Username: "admin", Role: domain.RoleAdmin}

func TestPowerRequestIsAuditedBeforeTheAction(t *testing.T) {
	usecase, power, audit := newPowerFixture(true)
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerReboot, " homelab ", "req-1"); err != nil {
		t.Fatal(err)
	}
	if len(power.requested) != 1 || power.requested[0] != domain.PowerReboot {
		t.Fatalf("requested = %v, want [reboot]", power.requested)
	}
	if power.auditedFirst[0] != 1 {
		t.Fatal("the audit entry must be written before asking the system")
	}
	want := domain.AuditEntry{Actor: "admin", TargetType: "host", TargetID: "homelab", Action: "reboot", RequestID: "req-1", Outcome: "requested"}
	if audit.entries[0] != want {
		t.Fatalf("audit = %+v, want %+v", audit.entries[0], want)
	}
}

func TestPowerRequestIsRefusedBeforeAnyEffect(t *testing.T) {
	cases := []struct {
		name         string
		enabled      bool
		action       domain.PowerAction
		confirmation string
		want         error
	}{
		{"disabled", false, domain.PowerReboot, "homelab", domain.ErrPowerActionsDisabled},
		{"wrong host", true, domain.PowerPowerOff, "other", domain.ErrHostnameMismatch},
		{"empty confirmation", true, domain.PowerPowerOff, "", domain.ErrHostnameMismatch},
		{"unknown action", true, "suspend", "homelab", domain.ErrInvalidPowerAction},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			usecase, power, audit := newPowerFixture(c.enabled)
			err := usecase.Request(context.Background(), powerAdmin, c.action, c.confirmation, "")
			if !errors.Is(err, c.want) {
				t.Fatalf("Request error = %v, want %v", err, c.want)
			}
			if len(power.requested) != 0 || len(audit.entries) != 0 {
				t.Fatal("a refused request must neither be audited nor reach the system")
			}
		})
	}
}

func TestPowerRequestWithoutAuditDoesNothing(t *testing.T) {
	usecase, power, audit := newPowerFixture(true)
	audit.err = errors.New("disk full")
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerReboot, "homelab", ""); err == nil {
		t.Fatal("expected an error when the audit cannot be written")
	}
	if len(power.requested) != 0 {
		t.Fatal("no trace, no action")
	}
}

func TestARefusedPowerRequestIsAuditedAndCanBeRetried(t *testing.T) {
	usecase, power, audit := newPowerFixture(true)
	power.err = domain.ErrPowerActionRefused
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerPowerOff, "homelab", ""); !errors.Is(err, domain.ErrPowerActionRefused) {
		t.Fatalf("Request error = %v, want ErrPowerActionRefused", err)
	}
	if len(audit.entries) != 2 || audit.entries[1].Outcome != "failure" {
		t.Fatalf("audit = %+v, want requested then failure", audit.entries)
	}
	power.err = nil
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerPowerOff, "homelab", ""); err != nil {
		t.Fatalf("a refused request must not block the next one: %v", err)
	}
}

func TestOnlyOnePowerRequestIsAccepted(t *testing.T) {
	usecase, power, _ := newPowerFixture(true)
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerReboot, "homelab", ""); err != nil {
		t.Fatal(err)
	}
	if err := usecase.Request(context.Background(), powerAdmin, domain.PowerPowerOff, "homelab", ""); !errors.Is(err, domain.ErrActionInProgress) {
		t.Fatalf("second request = %v, want ErrActionInProgress", err)
	}
	if len(power.requested) != 1 {
		t.Fatalf("requested = %v, want a single request", power.requested)
	}
}
