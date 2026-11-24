package ports

import (
	"context"
	"time"

	"homelab-manager/backend/internal/domain"
)

// SystemUnitReader reads the systemd manager, read-only. No method that
// changes a unit is ever called (SPEC_ADMINISTRATION.md § 2.1).
type SystemUnitReader interface {
	// Units lists the loaded service, timer and socket units, failed units
	// first then by name, at most domain.MaxSystemUnits. An unreachable
	// bus or systemd wraps domain.ErrSystemdUnavailable.
	Units(ctx context.Context) ([]domain.SystemUnit, error)
	// Unit reads the detail of one unit, or domain.ErrUnitNotFound.
	Unit(ctx context.Context, name string) (domain.SystemUnitDetail, error)
}

// FailedUnitRepository keeps the set of units recorded as failed, so that
// a failure is reported once across restarts of the Manager.
type FailedUnitRepository interface {
	FailedUnits(ctx context.Context) (map[string]struct{}, error)
	// ApplyFailureTransitions updates the set and writes the events in one
	// transaction; added units are recorded as failed at the given time.
	ApplyFailureTransitions(ctx context.Context, transitions domain.FailureTransitions, at time.Time) error
}
