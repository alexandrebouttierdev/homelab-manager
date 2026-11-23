package ports

import (
	"context"

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
