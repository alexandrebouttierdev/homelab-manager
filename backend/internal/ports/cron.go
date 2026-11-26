package ports

import (
	"context"

	"monoops/backend/internal/domain"
)

// CronReader reads the system crontabs, read-only (SPEC_ADMINISTRATION.md
// § 8). A missing file is no error; an unreadable one is listed.
type CronReader interface {
	Read(ctx context.Context) (domain.CronInventory, error)
}
