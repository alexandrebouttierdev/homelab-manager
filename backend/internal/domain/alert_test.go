package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var alertNow = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

func float(v float64) *float64 { return &v }

func hostRule(metric string, labels map[string]string, op AlertOperator, threshold float64, forSeconds int) AlertRule {
	return AlertRule{
		ID: uuid.New(), Name: "rule " + metric, Type: AlertRuleHostMetric, Severity: AlertWarning, Enabled: true,
		Params: AlertRuleParams{Metric: metric, Labels: labels, Operator: op, Threshold: float(threshold), ForSeconds: forSeconds},
	}
}

func sample(name string, value float64, labels map[string]string) MetricSample {
	return MetricSample{Name: name, Value: value, Labels: labels, RecordedAt: alertNow}
}

func TestAlertRuleValidation(t *testing.T) {
	container := uuid.New()
	valid := []AlertRule{
		hostRule("filesystem.percent", map[string]string{"mountpoint": "/var/lib/docker"}, OpGreater, 85, 300),
		hostRule("memory.available_percent", nil, OpLess, 10, 300),
		hostRule("cpu.percent", map[string]string{"core": "all"}, OpGreaterOrEqual, 90, 0),
		{Name: "mem", Type: AlertRuleContainerMetric, Severity: AlertCritical, Params: AlertRuleParams{ContainerID: &container, Metric: "memory.percent", Operator: OpGreater, Threshold: float(90)}},
		{Name: "net", Type: AlertRuleContainerMetric, Severity: AlertInfo, Params: AlertRuleParams{Metric: "network.errors", Labels: map[string]string{"interface": "eth0"}, Operator: OpGreater, Threshold: float(0)}},
		{Name: "down", Type: AlertRuleContainerState, Severity: AlertWarning, Params: AlertRuleParams{States: []string{"exited", "restarting"}, ForSeconds: 120}},
		{Name: "units", Type: AlertRuleUnitFailed, Severity: AlertCritical, Params: AlertRuleParams{UnitPattern: "*"}},
		{Name: "one unit", Type: AlertRuleUnitFailed, Severity: AlertCritical, Params: AlertRuleParams{UnitPattern: "getty@tty[1-6].service"}},
		{Name: "docker", Type: AlertRuleCollector, Severity: AlertWarning, Params: AlertRuleParams{Collector: "docker", ForSeconds: 120}},
	}
	for _, rule := range valid {
		if err := rule.Validate(); err != nil {
			t.Errorf("Validate(%s %+v) = %v, want nil", rule.Type, rule.Params, err)
		}
	}
	invalid := map[string]AlertRule{
		"empty name":               {Name: " ", Type: AlertRuleCollector, Severity: AlertInfo, Params: AlertRuleParams{Collector: "docker"}},
		"long name":                {Name: strings.Repeat("a", 101), Type: AlertRuleCollector, Severity: AlertInfo, Params: AlertRuleParams{Collector: "docker"}},
		"control in name":          {Name: "a\nb", Type: AlertRuleCollector, Severity: AlertInfo, Params: AlertRuleParams{Collector: "docker"}},
		"unknown type":             {Name: "x", Type: "backup", Severity: AlertInfo},
		"unknown severity":         {Name: "x", Type: AlertRuleCollector, Severity: "error", Params: AlertRuleParams{Collector: "docker"}},
		"negative duration":        {Name: "x", Type: AlertRuleCollector, Severity: AlertInfo, Params: AlertRuleParams{Collector: "docker", ForSeconds: -1}},
		"duration over a day":      {Name: "x", Type: AlertRuleCollector, Severity: AlertInfo, Params: AlertRuleParams{Collector: "docker", ForSeconds: 86401}},
		"unknown collector":        {Name: "x", Type: AlertRuleCollector, Severity: AlertInfo, Params: AlertRuleParams{Collector: "journal"}},
		"collector with extra":     {Name: "x", Type: AlertRuleCollector, Severity: AlertInfo, Params: AlertRuleParams{Collector: "docker", Metric: "cpu.percent"}},
		"unknown host metric":      hostRule("cpu.cores", nil, OpGreater, 1, 0),
		"inventory metric":         hostRule("network.interface", nil, OpGreater, 1, 0),
		"container metric on host": hostRule("memory.usage_bytes", nil, OpGreater, 1, 0),
		"label not of metric":      hostRule("filesystem.percent", map[string]string{"sensor": "x"}, OpGreater, 1, 0),
		"too many labels":          hostRule("filesystem.percent", map[string]string{"mountpoint": "/", "device": "a", "x": "", "y": "", "z": "", "w": ""}, OpGreater, 1, 0),
		"empty label value":        hostRule("filesystem.percent", map[string]string{"mountpoint": ""}, OpGreater, 1, 0),
		"bad operator":             hostRule("cpu.percent", nil, "==", 1, 0),
		"infinite threshold":       hostRule("cpu.percent", nil, OpGreater, math.Inf(1), 0),
		"nan threshold":            hostRule("cpu.percent", nil, OpGreater, math.NaN(), 0),
		"missing threshold":        {Name: "x", Type: AlertRuleHostMetric, Severity: AlertInfo, Params: AlertRuleParams{Metric: "cpu.percent", Operator: OpGreater}},
		"host rule with container": {Name: "x", Type: AlertRuleHostMetric, Severity: AlertInfo, Params: AlertRuleParams{Metric: "cpu.percent", Operator: OpGreater, Threshold: float(1), ContainerID: &container}},
		"unknown container metric": {Name: "x", Type: AlertRuleContainerMetric, Severity: AlertInfo, Params: AlertRuleParams{Metric: "filesystem.percent", Operator: OpGreater, Threshold: float(1)}},
		"no states":                {Name: "x", Type: AlertRuleContainerState, Severity: AlertInfo},
		"unknown state":            {Name: "x", Type: AlertRuleContainerState, Severity: AlertInfo, Params: AlertRuleParams{States: []string{"sleeping"}}},
		"duplicate state":          {Name: "x", Type: AlertRuleContainerState, Severity: AlertInfo, Params: AlertRuleParams{States: []string{"exited", "exited"}}},
		"state with threshold":     {Name: "x", Type: AlertRuleContainerState, Severity: AlertInfo, Params: AlertRuleParams{States: []string{"exited"}, Threshold: float(1)}},
		"empty pattern":            {Name: "x", Type: AlertRuleUnitFailed, Severity: AlertInfo},
		"pattern with slash":       {Name: "x", Type: AlertRuleUnitFailed, Severity: AlertInfo, Params: AlertRuleParams{UnitPattern: "../x.service"}},
		"malformed pattern":        {Name: "x", Type: AlertRuleUnitFailed, Severity: AlertInfo, Params: AlertRuleParams{UnitPattern: "ssh[.service"}},
		"unit with duration":       {Name: "x", Type: AlertRuleUnitFailed, Severity: AlertInfo, Params: AlertRuleParams{UnitPattern: "*", ForSeconds: 30}},
	}
	for name, rule := range invalid {
		if err := rule.Validate(); !errors.Is(err, ErrInvalidAlertRule) {
			t.Errorf("%s: Validate = %v, want ErrInvalidAlertRule", name, err)
		}
	}
}

func TestEvaluateFiresAfterTheDurationOnly(t *testing.T) {
	rule := hostRule("filesystem.percent", nil, OpGreater, 85, 300)
	snapshot := AlertSnapshot{HostMetrics: []MetricSample{
		sample("filesystem.percent", 91.5, map[string]string{"mountpoint": "/", "device": "sda1"}),
		sample("filesystem.percent", 40, map[string]string{"mountpoint": "/boot", "device": "sda2"}),
	}}
	state := AlertEngineState{}
	result := EvaluateAlerts([]AlertRule{rule}, snapshot, nil, state, alertNow)
	if len(result.Fired) != 0 || len(result.State.Pending) != 1 {
		t.Fatalf("first evaluation fired %d, pending %d; want 0 fired, 1 pending", len(result.Fired), len(result.State.Pending))
	}
	result = EvaluateAlerts([]AlertRule{rule}, snapshot, nil, result.State, alertNow.Add(299*time.Second))
	if len(result.Fired) != 0 {
		t.Fatalf("fired before the duration: %+v", result.Fired)
	}
	result = EvaluateAlerts([]AlertRule{rule}, snapshot, nil, result.State, alertNow.Add(300*time.Second))
	if len(result.Fired) != 1 {
		t.Fatalf("fired %d alerts at the duration, want 1", len(result.Fired))
	}
	alert := result.Fired[0]
	if alert.RuleID != rule.ID || alert.State != AlertFiring || alert.Severity != AlertWarning ||
		alert.TargetKey != "host:filesystem.percent{device=sda1,mountpoint=/}" ||
		alert.Value == nil || *alert.Value != 91.5 || alert.Threshold == nil || *alert.Threshold != 85 ||
		!alert.FiredAt.Equal(alertNow.Add(300*time.Second)) || alert.Target.Kind != AlertTargetHost ||
		alert.Target.Labels["mountpoint"] != "/" || alert.RuleName != rule.Name {
		t.Fatalf("fired alert = %+v", alert)
	}
	if len(result.State.Pending) != 0 {
		t.Fatalf("pending kept after firing: %v", result.State.Pending)
	}
}

func TestEvaluateRestartsTheDurationWhenTheConditionBreaks(t *testing.T) {
	rule := hostRule("cpu.percent", map[string]string{"core": "all"}, OpGreaterOrEqual, 90, 60)
	high := AlertSnapshot{HostMetrics: []MetricSample{sample("cpu.percent", 95, map[string]string{"core": "all"}), sample("cpu.percent", 99, map[string]string{"core": "0"})}}
	low := AlertSnapshot{HostMetrics: []MetricSample{sample("cpu.percent", 50, map[string]string{"core": "all"})}}
	result := EvaluateAlerts([]AlertRule{rule}, high, nil, AlertEngineState{}, alertNow)
	result = EvaluateAlerts([]AlertRule{rule}, low, nil, result.State, alertNow.Add(30*time.Second))
	result = EvaluateAlerts([]AlertRule{rule}, high, nil, result.State, alertNow.Add(60*time.Second))
	if len(result.Fired) != 0 {
		t.Fatalf("fired although the condition broke: %+v", result.Fired)
	}
	result = EvaluateAlerts([]AlertRule{rule}, high, nil, result.State, alertNow.Add(120*time.Second))
	if len(result.Fired) != 1 || result.Fired[0].Target.Labels["core"] != "all" {
		t.Fatalf("fired = %+v, want one alert on core all", result.Fired)
	}
}

func TestEvaluateImmediateRuleAndOperators(t *testing.T) {
	for _, tc := range []struct {
		op    AlertOperator
		value float64
		fires bool
	}{
		{OpGreater, 10, false}, {OpGreater, 10.1, true},
		{OpGreaterOrEqual, 10, true}, {OpGreaterOrEqual, 9.9, false},
		{OpLess, 10, false}, {OpLess, 9.9, true},
		{OpLessOrEqual, 10, true}, {OpLessOrEqual, 10.1, false},
	} {
		rule := hostRule("cpu.load1", nil, tc.op, 10, 0)
		snapshot := AlertSnapshot{HostMetrics: []MetricSample{sample("cpu.load1", tc.value, nil)}}
		result := EvaluateAlerts([]AlertRule{rule}, snapshot, nil, AlertEngineState{}, alertNow)
		if got := len(result.Fired) == 1; got != tc.fires {
			t.Errorf("%v %s 10: fired %v, want %v", tc.value, tc.op, got, tc.fires)
		}
	}
}

func TestEvaluateDerivesAvailableMemoryPercent(t *testing.T) {
	rule := hostRule("memory.available_percent", nil, OpLess, 10, 0)
	snapshot := AlertSnapshot{HostMetrics: []MetricSample{sample("memory.available", 512, nil), sample("memory.total", 8192, nil)}}
	result := EvaluateAlerts([]AlertRule{rule}, snapshot, nil, AlertEngineState{}, alertNow)
	if len(result.Fired) != 1 || *result.Fired[0].Value != 6.25 {
		t.Fatalf("fired = %+v, want one alert at 6.25 %%", result.Fired)
	}
	noTotal := AlertSnapshot{HostMetrics: []MetricSample{sample("memory.available", 512, nil), sample("memory.total", 0, nil)}}
	if result := EvaluateAlerts([]AlertRule{rule}, noTotal, nil, AlertEngineState{}, alertNow); len(result.Fired) != 0 {
		t.Fatalf("fired without a total: %+v", result.Fired)
	}
}

func openAlert(rule AlertRule, targetKey string, target AlertTarget) Alert {
	return Alert{
		ID: uuid.New(), RuleID: rule.ID, RuleName: rule.Name, RuleType: rule.Type, Severity: rule.Severity,
		TargetKey: targetKey, Target: target, State: AlertFiring, FiredAt: alertNow.Add(-time.Hour),
	}
}

func TestEvaluateResolvesAClearedConditionAndDoesNotRefireAnOpenAlert(t *testing.T) {
	rule := hostRule("cpu.load1", nil, OpGreater, 10, 0)
	open := openAlert(rule, "host:cpu.load1{}", AlertTarget{Kind: AlertTargetHost, Metric: "cpu.load1"})
	stillHigh := AlertSnapshot{HostMetrics: []MetricSample{sample("cpu.load1", 20, nil)}}
	result := EvaluateAlerts([]AlertRule{rule}, stillHigh, []Alert{open}, AlertEngineState{}, alertNow)
	if len(result.Fired) != 0 || len(result.Resolved) != 0 {
		t.Fatalf("open alert changed: fired %+v resolved %+v", result.Fired, result.Resolved)
	}
	acknowledged := open
	acknowledged.State = AlertAcknowledged
	low := AlertSnapshot{HostMetrics: []MetricSample{sample("cpu.load1", 2, nil)}}
	result = EvaluateAlerts([]AlertRule{rule}, low, []Alert{acknowledged}, AlertEngineState{}, alertNow)
	if len(result.Resolved) != 1 || result.Resolved[0].ID != open.ID || result.Resolved[0].Resolution != ResolutionConditionCleared || !result.Resolved[0].At.Equal(alertNow) {
		t.Fatalf("resolved = %+v, want the acknowledged alert cleared", result.Resolved)
	}
}

func TestEvaluateFreezesWhenTheSourceIsUnavailable(t *testing.T) {
	host := hostRule("cpu.load1", nil, OpGreater, 10, 0)
	containers := AlertRule{ID: uuid.New(), Name: "down", Type: AlertRuleContainerState, Severity: AlertWarning, Enabled: true, Params: AlertRuleParams{States: []string{"exited"}}}
	units := AlertRule{ID: uuid.New(), Name: "units", Type: AlertRuleUnitFailed, Severity: AlertCritical, Enabled: true, Params: AlertRuleParams{UnitPattern: "*"}}
	id := uuid.New()
	open := []Alert{
		openAlert(host, "host:cpu.load1{}", AlertTarget{Kind: AlertTargetHost, Metric: "cpu.load1"}),
		openAlert(containers, "container:"+id.String(), AlertTarget{Kind: AlertTargetContainer, ContainerID: &id}),
		openAlert(units, "unit:ssh.service", AlertTarget{Kind: AlertTargetUnit, Unit: "ssh.service"}),
	}
	pending := AlertEngineState{Pending: map[string]time.Time{host.ID.String() + "|host:cpu.load5{}": alertNow.Add(-time.Minute)}}
	result := EvaluateAlerts([]AlertRule{host, containers, units}, AlertSnapshot{}, open, pending, alertNow.Add(time.Hour))
	if len(result.Fired) != 0 || len(result.Resolved) != 0 {
		t.Fatalf("frozen sources changed alerts: fired %+v resolved %+v", result.Fired, result.Resolved)
	}
	if len(result.State.Pending) != 1 {
		t.Fatalf("pending of a frozen rule lost: %v", result.State.Pending)
	}
}

func TestEvaluateResolvesAMissingSeriesAfterFiveMinutes(t *testing.T) {
	rule := hostRule("filesystem.percent", nil, OpGreater, 85, 0)
	key := "host:filesystem.percent{mountpoint=/mnt/usb}"
	open := openAlert(rule, key, AlertTarget{Kind: AlertTargetHost, Metric: "filesystem.percent", Labels: map[string]string{"mountpoint": "/mnt/usb"}})
	cycle := AlertSnapshot{HostMetrics: []MetricSample{sample("filesystem.percent", 40, map[string]string{"mountpoint": "/"})}}
	result := EvaluateAlerts([]AlertRule{rule}, cycle, []Alert{open}, AlertEngineState{}, alertNow)
	if len(result.Resolved) != 0 {
		t.Fatalf("resolved at once: %+v", result.Resolved)
	}
	result = EvaluateAlerts([]AlertRule{rule}, cycle, []Alert{open}, result.State, alertNow.Add(MissingDataGrace-time.Second))
	if len(result.Resolved) != 0 {
		t.Fatalf("resolved before the grace: %+v", result.Resolved)
	}
	result = EvaluateAlerts([]AlertRule{rule}, cycle, []Alert{open}, result.State, alertNow.Add(MissingDataGrace))
	if len(result.Resolved) != 1 || result.Resolved[0].Resolution != ResolutionNoData {
		t.Fatalf("resolved = %+v, want no_data after the grace", result.Resolved)
	}
	// The series coming back before the grace keeps the alert open.
	result = EvaluateAlerts([]AlertRule{rule}, cycle, []Alert{open}, AlertEngineState{}, alertNow)
	back := AlertSnapshot{HostMetrics: []MetricSample{sample("filesystem.percent", 95, map[string]string{"mountpoint": "/mnt/usb"})}}
	result = EvaluateAlerts([]AlertRule{rule}, back, []Alert{open}, result.State, alertNow.Add(time.Minute))
	result = EvaluateAlerts([]AlertRule{rule}, cycle, []Alert{open}, result.State, alertNow.Add(MissingDataGrace))
	if len(result.Resolved) != 0 {
		t.Fatalf("grace not restarted after the series came back: %+v", result.Resolved)
	}
}

func TestEvaluateContainerRules(t *testing.T) {
	web, db := uuid.New(), uuid.New()
	snapshot := AlertSnapshot{
		ContainersKnown: true,
		Containers:      []Container{{ID: web, Name: "web", State: "running"}, {ID: db, Name: "db", State: "exited"}},
		ContainerMetrics: map[uuid.UUID][]MetricSample{
			web: {sample("memory.percent", 95, nil), sample("network.errors", 3, map[string]string{"interface": "eth0"})},
		},
	}
	memory := AlertRule{ID: uuid.New(), Name: "mem", Type: AlertRuleContainerMetric, Severity: AlertCritical, Enabled: true, Params: AlertRuleParams{Metric: "memory.percent", Operator: OpGreater, Threshold: float(90)}}
	state := AlertRule{ID: uuid.New(), Name: "down", Type: AlertRuleContainerState, Severity: AlertWarning, Enabled: true, Params: AlertRuleParams{States: []string{"exited", "dead"}}}
	onlyWeb := AlertRule{ID: uuid.New(), Name: "web down", Type: AlertRuleContainerState, Severity: AlertWarning, Enabled: true, Params: AlertRuleParams{ContainerID: &web, States: []string{"exited"}}}
	result := EvaluateAlerts([]AlertRule{memory, state, onlyWeb}, snapshot, nil, AlertEngineState{}, alertNow)
	if len(result.Fired) != 2 {
		t.Fatalf("fired %d alerts, want 2: %+v", len(result.Fired), result.Fired)
	}
	byRule := map[uuid.UUID]Alert{}
	for _, alert := range result.Fired {
		byRule[alert.RuleID] = alert
	}
	if got := byRule[memory.ID]; got.TargetKey != "container:"+web.String()+":memory.percent{}" || got.Target.ContainerName != "web" || *got.Value != 95 {
		t.Errorf("memory alert = %+v", got)
	}
	if got := byRule[state.ID]; got.TargetKey != "container:"+db.String() || got.Observed != "exited" || got.Value != nil || got.Target.ContainerName != "db" {
		t.Errorf("state alert = %+v", got)
	}
	// A removed container resolves its alerts at once.
	open := []Alert{byRule[memory.ID], byRule[state.ID]}
	gone := AlertSnapshot{ContainersKnown: true}
	result = EvaluateAlerts([]AlertRule{memory, state}, gone, open, AlertEngineState{}, alertNow.Add(time.Minute))
	if len(result.Resolved) != 2 || result.Resolved[0].Resolution != ResolutionTargetGone || result.Resolved[1].Resolution != ResolutionTargetGone {
		t.Fatalf("resolved = %+v, want two target_gone", result.Resolved)
	}
}

func TestEvaluateUnitRules(t *testing.T) {
	rule := AlertRule{ID: uuid.New(), Name: "units", Type: AlertRuleUnitFailed, Severity: AlertCritical, Enabled: true, Params: AlertRuleParams{UnitPattern: "backup-*.service"}}
	snapshot := AlertSnapshot{UnitsKnown: true, Units: []SystemUnit{
		{Name: "backup-db.service", ActiveState: "failed", Result: "exit-code"},
		{Name: "backup-web.service", ActiveState: "active"},
		{Name: "ssh.service", ActiveState: "failed"},
	}}
	result := EvaluateAlerts([]AlertRule{rule}, snapshot, nil, AlertEngineState{}, alertNow)
	if len(result.Fired) != 1 || result.Fired[0].TargetKey != "unit:backup-db.service" || result.Fired[0].Observed != "failed (exit-code)" || result.Fired[0].Target.Unit != "backup-db.service" {
		t.Fatalf("fired = %+v, want backup-db.service only", result.Fired)
	}
	open := result.Fired
	recovered := AlertSnapshot{UnitsKnown: true, Units: []SystemUnit{{Name: "backup-db.service", ActiveState: "active"}}}
	if result := EvaluateAlerts([]AlertRule{rule}, recovered, open, AlertEngineState{}, alertNow); len(result.Resolved) != 1 || result.Resolved[0].Resolution != ResolutionConditionCleared {
		t.Fatalf("resolved = %+v, want condition_cleared", result.Resolved)
	}
	unloaded := AlertSnapshot{UnitsKnown: true}
	if result := EvaluateAlerts([]AlertRule{rule}, unloaded, open, AlertEngineState{}, alertNow); len(result.Resolved) != 1 || result.Resolved[0].Resolution != ResolutionTargetGone {
		t.Fatalf("resolved = %+v, want target_gone", result.Resolved)
	}
}

func TestEvaluateCollectorRules(t *testing.T) {
	rule := AlertRule{ID: uuid.New(), Name: "docker", Type: AlertRuleCollector, Severity: AlertWarning, Enabled: true, Params: AlertRuleParams{Collector: "docker", ForSeconds: 120}}
	for status, fires := range map[string]bool{"ok": false, "starting": false, "pending": false, "stale": true, "unavailable": true, "error": true} {
		snapshot := AlertSnapshot{Collectors: map[string]string{"docker": status}}
		result := EvaluateAlerts([]AlertRule{rule}, snapshot, nil, AlertEngineState{}, alertNow)
		result = EvaluateAlerts([]AlertRule{rule}, snapshot, nil, result.State, alertNow.Add(2*time.Minute))
		if got := len(result.Fired) == 1; got != fires {
			t.Errorf("docker %s: fired %v, want %v", status, got, fires)
		}
		if fires && (result.Fired[0].TargetKey != "collector:docker" || result.Fired[0].Observed != status) {
			t.Errorf("docker %s: alert %+v", status, result.Fired[0])
		}
	}
}

func TestEvaluateResolvesAlertsOfDisabledAndDeletedRules(t *testing.T) {
	disabled := hostRule("cpu.load1", nil, OpGreater, 10, 0)
	disabled.Enabled = false
	deleted := hostRule("cpu.load5", nil, OpGreater, 10, 0)
	open := []Alert{
		openAlert(disabled, "host:cpu.load1{}", AlertTarget{Kind: AlertTargetHost}),
		openAlert(deleted, "host:cpu.load5{}", AlertTarget{Kind: AlertTargetHost}),
	}
	snapshot := AlertSnapshot{HostMetrics: []MetricSample{sample("cpu.load1", 20, nil), sample("cpu.load5", 20, nil)}}
	result := EvaluateAlerts([]AlertRule{disabled}, snapshot, open, AlertEngineState{}, alertNow)
	if len(result.Fired) != 0 || len(result.Resolved) != 2 {
		t.Fatalf("fired %+v resolved %+v", result.Fired, result.Resolved)
	}
	got := map[uuid.UUID]AlertResolution{}
	for _, r := range result.Resolved {
		got[r.ID] = r.Resolution
	}
	if got[open[0].ID] != ResolutionRuleDisabled || got[open[1].ID] != ResolutionRuleDeleted {
		t.Fatalf("resolutions = %v", got)
	}
}

func TestEvaluateDropsStateOfRulesThatNoLongerApply(t *testing.T) {
	rule := hostRule("cpu.load1", nil, OpGreater, 10, 600)
	state := AlertEngineState{
		Pending: map[string]time.Time{uuid.NewString() + "|host:cpu.load1{}": alertNow},
		Missing: map[string]time.Time{uuid.NewString() + "|host:cpu.load1{}": alertNow},
	}
	snapshot := AlertSnapshot{HostMetrics: []MetricSample{sample("cpu.load1", 20, nil)}}
	result := EvaluateAlerts([]AlertRule{rule}, snapshot, nil, state, alertNow)
	if len(result.State.Pending) != 1 || len(result.State.Missing) != 0 {
		t.Fatalf("state = %+v, want only the pending condition of the rule", result.State)
	}
	if _, ok := result.State.Pending[rule.ID.String()+"|host:cpu.load1{}"]; !ok {
		t.Fatalf("pending of the rule missing: %v", result.State.Pending)
	}
}

func TestRoleAlertPermissions(t *testing.T) {
	for _, tc := range []struct {
		role            Role
		operate, manage bool
	}{
		{RoleViewer, false, false},
		{RoleOperator, true, false},
		{RoleAdmin, true, true},
	} {
		if tc.role.Can(PermOperateAlerts) != tc.operate || tc.role.Can(PermManageAlerts) != tc.manage {
			t.Errorf("%s: operate %v manage %v", tc.role, tc.role.Can(PermOperateAlerts), tc.role.Can(PermManageAlerts))
		}
	}
}

func TestVulnerabilitiesRule(t *testing.T) {
	rule := AlertRule{ID: uuid.New(), Name: "Vulnérabilités critiques", Type: AlertRuleVulnerabilities, Severity: AlertCritical, Enabled: true,
		Params: AlertRuleParams{VulnSeverity: VulnHigh, FixableOnly: true}}
	if err := rule.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	for name, params := range map[string]AlertRuleParams{
		"no severity":       {},
		"negligible":        {VulnSeverity: VulnNegligible},
		"with a duration":   {VulnSeverity: VulnHigh, ForSeconds: 60},
		"with a metric":     {VulnSeverity: VulnHigh, Metric: "cpu.percent"},
		"vuln on collector": {Collector: "docker", VulnSeverity: VulnHigh},
	} {
		invalid := rule
		invalid.Params = params
		if name == "vuln on collector" {
			invalid.Type = AlertRuleCollector
		}
		if err := invalid.Validate(); !errors.Is(err, ErrInvalidAlertRule) {
			t.Errorf("%s: Validate() = %v", name, err)
		}
	}
	counts := []VulnerabilityCount{
		{Severity: VulnCritical, Fixable: false, Count: 2},
		{Severity: VulnHigh, Fixable: true, Count: 3},
		{Severity: VulnMedium, Fixable: true, Count: 7},
	}
	frozen := EvaluateAlerts([]AlertRule{rule}, AlertSnapshot{}, nil, AlertEngineState{}, alertNow)
	if len(frozen.Fired) != 0 {
		t.Fatal("a vulnerabilities rule fired before any successful scan")
	}
	result := EvaluateAlerts([]AlertRule{rule}, AlertSnapshot{VulnerabilitiesKnown: true, VulnerabilityCounts: counts}, nil, AlertEngineState{}, alertNow)
	if len(result.Fired) != 1 || *result.Fired[0].Value != 3 || result.Fired[0].Target.Kind != AlertTargetVulnerabilities || result.Fired[0].TargetKey != "vulnerabilities" {
		t.Fatalf("fired = %+v, want one alert counting the 3 fixable high findings", result.Fired)
	}
	rule.Params.FixableOnly = false
	result = EvaluateAlerts([]AlertRule{rule}, AlertSnapshot{VulnerabilitiesKnown: true, VulnerabilityCounts: counts}, nil, AlertEngineState{}, alertNow)
	if *result.Fired[0].Value != 5 {
		t.Fatalf("count = %v, want 5 critical and high findings", *result.Fired[0].Value)
	}
	open := result.Fired
	cleared := EvaluateAlerts([]AlertRule{rule}, AlertSnapshot{VulnerabilitiesKnown: true}, open, AlertEngineState{}, alertNow.Add(time.Hour))
	if len(cleared.Resolved) != 1 || cleared.Resolved[0].Resolution != ResolutionConditionCleared {
		t.Fatalf("resolved = %+v", cleared.Resolved)
	}
	message := RenderNotification("nas", NotificationBatch{Notifications: []Notification{{Event: NotifyFiring, Alert: open[0]}}}, alertNow, 0)
	if !strings.Contains(message.Body, "Paquets de la machine · 5 vulnérabilités") {
		t.Fatalf("message = %q", message.Body)
	}
}
