package usecase

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// alertCycleTimeout bounds one evaluation, database included, below
	// the 15 s period.
	alertCycleTimeout = 10 * time.Second
	// alertFreshness is how old the latest cycle of a metric may be.
	alertFreshness = currentWindow
	// alertPurgeInterval: resolved alerts are purged at start, then hourly.
	alertPurgeInterval = time.Hour
	// maxAlertContainers bounds the containers read at each evaluation.
	maxAlertContainers   = 500
	auditTargetAlertRule = "alert_rule"
	auditTargetAlert     = "alert"
)

// AlertMetricsReader reads the latest collected data.
type AlertMetricsReader interface {
	LatestMetrics(ctx context.Context, containerID *string, since time.Time) ([]domain.MetricSample, error)
	ListContainers(ctx context.Context, includeRemoved bool, limit int) ([]domain.Container, error)
	ports.LatestContainerMetrics
}

// CollectorStatusReader gives the status of the system and Docker
// collections (MonitoringUsecase).
type CollectorStatusReader interface {
	Status() (system, docker domain.CollectorStatus)
}

// UnitInventoryReader gives the systemd inventory (ServiceUsecase).
type UnitInventoryReader interface {
	Inventory() UnitInventory
}

// RetentionReader gives the effective retention (MonitoringUsecase).
type RetentionReader interface {
	Retention() time.Duration
}

// AlertSources are the read-only views the engine evaluates.
type AlertSources struct {
	Metrics    AlertMetricsReader
	Collectors CollectorStatusReader
	Units      UnitInventoryReader
	Retention  RetentionReader
}

// AlertUsecase evaluates the alert rules every 15 s on the data already
// collected (SPEC_ADMINISTRATION.md § 6). Run owns the evaluation
// goroutine; rule changes and evaluations are serialized so that an
// evaluation never fires an alert of a rule changed meanwhile.
type AlertUsecase struct {
	repo    ports.AlertRepository
	sources AlertSources
	logger  *zap.Logger
	now     func() time.Time

	interval time.Duration

	mu sync.Mutex
	// state is the memory of the engine: lost on restart, the durations
	// start again.
	state domain.AlertEngineState
}

func NewAlertUsecase(repo ports.AlertRepository, sources AlertSources, logger *zap.Logger) *AlertUsecase {
	return &AlertUsecase{repo: repo, sources: sources, logger: logger, now: time.Now, interval: domain.AlertEvaluationInterval}
}

// Run evaluates at once, then every 15 s, and purges resolved alerts at
// start then hourly, until ctx is cancelled.
func (u *AlertUsecase) Run(ctx context.Context) {
	ticker := time.NewTicker(u.interval)
	defer ticker.Stop()
	purge := time.NewTicker(alertPurgeInterval)
	defer purge.Stop()
	u.purge(ctx)
	for {
		u.evaluate(ctx)
		select {
		case <-ctx.Done():
			return
		case <-purge.C:
			u.purge(ctx)
		case <-ticker.C:
		}
	}
}

// evaluate compares the latest data with the rules and stores the
// transitions. On a database error the engine state is kept, so that the
// next evaluation computes the same transitions again.
func (u *AlertUsecase) evaluate(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, alertCycleTimeout)
	defer cancel()
	u.mu.Lock()
	defer u.mu.Unlock()
	rules, err := u.repo.AlertRules(ctx)
	if err != nil {
		u.logger.Warn("could not read the alert rules", zap.Error(err))
		return
	}
	open, err := u.repo.OpenAlerts(ctx)
	if err != nil {
		u.logger.Warn("could not read the open alerts", zap.Error(err))
		return
	}
	now := u.now().UTC()
	result := domain.EvaluateAlerts(rules, u.snapshot(ctx, now), open, u.state, now)
	if err := u.repo.ApplyAlertEvaluation(ctx, result.Fired, result.Resolved); err != nil {
		u.logger.Warn("could not record the alerts", zap.Error(err))
		return
	}
	u.state = result.State
	for _, alert := range result.Fired {
		u.logger.Info("alert fired", zap.String("rule", alert.RuleName), zap.String("target", alert.TargetKey), zap.String("severity", string(alert.Severity)))
	}
}

// snapshot reads every source; a source that cannot be read, or whose
// collection is not current, freezes the rules that depend on it.
func (u *AlertUsecase) snapshot(ctx context.Context, now time.Time) domain.AlertSnapshot {
	since := now.Add(-alertFreshness)
	system, docker := u.sources.Collectors.Status()
	inventory := u.sources.Units.Inventory()
	snapshot := domain.AlertSnapshot{Collectors: map[string]string{
		"system": system.Status, "docker": docker.Status, "services": string(inventory.Status),
	}}
	if host, err := u.sources.Metrics.LatestMetrics(ctx, nil, since); err != nil {
		u.logger.Warn("alerts: could not read the host metrics", zap.Error(err))
	} else if len(host) > 0 {
		snapshot.HostMetrics = host
	}
	if docker.Status == StatusOK {
		containers, err := u.sources.Metrics.ListContainers(ctx, false, maxAlertContainers)
		metrics, metricsErr := u.sources.Metrics.LatestContainerMetrics(ctx, since)
		switch {
		case err != nil:
			u.logger.Warn("alerts: could not read the containers", zap.Error(err))
		case metricsErr != nil:
			u.logger.Warn("alerts: could not read the container metrics", zap.Error(metricsErr))
		default:
			snapshot.ContainersKnown, snapshot.Containers, snapshot.ContainerMetrics = true, containers, metrics
		}
	}
	if inventory.Status == domain.UnitsOK && inventory.CollectedAt != nil {
		snapshot.UnitsKnown, snapshot.Units = true, inventory.Units
	}
	return snapshot
}

func (u *AlertUsecase) purge(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	before := u.now().UTC().Add(-u.sources.Retention.Retention())
	for {
		deleted, err := u.repo.DeleteResolvedAlertsBefore(ctx, before, purgeBatchSize)
		if err != nil {
			u.logger.Warn("could not purge the resolved alerts", zap.Error(err))
			return
		}
		if deleted < purgeBatchSize {
			return
		}
	}
}

// forget drops the engine memory of a rule after a change: its durations
// start again with the new parameters.
func (u *AlertUsecase) forget(id uuid.UUID) {
	prefix := id.String() + "|"
	for _, memory := range []map[string]time.Time{u.state.Pending, u.state.Missing} {
		for key := range memory {
			if strings.HasPrefix(key, prefix) {
				delete(memory, key)
			}
		}
	}
}

func (u *AlertUsecase) Rules(ctx context.Context) ([]domain.AlertRule, error) {
	return u.repo.AlertRules(ctx)
}

func (u *AlertUsecase) Rule(ctx context.Context, id uuid.UUID) (domain.AlertRule, error) {
	return u.repo.AlertRule(ctx, id)
}

// CreateRule validates and stores a rule; a created rule is never a
// default rule.
func (u *AlertUsecase) CreateRule(ctx context.Context, actor domain.User, input domain.AlertRule, requestID string) (domain.AlertRule, error) {
	if err := input.Validate(); err != nil {
		return domain.AlertRule{}, err
	}
	now := u.now().UTC()
	rule := input
	rule.ID, rule.Default, rule.CreatedAt, rule.UpdatedAt = uuid.New(), false, now, now
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.repo.CreateAlertRule(ctx, rule, alertAudit(actor, auditTargetAlertRule, rule.ID, "create", requestID)); err != nil {
		return domain.AlertRule{}, err
	}
	return rule, nil
}

// UpdateRule replaces a rule. Its open alerts are resolved when it is
// disabled (rule_disabled) or when its type, severity or parameters change
// (rule_changed); a rename keeps them.
func (u *AlertUsecase) UpdateRule(ctx context.Context, actor domain.User, id uuid.UUID, input domain.AlertRule, requestID string) (domain.AlertRule, error) {
	if err := input.Validate(); err != nil {
		return domain.AlertRule{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	current, err := u.repo.AlertRule(ctx, id)
	if err != nil {
		return domain.AlertRule{}, err
	}
	now := u.now().UTC()
	rule := input
	rule.ID, rule.Default, rule.CreatedAt, rule.UpdatedAt = id, current.Default, current.CreatedAt, now
	var resolution domain.AlertResolution
	switch {
	case current.Enabled && !rule.Enabled:
		resolution = domain.ResolutionRuleDisabled
	case current.Type != rule.Type || current.Severity != rule.Severity || !reflect.DeepEqual(current.Params, rule.Params):
		resolution = domain.ResolutionRuleChanged
	}
	if err := u.repo.UpdateAlertRule(ctx, rule, resolution, now, alertAudit(actor, auditTargetAlertRule, id, "update", requestID)); err != nil {
		return domain.AlertRule{}, err
	}
	if resolution != "" {
		u.forget(id)
	}
	return rule, nil
}

// DeleteRule deletes a rule that is not a default rule and resolves its
// open alerts.
func (u *AlertUsecase) DeleteRule(ctx context.Context, actor domain.User, id uuid.UUID, requestID string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.repo.DeleteAlertRule(ctx, id, u.now().UTC(), alertAudit(actor, auditTargetAlertRule, id, "delete", requestID)); err != nil {
		return err
	}
	u.forget(id)
	return nil
}

func (u *AlertUsecase) Alerts(ctx context.Context, filter domain.AlertFilter) ([]domain.Alert, error) {
	filter, err := filter.Validate()
	if err != nil {
		return nil, err
	}
	return u.repo.Alerts(ctx, filter)
}

func (u *AlertUsecase) Alert(ctx context.Context, id uuid.UUID) (domain.Alert, error) {
	return u.repo.Alert(ctx, id)
}

func (u *AlertUsecase) Summary(ctx context.Context) (domain.AlertSummary, error) {
	return u.repo.AlertSummary(ctx, u.now().UTC())
}

// Acknowledge stops the renotification of an open alert; it stays open
// until its resolution.
func (u *AlertUsecase) Acknowledge(ctx context.Context, actor domain.User, id uuid.UUID, requestID string) (domain.Alert, error) {
	return u.repo.AcknowledgeAlert(ctx, id, actor.Username, u.now().UTC(), alertAudit(actor, auditTargetAlert, id, "acknowledge", requestID))
}

func alertAudit(actor domain.User, targetType string, id uuid.UUID, action, requestID string) domain.AuditEntry {
	return domain.AuditEntry{
		Actor: actor.Username, TargetType: targetType, TargetID: id.String(), Action: action,
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
}
