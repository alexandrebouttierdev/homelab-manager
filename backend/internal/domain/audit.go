package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Outcomes of an audit entry.
const (
	AuditSuccess   = "success"
	AuditDenied    = "denied"
	AuditFailure   = "failure"
	AuditRequested = "requested"
)

// Bounds of the audit log consultation (SPEC_ADMINISTRATION.md § 3.5).
const (
	DefaultAuditLimit = 200
	MaxAuditLimit     = 1000
	MaxAuditExport    = 10000
	maxAuditTextBytes = 255
)

var ErrInvalidAuditFilter = errors.New("audit filter values are out of bounds")

// AuditRecord is a stored audit entry.
type AuditRecord struct {
	ID uuid.UUID
	AuditEntry
	CreatedAt time.Time
}

// AuditFilter selects audit records; empty fields do not filter. The time
// range is [From, To).
type AuditFilter struct {
	Actor      string
	TargetType string
	Action     string
	Outcome    string
	From       *time.Time
	To         *time.Time
	Limit      int
}

// Validate checks the bounds; a zero limit becomes the default.
func (f AuditFilter) Validate(maxLimit int) (AuditFilter, error) {
	if f.Limit < 0 || f.Limit > maxLimit {
		return AuditFilter{}, ErrInvalidAuditFilter
	}
	if f.Limit == 0 {
		f.Limit = min(DefaultAuditLimit, maxLimit)
	}
	for _, value := range []string{f.Actor, f.TargetType, f.Action, f.Outcome} {
		if len(value) > maxAuditTextBytes {
			return AuditFilter{}, ErrInvalidAuditFilter
		}
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return AuditFilter{}, ErrInvalidAuditFilter
	}
	return f, nil
}
