package ports

import (
	"context"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// AlertRepository stores the alert rules and the alerts
// (SPEC_ADMINISTRATION.md § 6). Every change made by a user commits with
// its audit entry; a rule change resolves the open alerts of the rule in
// the same transaction.
type AlertRepository interface {
	AlertRules(ctx context.Context) ([]domain.AlertRule, error)
	AlertRule(ctx context.Context, id uuid.UUID) (domain.AlertRule, error)
	// CreateAlertRule fails with domain.ErrTooManyAlertRules beyond
	// domain.MaxAlertRules.
	CreateAlertRule(ctx context.Context, rule domain.AlertRule, audit domain.AuditEntry) error
	// UpdateAlertRule replaces the rule and resolves its open alerts with
	// resolution; an empty resolution (a rename) leaves them open.
	UpdateAlertRule(ctx context.Context, rule domain.AlertRule, resolution domain.AlertResolution, at time.Time, audit domain.AuditEntry) error
	// DeleteAlertRule fails with domain.ErrDefaultAlertRule for a default
	// rule and resolves the open alerts of the rule as rule_deleted.
	DeleteAlertRule(ctx context.Context, id uuid.UUID, at time.Time, audit domain.AuditEntry) error

	OpenAlerts(ctx context.Context) ([]domain.Alert, error)
	// ApplyAlertEvaluation stores the alerts fired and resolved by one
	// evaluation in a single transaction.
	ApplyAlertEvaluation(ctx context.Context, fired []domain.Alert, resolved []domain.AlertResolutionChange) error
	Alert(ctx context.Context, id uuid.UUID) (domain.Alert, error)
	// Alerts returns the history, most recent first.
	Alerts(ctx context.Context, filter domain.AlertFilter) ([]domain.Alert, error)
	// AcknowledgeAlert fails with domain.ErrAlertResolved for a resolved
	// alert; an acknowledged alert is returned unchanged.
	AcknowledgeAlert(ctx context.Context, id uuid.UUID, by string, at time.Time, audit domain.AuditEntry) (domain.Alert, error)
	AlertSummary(ctx context.Context, now time.Time) (domain.AlertSummary, error)
	// DeleteResolvedAlertsBefore purges resolved alerts, at most limit.
	DeleteResolvedAlertsBefore(ctx context.Context, before time.Time, limit int) (int, error)
}

// LatestContainerMetrics reads the latest collection cycle of every
// container recorded at or after since, keyed by the container.
type LatestContainerMetrics interface {
	LatestContainerMetrics(ctx context.Context, since time.Time) (map[uuid.UUID][]domain.MetricSample, error)
}
