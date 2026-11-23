package ports

import (
	"context"

	"homelab-manager/backend/internal/domain"
)

// JournalRead describes one read of the system journal. The filter is
// already validated; its text search is applied by the caller.
type JournalRead struct {
	Filter domain.JournalFilter
	// After starts after this cursor, excluded; empty starts from the
	// newest entry when Reverse is set, else from the most recent Limit
	// entries in chronological order.
	After   string
	Reverse bool
	Limit   int
	// Fields limits the fields read; nil reads them all.
	Fields []string
}

// JournalReadStats tells how a read ended.
type JournalReadStats struct {
	// Skipped counts the entries too large to be read.
	Skipped int
	// Truncated is set when the read stopped at the output size bound
	// before Limit entries.
	Truncated bool
}

// JournalReader reads the systemd journal, read-only (journalctl).
type JournalReader interface {
	// FullAccess reports whether the Manager may read the whole journal
	// (group systemd-journal or adm); otherwise only its own entries.
	FullAccess() bool
	// Read gives the entries to visit in the order read, until visit
	// returns false or Limit entries were read.
	Read(ctx context.Context, request JournalRead, visit func(domain.JournalRecord) bool) (JournalReadStats, error)
	// Entry returns every field of one entry, or
	// domain.ErrJournalEntryNotFound.
	Entry(ctx context.Context, cursor string) (domain.JournalRecord, error)
	Boots(ctx context.Context) ([]domain.JournalBoot, error)
	Units(ctx context.Context) ([]string, error)
	DiskUsage(ctx context.Context) (int64, error)
}
