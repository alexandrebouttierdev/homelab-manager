package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

var alertAt = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

func alertAudit(action string) domain.AuditEntry {
	return domain.AuditEntry{Actor: "admin", TargetType: "alert_rule", TargetID: "x", Action: action, Outcome: domain.AuditSuccess}
}

func fired(rule domain.AlertRule, targetKey string, at time.Time) domain.Alert {
	value, threshold := 91.0, 85.0
	return domain.Alert{
		ID: uuid.New(), RuleID: rule.ID, RuleName: rule.Name, RuleType: rule.Type, Severity: rule.Severity,
		TargetKey: targetKey, Target: domain.AlertTarget{Kind: domain.AlertTargetHost, Metric: "filesystem.percent", Labels: map[string]string{"mountpoint": "/"}},
		State: domain.AlertFiring, Value: &value, Threshold: &threshold, FiredAt: at,
	}
}

func customRule(name string) domain.AlertRule {
	threshold := 85.0
	return domain.AlertRule{
		ID: uuid.New(), Name: name, Type: domain.AlertRuleHostMetric, Severity: domain.AlertWarning, Enabled: true,
		Params:    domain.AlertRuleParams{Metric: "filesystem.percent", Labels: map[string]string{"mountpoint": "/data"}, Operator: domain.OpGreater, Threshold: &threshold, ForSeconds: 60},
		CreatedAt: alertAt, UpdatedAt: alertAt,
	}
}

func TestDefaultAlertRulesAreSeeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := NewAlertRepository(openTestDatabase(t, ctx).Client())
	rules, err := repository.AlertRules(ctx)
	if err != nil {
		t.Fatalf("AlertRules: %v", err)
	}
	// Six from 007, the backup rule from 011.
	if len(rules) != 7 {
		t.Fatalf("seeded %d rules, want 7", len(rules))
	}
	for _, rule := range rules {
		if !rule.Default || !rule.Enabled {
			t.Errorf("rule %q: default %v enabled %v", rule.Name, rule.Default, rule.Enabled)
		}
		if err := rule.Validate(); err != nil {
			t.Errorf("seeded rule %q is invalid: %v", rule.Name, err)
		}
	}
	disk, err := repository.AlertRule(ctx, uuid.MustParse("6f1c2a10-7a4e-4c4b-9a51-000000000001"))
	if err != nil || disk.Params.Metric != "filesystem.percent" || disk.Params.Operator != domain.OpGreater ||
		disk.Params.Threshold == nil || *disk.Params.Threshold != 85 || disk.Params.ForSeconds != 300 || disk.Severity != domain.AlertWarning {
		t.Fatalf("disk rule = (%+v, %v)", disk, err)
	}
}

func TestAlertRuleChangesCommitWithTheirAuditAndResolveOpenAlerts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	repository := NewAlertRepository(database.Client())
	audits := NewAuditRepository(database.Client())

	rule := customRule("data disk")
	if err := repository.CreateAlertRule(ctx, rule, alertAudit("alert_rule.create")); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	stored, err := repository.AlertRule(ctx, rule.ID)
	if err != nil || stored.Name != "data disk" || stored.Default || stored.Params.Labels["mountpoint"] != "/data" {
		t.Fatalf("stored rule = (%+v, %v)", stored, err)
	}
	alert := fired(rule, "host:filesystem.percent{mountpoint=/data}", alertAt)
	if err := repository.ApplyAlertEvaluation(ctx, []domain.Alert{alert}, nil); err != nil {
		t.Fatalf("ApplyAlertEvaluation: %v", err)
	}

	rule.Params.ForSeconds = 120
	rule.UpdatedAt = alertAt.Add(time.Minute)
	if err := repository.UpdateAlertRule(ctx, rule, domain.ResolutionRuleChanged, alertAt.Add(time.Minute), alertAudit("alert_rule.update")); err != nil {
		t.Fatalf("UpdateAlertRule: %v", err)
	}
	if got, _ := repository.AlertRule(ctx, rule.ID); got.Params.ForSeconds != 120 {
		t.Fatalf("rule not updated: %+v", got)
	}
	resolved, err := repository.Alert(ctx, alert.ID)
	if err != nil || resolved.State != domain.AlertResolved || resolved.Resolution != domain.ResolutionRuleChanged || resolved.ResolvedAt == nil {
		t.Fatalf("alert after the rule change = (%+v, %v)", resolved, err)
	}

	second := fired(rule, "host:filesystem.percent{mountpoint=/data}", alertAt.Add(2*time.Minute))
	if err := repository.ApplyAlertEvaluation(ctx, []domain.Alert{second}, nil); err != nil {
		t.Fatalf("firing again after the resolution: %v", err)
	}
	if err := repository.DeleteAlertRule(ctx, rule.ID, alertAt.Add(3*time.Minute), alertAudit("alert_rule.delete")); err != nil {
		t.Fatalf("DeleteAlertRule: %v", err)
	}
	if _, err := repository.AlertRule(ctx, rule.ID); !errors.Is(err, domain.ErrAlertRuleNotFound) {
		t.Fatalf("deleted rule read = %v, want ErrAlertRuleNotFound", err)
	}
	if got, _ := repository.Alert(ctx, second.ID); got.Resolution != domain.ResolutionRuleDeleted || got.RuleName != "data disk" {
		t.Fatalf("alert of the deleted rule = %+v", got)
	}

	if err := repository.DeleteAlertRule(ctx, uuid.MustParse("6f1c2a10-7a4e-4c4b-9a51-000000000001"), alertAt, alertAudit("alert_rule.delete")); !errors.Is(err, domain.ErrDefaultAlertRule) {
		t.Fatalf("deleting a default rule = %v, want ErrDefaultAlertRule", err)
	}
	if err := repository.DeleteAlertRule(ctx, uuid.New(), alertAt, alertAudit("alert_rule.delete")); !errors.Is(err, domain.ErrAlertRuleNotFound) {
		t.Fatalf("deleting an unknown rule = %v, want ErrAlertRuleNotFound", err)
	}
	if err := repository.UpdateAlertRule(ctx, customRule("ghost"), domain.ResolutionRuleChanged, alertAt, alertAudit("alert_rule.update")); !errors.Is(err, domain.ErrAlertRuleNotFound) {
		t.Fatalf("updating an unknown rule = %v, want ErrAlertRuleNotFound", err)
	}

	records, err := audits.ListAudit(ctx, domain.AuditFilter{Limit: 10})
	if err != nil || len(records) != 3 {
		t.Fatalf("audit = (%d records, %v), want create, update and delete", len(records), err)
	}
}

func TestAlertRulesAreBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repository := NewAlertRepository(openTestDatabase(t, ctx).Client())
	for i := 7; i < domain.MaxAlertRules; i++ {
		if err := repository.CreateAlertRule(ctx, customRule("rule"), alertAudit("alert_rule.create")); err != nil {
			t.Fatalf("rule %d: %v", i, err)
		}
	}
	if err := repository.CreateAlertRule(ctx, customRule("one too many"), alertAudit("alert_rule.create")); !errors.Is(err, domain.ErrTooManyAlertRules) {
		t.Fatalf("rule beyond the bound = %v, want ErrTooManyAlertRules", err)
	}
}

func TestOpenAlertsAreUniquePerRuleAndTarget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := NewAlertRepository(openTestDatabase(t, ctx).Client())
	rule := customRule("data disk")
	first := fired(rule, "host:x", alertAt)
	if err := repository.ApplyAlertEvaluation(ctx, []domain.Alert{first}, nil); err != nil {
		t.Fatalf("first firing: %v", err)
	}
	if err := repository.ApplyAlertEvaluation(ctx, []domain.Alert{fired(rule, "host:x", alertAt.Add(time.Minute))}, nil); err == nil {
		t.Fatal("a second open alert on the same rule and target was stored")
	}
	open, err := repository.OpenAlerts(ctx)
	if err != nil || len(open) != 1 || open[0].ID != first.ID || open[0].Target.Labels["mountpoint"] != "/" || *open[0].Value != 91 {
		t.Fatalf("OpenAlerts = (%+v, %v)", open, err)
	}
	// Resolving and firing in one evaluation commit together.
	next := fired(rule, "host:x", alertAt.Add(2*time.Minute))
	if err := repository.ApplyAlertEvaluation(ctx, []domain.Alert{next}, []domain.AlertResolutionChange{{ID: first.ID, Resolution: domain.ResolutionConditionCleared, At: alertAt.Add(2 * time.Minute)}}); err != nil {
		t.Fatalf("resolve and fire: %v", err)
	}
	if open, _ := repository.OpenAlerts(ctx); len(open) != 1 || open[0].ID != next.ID {
		t.Fatalf("open after resolve and fire = %+v", open)
	}
}

func TestAcknowledgeAlert(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := NewAlertRepository(openTestDatabase(t, ctx).Client())
	rule := customRule("data disk")
	alert := fired(rule, "host:x", alertAt)
	resolved := fired(rule, "host:y", alertAt)
	if err := repository.ApplyAlertEvaluation(ctx, []domain.Alert{alert, resolved}, nil); err != nil {
		t.Fatal(err)
	}
	if err := repository.ApplyAlertEvaluation(ctx, nil, []domain.AlertResolutionChange{{ID: resolved.ID, Resolution: domain.ResolutionConditionCleared, At: alertAt}}); err != nil {
		t.Fatal(err)
	}
	got, err := repository.AcknowledgeAlert(ctx, alert.ID, "alice", alertAt.Add(time.Minute), alertAudit("alert.acknowledge"))
	if err != nil || got.State != domain.AlertAcknowledged || got.AcknowledgedBy != "alice" || got.AcknowledgedAt == nil || !got.AcknowledgedAt.Equal(alertAt.Add(time.Minute)) {
		t.Fatalf("AcknowledgeAlert = (%+v, %v)", got, err)
	}
	again, err := repository.AcknowledgeAlert(ctx, alert.ID, "bob", alertAt.Add(2*time.Minute), alertAudit("alert.acknowledge"))
	if err != nil || again.AcknowledgedBy != "alice" {
		t.Fatalf("second acknowledgement = (%+v, %v), want unchanged", again, err)
	}
	if _, err := repository.AcknowledgeAlert(ctx, resolved.ID, "alice", alertAt, alertAudit("alert.acknowledge")); !errors.Is(err, domain.ErrAlertResolved) {
		t.Fatalf("acknowledging a resolved alert = %v, want ErrAlertResolved", err)
	}
	if _, err := repository.AcknowledgeAlert(ctx, uuid.New(), "alice", alertAt, alertAudit("alert.acknowledge")); !errors.Is(err, domain.ErrAlertNotFound) {
		t.Fatalf("acknowledging an unknown alert = %v, want ErrAlertNotFound", err)
	}
	if open, _ := repository.OpenAlerts(ctx); len(open) != 1 || open[0].State != domain.AlertAcknowledged {
		t.Fatalf("an acknowledged alert must stay open: %+v", open)
	}
}

func TestAlertHistorySummaryAndPurge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := NewAlertRepository(openTestDatabase(t, ctx).Client())
	rule := customRule("data disk")
	now := alertAt
	critical := rule
	critical.Severity = domain.AlertCritical
	open1 := fired(rule, "a", now.Add(-time.Hour))
	open2 := fired(critical, "b", now.Add(-30*time.Minute))
	recent := fired(rule, "c", now.Add(-3*time.Hour))
	old := fired(rule, "d", now.Add(-72*time.Hour))
	if err := repository.ApplyAlertEvaluation(ctx, []domain.Alert{open1, open2, recent, old}, nil); err != nil {
		t.Fatal(err)
	}
	if err := repository.ApplyAlertEvaluation(ctx, nil, []domain.AlertResolutionChange{
		{ID: recent.ID, Resolution: domain.ResolutionConditionCleared, At: now.Add(-2 * time.Hour)},
		{ID: old.ID, Resolution: domain.ResolutionConditionCleared, At: now.Add(-71 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}

	all, err := repository.Alerts(ctx, domain.AlertFilter{State: "all", Limit: 10})
	if err != nil || len(all) != 4 || all[0].ID != open2.ID || all[3].ID != old.ID {
		t.Fatalf("history = (%d alerts, %v), want 4, most recent first", len(all), err)
	}
	if open, _ := repository.Alerts(ctx, domain.AlertFilter{State: "open", Limit: 10}); len(open) != 2 {
		t.Fatalf("open history = %d, want 2", len(open))
	}
	if resolved, _ := repository.Alerts(ctx, domain.AlertFilter{State: "resolved", Limit: 1}); len(resolved) != 1 || resolved[0].ID != recent.ID {
		t.Fatalf("resolved history with limit 1 = %+v", resolved)
	}
	from, to := now.Add(-4*time.Hour), now.Add(-45*time.Minute)
	if window, _ := repository.Alerts(ctx, domain.AlertFilter{State: "all", From: &from, To: &to, Limit: 10}); len(window) != 2 {
		t.Fatalf("history in [-4 h, -45 min) = %d alerts, want 2", len(window))
	}

	summary, err := repository.AlertSummary(ctx, now)
	if err != nil || summary.Open[domain.AlertWarning] != 1 || summary.Open[domain.AlertCritical] != 1 || summary.Open[domain.AlertInfo] != 0 ||
		summary.ResolvedLast24h != 1 || summary.MeanResolutionSeconds24h == nil || *summary.MeanResolutionSeconds24h != 3600 {
		t.Fatalf("AlertSummary = (%+v, %v)", summary, err)
	}

	deleted, err := repository.DeleteResolvedAlertsBefore(ctx, now.Add(-24*time.Hour), 100)
	if err != nil || deleted != 1 {
		t.Fatalf("purge = (%d, %v), want the old resolved alert only", deleted, err)
	}
	if left, _ := repository.Alerts(ctx, domain.AlertFilter{State: "all", Limit: 10}); len(left) != 3 {
		t.Fatalf("after the purge %d alerts, want 3", len(left))
	}
	empty, err := NewAlertRepository(openTestDatabase(t, ctx).Client()).AlertSummary(ctx, now)
	if err != nil || empty.MeanResolutionSeconds24h != nil || empty.ResolvedLast24h != 0 {
		t.Fatalf("summary without alerts = (%+v, %v)", empty, err)
	}
}

func TestLatestContainerMetricsKeepsTheLatestFreshCycleOfEachContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := NewMonitoringRepository(openTestDatabase(t, ctx))
	now := alertAt
	if _, err := repository.ReconcileContainers(ctx, []domain.Container{
		{DockerID: "web-docker", Name: "web", Image: "nginx", State: "running", CreatedAt: now},
		{DockerID: "db-docker", Name: "db", Image: "postgres", State: "running", CreatedAt: now},
		{DockerID: "old-docker", Name: "old", Image: "busybox", State: "running", CreatedAt: now},
	}, now); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	ids := map[string]uuid.UUID{}
	for _, name := range []string{"web-docker", "db-docker", "old-docker"} {
		container, err := repository.GetContainer(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = container.ID
	}
	web, db, old := ids["web-docker"], ids["db-docker"], ids["old-docker"]
	if err := repository.SaveContainerMetrics(ctx, []domain.MetricSample{
		{ID: uuid.New(), ContainerID: &web, Name: "cpu.percent", Value: 1, Unit: "percent", RecordedAt: now.Add(-20 * time.Second)},
		{ID: uuid.New(), ContainerID: &web, Name: "cpu.percent", Value: 2, Unit: "percent", RecordedAt: now},
		{ID: uuid.New(), ContainerID: &web, Name: "network.errors", Value: 3, Unit: "count", Labels: map[string]string{"interface": "eth0"}, RecordedAt: now},
		{ID: uuid.New(), ContainerID: &db, Name: "cpu.percent", Value: 7, Unit: "percent", RecordedAt: now.Add(-10 * time.Second)},
		{ID: uuid.New(), ContainerID: &old, Name: "cpu.percent", Value: 9, Unit: "percent", RecordedAt: now.Add(-10 * time.Minute)},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	latest, err := repository.LatestContainerMetrics(ctx, now.Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("LatestContainerMetrics: %v", err)
	}
	if len(latest) != 2 || len(latest[web]) != 2 || len(latest[db]) != 1 || latest[old] != nil {
		t.Fatalf("latest = %+v, want web (2 samples) and db (1)", latest)
	}
	for _, s := range latest[web] {
		if s.Name == "cpu.percent" && s.Value != 2 {
			t.Errorf("web cpu = %v, want the latest cycle", s.Value)
		}
		if s.Name == "network.errors" && s.Labels["interface"] != "eth0" {
			t.Errorf("labels lost: %+v", s)
		}
		if s.ContainerID == nil || *s.ContainerID != web {
			t.Errorf("container id = %v", s.ContainerID)
		}
	}
}
