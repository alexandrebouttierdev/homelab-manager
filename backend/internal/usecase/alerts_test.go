package usecase

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// memoryAlerts stands for the alert tables.
type memoryAlerts struct {
	mu       sync.Mutex
	rules    []domain.AlertRule
	alerts   []domain.Alert
	audits   []domain.AuditEntry
	applyErr error
	purged   []time.Time
}

func (m *memoryAlerts) AlertRules(context.Context) ([]domain.AlertRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.rules), nil
}

func (m *memoryAlerts) AlertRule(_ context.Context, id uuid.UUID) (domain.AlertRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rule := range m.rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return domain.AlertRule{}, domain.ErrAlertRuleNotFound
}

func (m *memoryAlerts) CreateAlertRule(_ context.Context, rule domain.AlertRule, audit domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules = append(m.rules, rule)
	m.audits = append(m.audits, audit)
	return nil
}

func (m *memoryAlerts) resolveRule(id uuid.UUID, resolution domain.AlertResolution, at time.Time) {
	for i, alert := range m.alerts {
		if alert.RuleID == id && alert.Open() {
			m.alerts[i].State, m.alerts[i].Resolution, m.alerts[i].ResolvedAt = domain.AlertResolved, resolution, &at
		}
	}
}

func (m *memoryAlerts) UpdateAlertRule(_ context.Context, rule domain.AlertRule, resolution domain.AlertResolution, at time.Time, audit domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rules {
		if m.rules[i].ID == rule.ID {
			m.rules[i] = rule
			if resolution != "" {
				m.resolveRule(rule.ID, resolution, at)
			}
			m.audits = append(m.audits, audit)
			return nil
		}
	}
	return domain.ErrAlertRuleNotFound
}

func (m *memoryAlerts) DeleteAlertRule(_ context.Context, id uuid.UUID, at time.Time, audit domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, rule := range m.rules {
		if rule.ID == id {
			if rule.Default {
				return domain.ErrDefaultAlertRule
			}
			m.rules = slices.Delete(m.rules, i, i+1)
			m.resolveRule(id, domain.ResolutionRuleDeleted, at)
			m.audits = append(m.audits, audit)
			return nil
		}
	}
	return domain.ErrAlertRuleNotFound
}

func (m *memoryAlerts) OpenAlerts(context.Context) ([]domain.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var open []domain.Alert
	for _, alert := range m.alerts {
		if alert.Open() {
			open = append(open, alert)
		}
	}
	return open, nil
}

func (m *memoryAlerts) ApplyAlertEvaluation(_ context.Context, fired []domain.Alert, resolved []domain.AlertResolutionChange) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.applyErr != nil {
		return m.applyErr
	}
	for _, change := range resolved {
		for i := range m.alerts {
			if m.alerts[i].ID == change.ID && m.alerts[i].Open() {
				at := change.At
				m.alerts[i].State, m.alerts[i].Resolution, m.alerts[i].ResolvedAt = domain.AlertResolved, change.Resolution, &at
			}
		}
	}
	m.alerts = append(m.alerts, fired...)
	return nil
}

func (m *memoryAlerts) Alert(_ context.Context, id uuid.UUID) (domain.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, alert := range m.alerts {
		if alert.ID == id {
			return alert, nil
		}
	}
	return domain.Alert{}, domain.ErrAlertNotFound
}

func (m *memoryAlerts) Alerts(_ context.Context, filter domain.AlertFilter) ([]domain.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.alerts), nil
}

func (m *memoryAlerts) AcknowledgeAlert(_ context.Context, id uuid.UUID, by string, at time.Time, audit domain.AuditEntry) (domain.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.alerts {
		if m.alerts[i].ID == id {
			m.alerts[i].State, m.alerts[i].AcknowledgedBy, m.alerts[i].AcknowledgedAt = domain.AlertAcknowledged, by, &at
			m.audits = append(m.audits, audit)
			return m.alerts[i], nil
		}
	}
	return domain.Alert{}, domain.ErrAlertNotFound
}

func (m *memoryAlerts) AlertSummary(context.Context, time.Time) (domain.AlertSummary, error) {
	return domain.AlertSummary{}, nil
}

func (m *memoryAlerts) DeleteResolvedAlertsBefore(_ context.Context, before time.Time, _ int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purged = append(m.purged, before)
	return 0, nil
}

func (m *memoryAlerts) open() []domain.Alert {
	open, _ := m.OpenAlerts(context.Background())
	return open
}

// fakeAlertSources stands for the collections the engine reads.
type fakeAlertSources struct {
	mu               sync.Mutex
	host             []domain.MetricSample
	hostErr          error
	containers       []domain.Container
	containerMetrics map[uuid.UUID][]domain.MetricSample
	system, docker   string
	units            UnitInventory
	since            time.Time
}

func (f *fakeAlertSources) LatestMetrics(_ context.Context, containerID *string, since time.Time) ([]domain.MetricSample, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.since = since
	if f.hostErr != nil {
		return nil, f.hostErr
	}
	return slices.Clone(f.host), nil
}

func (f *fakeAlertSources) ListContainers(context.Context, bool, int) ([]domain.Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.containers), nil
}

func (f *fakeAlertSources) LatestContainerMetrics(context.Context, time.Time) (map[uuid.UUID][]domain.MetricSample, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.containerMetrics, nil
}

func (f *fakeAlertSources) Status() (domain.CollectorStatus, domain.CollectorStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return domain.CollectorStatus{Status: f.system}, domain.CollectorStatus{Status: f.docker}
}

func (f *fakeAlertSources) Inventory() UnitInventory {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.units
}

func (f *fakeAlertSources) Retention() time.Duration { return 7 * 24 * time.Hour }

func newAlertFixture(t *testing.T) (*AlertUsecase, *memoryAlerts, *fakeAlertSources, *time.Time) {
	t.Helper()
	collected := alertBase
	sources := &fakeAlertSources{system: "ok", docker: "ok", units: UnitInventory{Status: domain.UnitsOK, CollectedAt: &collected}}
	repo := &memoryAlerts{}
	now := alertBase
	usecase := NewAlertUsecase(repo, AlertSources{Metrics: sources, Collectors: sources, Units: sources, Retention: sources}, zap.NewNop())
	usecase.now = func() time.Time { return now }
	return usecase, repo, sources, &now
}

var alertBase = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

func diskRule(forSeconds int) domain.AlertRule {
	threshold := 85.0
	return domain.AlertRule{
		ID: uuid.New(), Name: "disk", Type: domain.AlertRuleHostMetric, Severity: domain.AlertWarning, Enabled: true,
		Params: domain.AlertRuleParams{Metric: "filesystem.percent", Operator: domain.OpGreater, Threshold: &threshold, ForSeconds: forSeconds},
	}
}

func diskSample(value float64) []domain.MetricSample {
	return []domain.MetricSample{{Name: "filesystem.percent", Value: value, Labels: map[string]string{"mountpoint": "/"}}}
}

func TestAlertEvaluationFiresOnceAndSurvivesARestart(t *testing.T) {
	usecase, repo, sources, now := newAlertFixture(t)
	rule := diskRule(30)
	repo.rules = []domain.AlertRule{rule}
	sources.host = diskSample(92)

	usecase.evaluate(context.Background())
	if len(repo.open()) != 0 {
		t.Fatal("fired before the duration")
	}
	if !sources.since.Equal(alertBase.Add(-5 * time.Minute)) {
		t.Fatalf("host metrics read since %v, want the 5 min window", sources.since)
	}
	*now = now.Add(30 * time.Second)
	usecase.evaluate(context.Background())
	if open := repo.open(); len(open) != 1 || open[0].RuleID != rule.ID {
		t.Fatalf("open = %+v, want one alert", open)
	}
	// A new instance (restart) reads the open alerts: no duplicate.
	restarted := NewAlertUsecase(repo, AlertSources{Metrics: sources, Collectors: sources, Units: sources, Retention: sources}, zap.NewNop())
	restarted.now = func() time.Time { return *now }
	restarted.evaluate(context.Background())
	if open := repo.open(); len(open) != 1 {
		t.Fatalf("restart duplicated the alert: %+v", open)
	}
	sources.host = diskSample(50)
	restarted.evaluate(context.Background())
	if open := repo.open(); len(open) != 0 {
		t.Fatalf("cleared condition left %+v open", open)
	}
}

func TestAlertEvaluationKeepsItsStateWhenTheWriteFails(t *testing.T) {
	usecase, repo, sources, now := newAlertFixture(t)
	repo.rules = []domain.AlertRule{diskRule(30)}
	sources.host = diskSample(92)
	usecase.evaluate(context.Background())
	*now = now.Add(30 * time.Second)
	repo.applyErr = errors.New("disk full")
	usecase.evaluate(context.Background())
	repo.applyErr = nil
	*now = now.Add(15 * time.Second)
	usecase.evaluate(context.Background())
	if len(repo.open()) != 1 {
		t.Fatal("the duration restarted after a failed write")
	}
}

func TestAlertSnapshotFreezesUnreadableSources(t *testing.T) {
	usecase, repo, sources, _ := newAlertFixture(t)
	web := uuid.New()
	down := domain.AlertRule{ID: uuid.New(), Name: "down", Type: domain.AlertRuleContainerState, Severity: domain.AlertWarning, Enabled: true, Params: domain.AlertRuleParams{States: []string{"exited"}}}
	units := domain.AlertRule{ID: uuid.New(), Name: "units", Type: domain.AlertRuleUnitFailed, Severity: domain.AlertCritical, Enabled: true, Params: domain.AlertRuleParams{UnitPattern: "*"}}
	host := diskRule(0)
	repo.rules = []domain.AlertRule{down, units, host}
	sources.containers = []domain.Container{{ID: web, Name: "web", State: "exited"}}
	sources.docker = "stale"
	sources.units = UnitInventory{Status: domain.UnitsUnavailable, Units: []domain.SystemUnit{{Name: "x.service", ActiveState: "failed"}}}
	sources.host = nil
	usecase.evaluate(context.Background())
	if open := repo.open(); len(open) != 0 {
		t.Fatalf("unreadable sources fired %+v", open)
	}
	sources.docker = "ok"
	collected := alertBase
	sources.units = UnitInventory{Status: domain.UnitsOK, CollectedAt: &collected, Units: []domain.SystemUnit{{Name: "x.service", ActiveState: "failed"}}}
	sources.host = diskSample(99)
	usecase.evaluate(context.Background())
	if open := repo.open(); len(open) != 3 {
		t.Fatalf("open = %+v, want container, unit and disk alerts", open)
	}
}

func TestAlertCollectorRulesReadEveryCollector(t *testing.T) {
	usecase, repo, sources, _ := newAlertFixture(t)
	rule := func(name string) domain.AlertRule {
		return domain.AlertRule{ID: uuid.New(), Name: name, Type: domain.AlertRuleCollector, Severity: domain.AlertWarning, Enabled: true, Params: domain.AlertRuleParams{Collector: name}}
	}
	repo.rules = []domain.AlertRule{rule("system"), rule("docker"), rule("services")}
	sources.system, sources.docker = "unavailable", "ok"
	sources.units = UnitInventory{Status: domain.UnitsError}
	usecase.evaluate(context.Background())
	got := map[string]string{}
	for _, alert := range repo.open() {
		got[alert.Target.Collector] = alert.Observed
	}
	if len(got) != 2 || got["system"] != "unavailable" || got["services"] != "error" {
		t.Fatalf("collector alerts = %v, want system and services", got)
	}
}

func TestCreateAlertRule(t *testing.T) {
	usecase, repo, _, _ := newAlertFixture(t)
	actor := domain.User{Username: "admin"}
	invalid := diskRule(0)
	invalid.Name = ""
	if _, err := usecase.CreateRule(context.Background(), actor, invalid, "req-1"); !errors.Is(err, domain.ErrInvalidAlertRule) {
		t.Fatalf("invalid rule = %v, want ErrInvalidAlertRule", err)
	}
	input := diskRule(60)
	input.ID = uuid.Nil
	input.Default = true
	created, err := usecase.CreateRule(context.Background(), actor, input, "req-2")
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if created.ID == uuid.Nil || created.Default || !created.CreatedAt.Equal(alertBase) || !created.UpdatedAt.Equal(alertBase) {
		t.Fatalf("created = %+v", created)
	}
	audit := repo.audits[0]
	if audit.Actor != "admin" || audit.TargetType != "alert_rule" || audit.TargetID != created.ID.String() || audit.Action != "create" || audit.RequestID != "req-2" || audit.Outcome != domain.AuditSuccess {
		t.Fatalf("audit = %+v", audit)
	}
}

func TestUpdateAlertRuleResolvesOnlyWhenTheEvaluationChanges(t *testing.T) {
	usecase, repo, sources, now := newAlertFixture(t)
	actor := domain.User{Username: "admin"}
	rule := diskRule(0)
	rule.Default = true
	repo.rules = []domain.AlertRule{rule}
	sources.host = diskSample(92)
	usecase.evaluate(context.Background())

	renamed := rule
	renamed.Name = "Disque"
	renamed.Default = false
	updated, err := usecase.UpdateRule(context.Background(), actor, rule.ID, renamed, "req")
	if err != nil || !updated.Default || updated.Name != "Disque" {
		t.Fatalf("UpdateRule = (%+v, %v), want the default flag kept", updated, err)
	}
	if len(repo.open()) != 1 {
		t.Fatal("a rename resolved the alert")
	}

	*now = now.Add(time.Minute)
	stricter := renamed
	threshold := 95.0
	stricter.Params.Threshold = &threshold
	if _, err := usecase.UpdateRule(context.Background(), actor, rule.ID, stricter, "req"); err != nil {
		t.Fatal(err)
	}
	if alerts, _ := repo.Alerts(context.Background(), domain.AlertFilter{}); alerts[0].Resolution != domain.ResolutionRuleChanged {
		t.Fatalf("alert after a threshold change = %+v", alerts[0])
	}

	usecase.evaluate(context.Background())
	disabled := stricter
	disabled.Enabled = false
	threshold = 50
	disabled.Params.Threshold = &threshold
	if _, err := usecase.UpdateRule(context.Background(), actor, rule.ID, disabled, "req"); err != nil {
		t.Fatal(err)
	}
	alerts, _ := repo.Alerts(context.Background(), domain.AlertFilter{})
	if len(alerts) != 1 || len(repo.open()) != 0 {
		t.Fatalf("alerts = %+v", alerts)
	}

	enabled := disabled
	enabled.Enabled = true
	if _, err := usecase.UpdateRule(context.Background(), actor, rule.ID, enabled, "req"); err != nil {
		t.Fatal(err)
	}
	usecase.evaluate(context.Background())
	if open := repo.open(); len(open) != 1 || *open[0].Threshold != 50 {
		t.Fatalf("open after enabling = %+v", open)
	}
	*now = now.Add(time.Minute)
	disabled.Name = "off"
	if _, err := usecase.UpdateRule(context.Background(), actor, rule.ID, disabled, "req"); err != nil {
		t.Fatal(err)
	}
	alerts, _ = repo.Alerts(context.Background(), domain.AlertFilter{})
	if last := alerts[len(alerts)-1]; last.Resolution != domain.ResolutionRuleDisabled {
		t.Fatalf("disabling resolved with %q, want rule_disabled", last.Resolution)
	}
	if _, err := usecase.UpdateRule(context.Background(), actor, uuid.New(), disabled, "req"); !errors.Is(err, domain.ErrAlertRuleNotFound) {
		t.Fatalf("unknown rule = %v", err)
	}
	invalid := disabled
	invalid.Severity = "fatal"
	if _, err := usecase.UpdateRule(context.Background(), actor, rule.ID, invalid, "req"); !errors.Is(err, domain.ErrInvalidAlertRule) {
		t.Fatalf("invalid update = %v", err)
	}
}

func TestDeleteAlertRuleAndAcknowledge(t *testing.T) {
	usecase, repo, sources, _ := newAlertFixture(t)
	actor := domain.User{Username: "alice"}
	rule := diskRule(0)
	repo.rules = []domain.AlertRule{rule}
	sources.host = diskSample(92)
	usecase.evaluate(context.Background())
	alert := repo.open()[0]
	acknowledged, err := usecase.Acknowledge(context.Background(), actor, alert.ID, "req")
	if err != nil || acknowledged.AcknowledgedBy != "alice" || acknowledged.State != domain.AlertAcknowledged {
		t.Fatalf("Acknowledge = (%+v, %v)", acknowledged, err)
	}
	if audit := repo.audits[len(repo.audits)-1]; audit.TargetType != "alert" || audit.TargetID != alert.ID.String() || audit.Action != "acknowledge" {
		t.Fatalf("audit = %+v", audit)
	}
	if err := usecase.DeleteRule(context.Background(), actor, rule.ID, "req"); err != nil {
		t.Fatal(err)
	}
	if len(repo.open()) != 0 || len(repo.rules) != 0 {
		t.Fatal("deleting the rule kept it or its alert")
	}
}

func TestAlertRunEvaluatesPurgesAndStops(t *testing.T) {
	usecase, repo, sources, _ := newAlertFixture(t)
	repo.rules = []domain.AlertRule{diskRule(0)}
	sources.host = diskSample(92)
	usecase.interval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		usecase.Run(ctx)
		close(done)
	}()
	deadline := time.After(5 * time.Second)
	for len(repo.open()) == 0 {
		select {
		case <-deadline:
			t.Fatal("Run did not evaluate")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.purged) == 0 || !repo.purged[0].Equal(alertBase.Add(-7*24*time.Hour)) {
		t.Fatalf("purges = %v, want one at the start with the retention", repo.purged)
	}
}
