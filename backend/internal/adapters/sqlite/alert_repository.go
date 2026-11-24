package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"homelab-manager/backend/ent"
	entalert "homelab-manager/backend/ent/alert"
	entalertrule "homelab-manager/backend/ent/alertrule"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

var _ ports.AlertRepository = (*AlertRepository)(nil)

// AlertRepository stores the alert rules and the alerts.
type AlertRepository struct {
	client *ent.Client
}

func NewAlertRepository(client *ent.Client) *AlertRepository {
	return &AlertRepository{client: client}
}

func (r *AlertRepository) AlertRules(ctx context.Context) ([]domain.AlertRule, error) {
	records, err := r.client.AlertRule.Query().
		Order(entalertrule.ByIsDefault(entsql.OrderDesc()), entalertrule.ByName(), entalertrule.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read alert rules: %w", err)
	}
	rules := make([]domain.AlertRule, 0, len(records))
	for _, record := range records {
		rule, err := alertRuleDomain(record)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (r *AlertRepository) AlertRule(ctx context.Context, id uuid.UUID) (domain.AlertRule, error) {
	record, err := r.client.AlertRule.Get(ctx, id)
	if ent.IsNotFound(err) {
		return domain.AlertRule{}, domain.ErrAlertRuleNotFound
	}
	if err != nil {
		return domain.AlertRule{}, fmt.Errorf("read alert rule: %w", err)
	}
	return alertRuleDomain(record)
}

func (r *AlertRepository) CreateAlertRule(ctx context.Context, rule domain.AlertRule, audit domain.AuditEntry) error {
	params, err := json.Marshal(rule.Params)
	if err != nil {
		return fmt.Errorf("encode alert rule: %w", err)
	}
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		count, err := tx.AlertRule.Query().Count(ctx)
		if err != nil {
			return fmt.Errorf("count alert rules: %w", err)
		}
		if count >= domain.MaxAlertRules {
			return domain.ErrTooManyAlertRules
		}
		if err := tx.AlertRule.Create().
			SetID(rule.ID).SetName(rule.Name).SetType(string(rule.Type)).SetSeverity(string(rule.Severity)).
			SetEnabled(rule.Enabled).SetIsDefault(false).SetParams(string(params)).
			SetCreatedAt(rule.CreatedAt.UTC()).SetUpdatedAt(rule.UpdatedAt.UTC()).
			Exec(ctx); err != nil {
			return fmt.Errorf("create alert rule: %w", err)
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *AlertRepository) UpdateAlertRule(ctx context.Context, rule domain.AlertRule, resolution domain.AlertResolution, at time.Time, audit domain.AuditEntry) error {
	params, err := json.Marshal(rule.Params)
	if err != nil {
		return fmt.Errorf("encode alert rule: %w", err)
	}
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		updated, err := tx.AlertRule.Update().Where(entalertrule.ID(rule.ID)).
			SetName(rule.Name).SetType(string(rule.Type)).SetSeverity(string(rule.Severity)).
			SetEnabled(rule.Enabled).SetParams(string(params)).SetUpdatedAt(rule.UpdatedAt.UTC()).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("update alert rule: %w", err)
		}
		if updated == 0 {
			return domain.ErrAlertRuleNotFound
		}
		if resolution != "" {
			if err := resolveRuleAlerts(ctx, tx, rule.ID, resolution, at); err != nil {
				return err
			}
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *AlertRepository) DeleteAlertRule(ctx context.Context, id uuid.UUID, at time.Time, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		record, err := tx.AlertRule.Get(ctx, id)
		if ent.IsNotFound(err) {
			return domain.ErrAlertRuleNotFound
		}
		if err != nil {
			return fmt.Errorf("read alert rule: %w", err)
		}
		if record.IsDefault {
			return domain.ErrDefaultAlertRule
		}
		if err := tx.AlertRule.DeleteOneID(id).Exec(ctx); err != nil {
			return fmt.Errorf("delete alert rule: %w", err)
		}
		if err := resolveRuleAlerts(ctx, tx, id, domain.ResolutionRuleDeleted, at); err != nil {
			return err
		}
		return writeAudit(ctx, tx, audit)
	})
}

func resolveRuleAlerts(ctx context.Context, tx *ent.Tx, ruleID uuid.UUID, resolution domain.AlertResolution, at time.Time) error {
	if _, err := tx.Alert.Update().
		Where(entalert.RuleID(ruleID), entalert.StateNEQ(string(domain.AlertResolved))).
		SetState(string(domain.AlertResolved)).SetResolvedAt(at.UTC()).SetResolution(string(resolution)).
		Save(ctx); err != nil {
		return fmt.Errorf("resolve alerts of the rule: %w", err)
	}
	return nil
}

func (r *AlertRepository) OpenAlerts(ctx context.Context) ([]domain.Alert, error) {
	records, err := r.client.Alert.Query().
		Where(entalert.StateNEQ(string(domain.AlertResolved))).
		Order(entalert.ByFiredAt(entsql.OrderDesc()), entalert.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read open alerts: %w", err)
	}
	return alertsDomain(records)
}

func (r *AlertRepository) ApplyAlertEvaluation(ctx context.Context, fired []domain.Alert, resolved []domain.AlertResolutionChange) error {
	if len(fired) == 0 && len(resolved) == 0 {
		return nil
	}
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		// Resolutions first: a target may be resolved and fire again in
		// the same evaluation only after a rule change.
		for _, change := range resolved {
			if _, err := tx.Alert.Update().
				Where(entalert.ID(change.ID), entalert.StateNEQ(string(domain.AlertResolved))).
				SetState(string(domain.AlertResolved)).SetResolvedAt(change.At.UTC()).SetResolution(string(change.Resolution)).
				Save(ctx); err != nil {
				return fmt.Errorf("resolve alert: %w", err)
			}
		}
		for _, alert := range fired {
			target, err := json.Marshal(alert.Target)
			if err != nil {
				return fmt.Errorf("encode alert target: %w", err)
			}
			if err := tx.Alert.Create().
				SetID(alert.ID).SetRuleID(alert.RuleID).SetRuleName(alert.RuleName).SetRuleType(string(alert.RuleType)).
				SetSeverity(string(alert.Severity)).SetTargetKey(alert.TargetKey).SetTarget(string(target)).
				SetState(string(alert.State)).SetNillableValue(alert.Value).SetNillableThreshold(alert.Threshold).
				SetObserved(alert.Observed).SetFiredAt(alert.FiredAt.UTC()).
				Exec(ctx); err != nil {
				return fmt.Errorf("record alert: %w", err)
			}
		}
		return nil
	})
}

func (r *AlertRepository) Alert(ctx context.Context, id uuid.UUID) (domain.Alert, error) {
	record, err := r.client.Alert.Get(ctx, id)
	if ent.IsNotFound(err) {
		return domain.Alert{}, domain.ErrAlertNotFound
	}
	if err != nil {
		return domain.Alert{}, fmt.Errorf("read alert: %w", err)
	}
	return alertDomain(record)
}

func (r *AlertRepository) Alerts(ctx context.Context, filter domain.AlertFilter) ([]domain.Alert, error) {
	query := r.client.Alert.Query()
	switch filter.State {
	case "open":
		query = query.Where(entalert.StateNEQ(string(domain.AlertResolved)))
	case "resolved":
		query = query.Where(entalert.State(string(domain.AlertResolved)))
	}
	if filter.From != nil {
		query = query.Where(entalert.FiredAtGTE(filter.From.UTC()))
	}
	if filter.To != nil {
		query = query.Where(entalert.FiredAtLT(filter.To.UTC()))
	}
	records, err := query.Order(entalert.ByFiredAt(entsql.OrderDesc()), entalert.ByID()).Limit(filter.Limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read alerts: %w", err)
	}
	return alertsDomain(records)
}

func (r *AlertRepository) AcknowledgeAlert(ctx context.Context, id uuid.UUID, by string, at time.Time, audit domain.AuditEntry) (domain.Alert, error) {
	var result domain.Alert
	err := withTx(ctx, r.client, func(tx *ent.Tx) error {
		record, err := tx.Alert.Get(ctx, id)
		if ent.IsNotFound(err) {
			return domain.ErrAlertNotFound
		}
		if err != nil {
			return fmt.Errorf("read alert: %w", err)
		}
		switch domain.AlertState(record.State) {
		case domain.AlertResolved:
			return domain.ErrAlertResolved
		case domain.AlertAcknowledged:
			result, err = alertDomain(record)
			return err
		}
		record, err = tx.Alert.UpdateOne(record).
			SetState(string(domain.AlertAcknowledged)).SetAcknowledgedAt(at.UTC()).SetAcknowledgedBy(by).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("acknowledge alert: %w", err)
		}
		if result, err = alertDomain(record); err != nil {
			return err
		}
		return writeAudit(ctx, tx, audit)
	})
	return result, err
}

func (r *AlertRepository) AlertSummary(ctx context.Context, now time.Time) (domain.AlertSummary, error) {
	summary := domain.AlertSummary{Open: map[domain.AlertSeverity]int{}}
	for _, severity := range domain.AlertSeverities {
		summary.Open[severity] = 0
	}
	open, err := r.client.Alert.Query().Where(entalert.StateNEQ(string(domain.AlertResolved))).
		Select(entalert.FieldSeverity).Strings(ctx)
	if err != nil {
		return domain.AlertSummary{}, fmt.Errorf("count open alerts: %w", err)
	}
	for _, severity := range open {
		summary.Open[domain.AlertSeverity(severity)]++
	}
	resolved, err := r.client.Alert.Query().
		Where(entalert.State(string(domain.AlertResolved)), entalert.ResolvedAtGTE(now.Add(-24*time.Hour).UTC())).
		All(ctx)
	if err != nil {
		return domain.AlertSummary{}, fmt.Errorf("read resolved alerts: %w", err)
	}
	summary.ResolvedLast24h = len(resolved)
	if len(resolved) > 0 {
		var total float64
		for _, record := range resolved {
			if record.ResolvedAt != nil {
				total += record.ResolvedAt.Sub(record.FiredAt).Seconds()
			}
		}
		mean := total / float64(len(resolved))
		summary.MeanResolutionSeconds24h = &mean
	}
	return summary, nil
}

func (r *AlertRepository) DeleteResolvedAlertsBefore(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit < 1 || limit > 10000 {
		limit = 1000
	}
	ids, err := r.client.Alert.Query().
		Where(entalert.State(string(domain.AlertResolved)), entalert.ResolvedAtLT(before.UTC())).
		Order(entalert.ByResolvedAt()).Limit(limit).IDs(ctx)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return r.client.Alert.Delete().Where(entalert.IDIn(ids...)).Exec(ctx)
}

func alertRuleDomain(record *ent.AlertRule) (domain.AlertRule, error) {
	rule := domain.AlertRule{
		ID: record.ID, Name: record.Name, Type: domain.AlertRuleType(record.Type), Severity: domain.AlertSeverity(record.Severity),
		Enabled: record.Enabled, Default: record.IsDefault, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if err := json.Unmarshal([]byte(record.Params), &rule.Params); err != nil {
		return domain.AlertRule{}, fmt.Errorf("decode alert rule %s: %w", record.ID, err)
	}
	return rule, nil
}

func alertsDomain(records []*ent.Alert) ([]domain.Alert, error) {
	alerts := make([]domain.Alert, 0, len(records))
	for _, record := range records {
		alert, err := alertDomain(record)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

func alertDomain(record *ent.Alert) (domain.Alert, error) {
	alert := domain.Alert{
		ID: record.ID, RuleID: record.RuleID, RuleName: record.RuleName, RuleType: domain.AlertRuleType(record.RuleType),
		Severity: domain.AlertSeverity(record.Severity), TargetKey: record.TargetKey, State: domain.AlertState(record.State),
		Value: record.Value, Threshold: record.Threshold, Observed: record.Observed, FiredAt: record.FiredAt,
		AcknowledgedAt: record.AcknowledgedAt, ResolvedAt: record.ResolvedAt,
	}
	if record.AcknowledgedBy != nil {
		alert.AcknowledgedBy = *record.AcknowledgedBy
	}
	if record.Resolution != nil {
		alert.Resolution = domain.AlertResolution(*record.Resolution)
	}
	if err := json.Unmarshal([]byte(record.Target), &alert.Target); err != nil {
		return domain.Alert{}, errors.Join(fmt.Errorf("decode alert target %s", record.ID), err)
	}
	return alert, nil
}
