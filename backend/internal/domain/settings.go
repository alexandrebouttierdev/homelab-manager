package domain

import "errors"

// Bounds of the user-configurable metrics retention, in days.
const (
	MinRetentionDays = 1
	MaxRetentionDays = 365
)

var ErrInvalidRetention = errors.New("retention must be between 1 and 365 days")

// AuditEntry records an administrative change made from the interface.
type AuditEntry struct {
	Actor      string
	TargetType string
	TargetID   string
	Action     string
	RequestID  string
	Outcome    string
}
