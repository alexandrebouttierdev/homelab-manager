package domain

import (
	"errors"
	"fmt"
	"math"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Bounds of the alerts module (SPEC_ADMINISTRATION.md § 6).
const (
	// AlertEvaluationInterval is the period of the rule engine.
	AlertEvaluationInterval = 15 * time.Second
	// MaxAlertRules bounds the rules an administrator can create.
	MaxAlertRules = 200
	// MaxAlertDuration bounds how long a condition must hold.
	MaxAlertDuration = 86400
	// MissingDataGrace is how long a series may be absent from a fresh
	// cycle before its open alert is resolved for lack of data.
	MissingDataGrace = 5 * time.Minute
	// DefaultAlertLimit and MaxAlertLimit bound an alert history query.
	DefaultAlertLimit = 200
	MaxAlertLimit     = 1000
	maxRuleNameBytes  = 100
	maxRuleLabels     = 5
	maxLabelBytes     = 255
	maxPatternBytes   = 255
)

var (
	ErrInvalidAlertRule    = errors.New("alert rule is invalid")
	ErrAlertRuleNotFound   = errors.New("alert rule not found")
	ErrAlertNotFound       = errors.New("alert not found")
	ErrDefaultAlertRule    = errors.New("a default alert rule cannot be deleted")
	ErrTooManyAlertRules   = errors.New("too many alert rules")
	ErrAlertResolved       = errors.New("alert is already resolved")
	ErrInvalidAlertFilter  = errors.New("alert filter values are out of bounds")
	unitPatternCharacters  = regexp.MustCompile(`^[A-Za-z0-9@._:\-*?\[\]]+$`)
	containerStates        = []string{"created", "running", "paused", "restarting", "removing", "exited", "dead"}
	alertCollectors        = []string{"system", "docker", "services"}
	healthyCollectorStates = []string{"ok", "starting", "pending"}
)

// AlertSeverity is the severity of a rule and of its alerts.
type AlertSeverity string

const (
	AlertInfo     AlertSeverity = "info"
	AlertWarning  AlertSeverity = "warning"
	AlertCritical AlertSeverity = "critical"
)

// AlertSeverities lists the severities, least severe first.
var AlertSeverities = []AlertSeverity{AlertInfo, AlertWarning, AlertCritical}

// AlertRuleType is one of the rule types of SPEC_ADMINISTRATION.md § 6.1.
// The backup rule comes with the backups module, which has its data.
type AlertRuleType string

const (
	AlertRuleHostMetric      AlertRuleType = "host_metric"
	AlertRuleContainerMetric AlertRuleType = "container_metric"
	AlertRuleContainerState  AlertRuleType = "container_state"
	AlertRuleUnitFailed      AlertRuleType = "unit_failed"
	AlertRuleCollector       AlertRuleType = "collector"
)

// AlertOperator compares a metric value with the threshold.
type AlertOperator string

const (
	OpGreater        AlertOperator = ">"
	OpGreaterOrEqual AlertOperator = ">="
	OpLess           AlertOperator = "<"
	OpLessOrEqual    AlertOperator = "<="
)

// DerivedMemoryAvailablePercent is computed by the engine from
// memory.available and memory.total of the host cycle.
const DerivedMemoryAvailablePercent = "memory.available_percent"

// hostAlertMetrics maps the host metrics a rule can watch to the labels
// they carry (API_CONTRACTS.md § 5.4). Cumulative counters, totals and the
// interface inventory are left out: a threshold on them means nothing.
var hostAlertMetrics = map[string][]string{
	"cpu.percent": {"core"}, "cpu.load1": nil, "cpu.load5": nil, "cpu.load15": nil,
	"memory.used": nil, "memory.available": nil, "memory.cached": nil, DerivedMemoryAvailablePercent: nil,
	"swap.used":       nil,
	"filesystem.used": {"mountpoint", "device"}, "filesystem.available": {"mountpoint", "device"}, "filesystem.percent": {"mountpoint", "device"},
	"disk.read_iops": {"device"}, "disk.write_iops": {"device"}, "disk.read_latency_ms": {"device"}, "disk.write_latency_ms": {"device"},
	"network.rx_bytes_per_second": {"interface"}, "network.tx_bytes_per_second": {"interface"},
	"network.errors": {"interface"}, "network.drops": {"interface"},
	"temperature.celsius": {"sensor"},
}

// containerAlertMetrics is the same list for containers (§ 5.5).
var containerAlertMetrics = map[string][]string{
	"cpu.percent": nil, "memory.usage_bytes": nil, "memory.percent": nil, "memory.available_bytes": nil,
	"network.rx_bytes_per_second": {"interface"}, "network.tx_bytes_per_second": {"interface"},
	"network.errors": {"interface"}, "network.drops": {"interface"},
	"process.count": nil, "restart.count": nil,
}

// HostAlertMetrics and ContainerAlertMetrics expose the metric lists
// with their labels, for the interface.
func HostAlertMetrics() map[string][]string      { return cloneMetricList(hostAlertMetrics) }
func ContainerAlertMetrics() map[string][]string { return cloneMetricList(containerAlertMetrics) }

// AlertContainerStates and AlertCollectors list the values a container
// state rule and a collector rule accept.
func AlertContainerStates() []string { return slices.Clone(containerStates) }
func AlertCollectors() []string      { return slices.Clone(alertCollectors) }

func cloneMetricList(list map[string][]string) map[string][]string {
	out := make(map[string][]string, len(list))
	for name, labels := range list {
		out[name] = append([]string{}, labels...)
	}
	return out
}

// AlertRule is a rule of the engine. Default rules are seeded by the
// migration: they can be changed and disabled, not deleted.
type AlertRule struct {
	ID        uuid.UUID
	Name      string
	Type      AlertRuleType
	Severity  AlertSeverity
	Enabled   bool
	Default   bool
	Params    AlertRuleParams
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AlertRuleParams holds the parameters of every type; each type uses its
// own and leaves the others empty.
type AlertRuleParams struct {
	// Metric, Labels, Operator and Threshold: metric rules. Labels keep the
	// series carrying all of these values.
	Metric    string            `json:"metric,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Operator  AlertOperator     `json:"operator,omitempty"`
	Threshold *float64          `json:"threshold,omitempty"`
	// ContainerID: container rules, nil for every container.
	ContainerID *uuid.UUID `json:"containerId,omitempty"`
	// States: container state rule.
	States []string `json:"states,omitempty"`
	// UnitPattern: unit rule, a name or a shell pattern ("*", "backup-*").
	UnitPattern string `json:"unitPattern,omitempty"`
	// Collector: collector rule, "system", "docker" or "services".
	Collector string `json:"collector,omitempty"`
	// ForSeconds is how long the condition must hold (0 to 86 400). A
	// failed unit fires at once.
	ForSeconds int `json:"forSeconds,omitempty"`
}

func invalidRule(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidAlertRule, fmt.Sprintf(format, args...))
}

// Validate checks a rule received from a client.
func (r AlertRule) Validate() error {
	name := strings.TrimSpace(r.Name)
	if name == "" || len(r.Name) > maxRuleNameBytes || !utf8.ValidString(r.Name) || strings.IndexFunc(r.Name, unicode.IsControl) >= 0 {
		return invalidRule("name must be 1 to %d bytes without control characters", maxRuleNameBytes)
	}
	if !slices.Contains(AlertSeverities, r.Severity) {
		return invalidRule("severity must be info, warning or critical")
	}
	p := r.Params
	if p.ForSeconds < 0 || p.ForSeconds > MaxAlertDuration {
		return invalidRule("forSeconds must be 0 to %d", MaxAlertDuration)
	}
	switch r.Type {
	case AlertRuleHostMetric:
		if p.ContainerID != nil || len(p.States) > 0 || p.UnitPattern != "" || p.Collector != "" {
			return invalidRule("a host metric rule takes metric, labels, operator, threshold and forSeconds only")
		}
		return validateMetric(p, hostAlertMetrics, "host")
	case AlertRuleContainerMetric:
		if len(p.States) > 0 || p.UnitPattern != "" || p.Collector != "" {
			return invalidRule("a container metric rule takes containerId, metric, labels, operator, threshold and forSeconds only")
		}
		return validateMetric(p, containerAlertMetrics, "container")
	case AlertRuleContainerState:
		if p.Metric != "" || len(p.Labels) > 0 || p.Operator != "" || p.Threshold != nil || p.UnitPattern != "" || p.Collector != "" {
			return invalidRule("a container state rule takes containerId, states and forSeconds only")
		}
		if len(p.States) == 0 {
			return invalidRule("states must not be empty")
		}
		for i, state := range p.States {
			if !slices.Contains(containerStates, state) || slices.Contains(p.States[:i], state) {
				return invalidRule("states must be distinct values among %s", strings.Join(containerStates, ", "))
			}
		}
		return nil
	case AlertRuleUnitFailed:
		if p.Metric != "" || len(p.Labels) > 0 || p.Operator != "" || p.Threshold != nil || p.ContainerID != nil || len(p.States) > 0 || p.Collector != "" || p.ForSeconds != 0 {
			return invalidRule("a unit rule takes unitPattern only")
		}
		if len(p.UnitPattern) > maxPatternBytes || !unitPatternCharacters.MatchString(p.UnitPattern) {
			return invalidRule("unitPattern must be 1 to %d characters among letters, digits, @._:-*?[]", maxPatternBytes)
		}
		if _, err := path.Match(p.UnitPattern, ""); err != nil {
			return invalidRule("unitPattern is malformed")
		}
		return nil
	case AlertRuleCollector:
		if p.Metric != "" || len(p.Labels) > 0 || p.Operator != "" || p.Threshold != nil || p.ContainerID != nil || len(p.States) > 0 || p.UnitPattern != "" {
			return invalidRule("a collector rule takes collector and forSeconds only")
		}
		if !slices.Contains(alertCollectors, p.Collector) {
			return invalidRule("collector must be system, docker or services")
		}
		return nil
	default:
		return invalidRule("type must be host_metric, container_metric, container_state, unit_failed or collector")
	}
}

func validateMetric(p AlertRuleParams, metrics map[string][]string, scope string) error {
	labels, ok := metrics[p.Metric]
	if !ok {
		return invalidRule("metric %q cannot be watched on a %s", p.Metric, scope)
	}
	if len(p.Labels) > maxRuleLabels {
		return invalidRule("at most %d labels", maxRuleLabels)
	}
	for key, value := range p.Labels {
		if !slices.Contains(labels, key) {
			return invalidRule("metric %s has no label %q", p.Metric, key)
		}
		if value == "" || len(value) > maxLabelBytes || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return invalidRule("label values must be 1 to %d bytes without control characters", maxLabelBytes)
		}
	}
	switch p.Operator {
	case OpGreater, OpGreaterOrEqual, OpLess, OpLessOrEqual:
	default:
		return invalidRule("operator must be >, >=, < or <=")
	}
	if p.Threshold == nil || math.IsNaN(*p.Threshold) || math.IsInf(*p.Threshold, 0) {
		return invalidRule("threshold must be a finite number")
	}
	return nil
}

func (o AlertOperator) holds(value, threshold float64) bool {
	switch o {
	case OpGreater:
		return value > threshold
	case OpGreaterOrEqual:
		return value >= threshold
	case OpLess:
		return value < threshold
	case OpLessOrEqual:
		return value <= threshold
	}
	return false
}

// AlertState is the lifecycle state of an alert (§ 6.2).
type AlertState string

const (
	AlertFiring       AlertState = "firing"
	AlertAcknowledged AlertState = "acknowledged"
	AlertResolved     AlertState = "resolved"
)

// AlertResolution tells why an alert was resolved.
type AlertResolution string

const (
	ResolutionConditionCleared AlertResolution = "condition_cleared"
	ResolutionNoData           AlertResolution = "no_data"
	ResolutionTargetGone       AlertResolution = "target_gone"
	ResolutionRuleDisabled     AlertResolution = "rule_disabled"
	ResolutionRuleDeleted      AlertResolution = "rule_deleted"
	ResolutionRuleChanged      AlertResolution = "rule_changed"
)

// AlertTargetKind is what an alert is about.
type AlertTargetKind string

const (
	AlertTargetHost      AlertTargetKind = "host"
	AlertTargetContainer AlertTargetKind = "container"
	AlertTargetUnit      AlertTargetKind = "unit"
	AlertTargetCollector AlertTargetKind = "collector"
)

// AlertTarget describes the target of an alert for display and links.
type AlertTarget struct {
	Kind          AlertTargetKind   `json:"kind"`
	Metric        string            `json:"metric,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	ContainerID   *uuid.UUID        `json:"containerId,omitempty"`
	ContainerName string            `json:"containerName,omitempty"`
	Unit          string            `json:"unit,omitempty"`
	Collector     string            `json:"collector,omitempty"`
}

// Alert is one firing of a rule on one target. RuleName, RuleType and
// Severity are copied so that the history survives a rule change.
type Alert struct {
	ID        uuid.UUID
	RuleID    uuid.UUID
	RuleName  string
	RuleType  AlertRuleType
	Severity  AlertSeverity
	TargetKey string
	Target    AlertTarget
	State     AlertState
	// Value and Threshold: metric rules. Observed: the state that fired a
	// state, unit or collector rule ("exited", "failed (exit-code)").
	Value          *float64
	Threshold      *float64
	Observed       string
	FiredAt        time.Time
	AcknowledgedAt *time.Time
	AcknowledgedBy string
	ResolvedAt     *time.Time
	Resolution     AlertResolution
}

// Open reports whether the alert is not resolved yet.
func (a Alert) Open() bool { return a.State != AlertResolved }

// AlertFilter selects the alert history. State "" or "all" keeps every
// alert; the time range applies to FiredAt, [From, To).
type AlertFilter struct {
	State string
	From  *time.Time
	To    *time.Time
	Limit int
}

// Validate checks the bounds; a zero limit becomes the default.
func (f AlertFilter) Validate() (AlertFilter, error) {
	switch f.State {
	case "":
		f.State = "all"
	case "all", "open", "resolved":
	default:
		return AlertFilter{}, ErrInvalidAlertFilter
	}
	if f.Limit < 0 || f.Limit > MaxAlertLimit {
		return AlertFilter{}, ErrInvalidAlertFilter
	}
	if f.Limit == 0 {
		f.Limit = DefaultAlertLimit
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return AlertFilter{}, ErrInvalidAlertFilter
	}
	return f, nil
}

// AlertSummary holds the indicators of § 6.2.
type AlertSummary struct {
	Open                     map[AlertSeverity]int
	ResolvedLast24h          int
	MeanResolutionSeconds24h *float64
}

// AlertSnapshot is what the engine reads at one evaluation. A source that
// could not be read (HostMetrics nil, ContainersKnown or UnitsKnown false)
// freezes the rules that depend on it: no firing, no resolution.
type AlertSnapshot struct {
	// HostMetrics is the latest fresh host cycle, nil when there is none.
	HostMetrics []MetricSample
	// Containers are the present containers, ContainerMetrics the latest
	// fresh cycle of each (absent for a container without one).
	ContainersKnown  bool
	Containers       []Container
	ContainerMetrics map[uuid.UUID][]MetricSample
	UnitsKnown       bool
	Units            []SystemUnit
	// Collectors maps "system", "docker" and "services" to their status.
	Collectors map[string]string
}

// AlertEngineState is the memory of the engine between evaluations, keyed
// by rule ID and target key: when a condition started to hold, and since
// when an open alert's series has been missing. It is lost on restart.
type AlertEngineState struct {
	Pending map[string]time.Time
	Missing map[string]time.Time
}

// AlertResolutionChange resolves an open alert.
type AlertResolutionChange struct {
	ID         uuid.UUID
	Resolution AlertResolution
	At         time.Time
}

// AlertEvaluation is the result of one evaluation.
type AlertEvaluation struct {
	Fired    []Alert
	Resolved []AlertResolutionChange
	State    AlertEngineState
}

// observation is one target of a rule seen at this evaluation.
type observation struct {
	key       string
	target    AlertTarget
	holds     bool
	value     *float64
	observed  string
	threshold *float64
}

// EvaluateAlerts compares the snapshot with the rules and the open alerts
// (SPEC_ADMINISTRATION.md § 6.1, § 6.2). It is pure: the caller stores the
// transitions and keeps the returned state for the next evaluation.
func EvaluateAlerts(rules []AlertRule, snapshot AlertSnapshot, open []Alert, state AlertEngineState, now time.Time) AlertEvaluation {
	result := AlertEvaluation{State: AlertEngineState{Pending: map[string]time.Time{}, Missing: map[string]time.Time{}}}
	openByKey := map[string]Alert{}
	for _, alert := range open {
		openByKey[engineKey(alert.RuleID, alert.TargetKey)] = alert
	}
	resolve := func(alert Alert, resolution AlertResolution) {
		result.Resolved = append(result.Resolved, AlertResolutionChange{ID: alert.ID, Resolution: resolution, At: now})
		delete(openByKey, engineKey(alert.RuleID, alert.TargetKey))
	}
	rulesByID := map[uuid.UUID]AlertRule{}
	for _, rule := range rules {
		rulesByID[rule.ID] = rule
		if !rule.Enabled {
			continue
		}
		prefix := rule.ID.String() + "|"
		observations, known := observe(rule, snapshot)
		if !known {
			// The source is unavailable: keep everything as it is.
			for _, memory := range []struct{ from, to map[string]time.Time }{{state.Pending, result.State.Pending}, {state.Missing, result.State.Missing}} {
				for key, since := range memory.from {
					if strings.HasPrefix(key, prefix) {
						memory.to[key] = since
					}
				}
			}
			continue
		}
		seen := map[string]bool{}
		for _, obs := range observations {
			key := prefix + obs.key
			seen[key] = true
			alert, isOpen := openByKey[key]
			if !obs.holds {
				if isOpen {
					resolve(alert, ResolutionConditionCleared)
				}
				continue
			}
			if isOpen {
				continue
			}
			since, pending := state.Pending[key]
			if !pending {
				since = now
			}
			if now.Sub(since) < time.Duration(rule.Params.ForSeconds)*time.Second {
				result.State.Pending[key] = since
				continue
			}
			result.Fired = append(result.Fired, Alert{
				ID: uuid.New(), RuleID: rule.ID, RuleName: rule.Name, RuleType: rule.Type, Severity: rule.Severity,
				TargetKey: obs.key, Target: obs.target, State: AlertFiring,
				Value: obs.value, Threshold: obs.threshold, Observed: obs.observed, FiredAt: now,
			})
		}
		for key, alert := range openByKey {
			if !strings.HasPrefix(key, prefix) || seen[key] {
				continue
			}
			if targetGone(alert.Target, snapshot) {
				resolve(alert, ResolutionTargetGone)
				continue
			}
			since, missing := state.Missing[key]
			if !missing {
				since = now
			}
			if now.Sub(since) >= MissingDataGrace {
				resolve(alert, ResolutionNoData)
				continue
			}
			result.State.Missing[key] = since
		}
	}
	for _, alert := range openByKey {
		rule, exists := rulesByID[alert.RuleID]
		switch {
		case !exists:
			resolve(alert, ResolutionRuleDeleted)
		case !rule.Enabled:
			resolve(alert, ResolutionRuleDisabled)
		}
	}
	sort.Slice(result.Resolved, func(i, j int) bool { return result.Resolved[i].ID.String() < result.Resolved[j].ID.String() })
	return result
}

func engineKey(ruleID uuid.UUID, targetKey string) string { return ruleID.String() + "|" + targetKey }

// targetGone reports a container or unit that left the inventory: its
// alerts are resolved at once, without waiting for the missing data grace.
func targetGone(target AlertTarget, snapshot AlertSnapshot) bool {
	switch target.Kind {
	case AlertTargetContainer:
		return target.ContainerID == nil || !slices.ContainsFunc(snapshot.Containers, func(c Container) bool { return c.ID == *target.ContainerID })
	case AlertTargetUnit:
		return !slices.ContainsFunc(snapshot.Units, func(u SystemUnit) bool { return u.Name == target.Unit })
	}
	return false
}

// observe lists the targets of a rule; known is false when the source of
// the rule could not be read.
func observe(rule AlertRule, snapshot AlertSnapshot) (observations []observation, known bool) {
	p := rule.Params
	switch rule.Type {
	case AlertRuleHostMetric:
		if snapshot.HostMetrics == nil {
			return nil, false
		}
		for _, s := range withDerivedMetrics(snapshot.HostMetrics) {
			if s.Name != p.Metric || !hasLabels(s.Labels, p.Labels) {
				continue
			}
			series := seriesKey(s.Name, s.Labels)
			observations = append(observations, metricObservation("host:"+series, AlertTarget{Kind: AlertTargetHost, Metric: s.Name, Labels: s.Labels}, s.Value, p))
		}
		return observations, true
	case AlertRuleContainerMetric:
		if !snapshot.ContainersKnown {
			return nil, false
		}
		for _, container := range selectContainers(snapshot.Containers, p.ContainerID) {
			for _, s := range snapshot.ContainerMetrics[container.ID] {
				if s.Name != p.Metric || !hasLabels(s.Labels, p.Labels) {
					continue
				}
				id := container.ID
				target := AlertTarget{Kind: AlertTargetContainer, Metric: s.Name, Labels: s.Labels, ContainerID: &id, ContainerName: container.Name}
				observations = append(observations, metricObservation("container:"+id.String()+":"+seriesKey(s.Name, s.Labels), target, s.Value, p))
			}
		}
		return observations, true
	case AlertRuleContainerState:
		if !snapshot.ContainersKnown {
			return nil, false
		}
		for _, container := range selectContainers(snapshot.Containers, p.ContainerID) {
			id := container.ID
			observations = append(observations, observation{
				key:    "container:" + id.String(),
				target: AlertTarget{Kind: AlertTargetContainer, ContainerID: &id, ContainerName: container.Name},
				holds:  slices.Contains(p.States, container.State), observed: container.State,
			})
		}
		return observations, true
	case AlertRuleUnitFailed:
		if !snapshot.UnitsKnown {
			return nil, false
		}
		for _, unit := range snapshot.Units {
			if matched, _ := path.Match(p.UnitPattern, unit.Name); !matched {
				continue
			}
			observed := unit.ActiveState
			if unit.Result != "" && unit.Result != "success" {
				observed += " (" + unit.Result + ")"
			}
			observations = append(observations, observation{
				key: "unit:" + unit.Name, target: AlertTarget{Kind: AlertTargetUnit, Unit: unit.Name},
				holds: unit.ActiveState == "failed", observed: observed,
			})
		}
		return observations, true
	case AlertRuleCollector:
		status, ok := snapshot.Collectors[p.Collector]
		if !ok {
			return nil, true
		}
		return []observation{{
			key: "collector:" + p.Collector, target: AlertTarget{Kind: AlertTargetCollector, Collector: p.Collector},
			holds: !slices.Contains(healthyCollectorStates, status), observed: status,
		}}, true
	}
	return nil, true
}

func metricObservation(key string, target AlertTarget, value float64, p AlertRuleParams) observation {
	threshold := *p.Threshold
	v := value
	return observation{key: key, target: target, holds: p.Operator.holds(value, threshold), value: &v, threshold: &threshold}
}

func selectContainers(containers []Container, id *uuid.UUID) []Container {
	if id == nil {
		return containers
	}
	for _, container := range containers {
		if container.ID == *id {
			return []Container{container}
		}
	}
	return nil
}

// withDerivedMetrics adds memory.available_percent to a host cycle that
// has a positive memory.total.
func withDerivedMetrics(samples []MetricSample) []MetricSample {
	var available, total *float64
	for _, s := range samples {
		switch s.Name {
		case "memory.available":
			available = &s.Value
		case "memory.total":
			total = &s.Value
		}
	}
	if available == nil || total == nil || *total <= 0 {
		return samples
	}
	return append(slices.Clone(samples), MetricSample{Name: DerivedMemoryAvailablePercent, Value: *available / *total * 100, Unit: "percent"})
}

func hasLabels(labels, wanted map[string]string) bool {
	for key, value := range wanted {
		if labels[key] != value {
			return false
		}
	}
	return true
}

// seriesKey names a series: metric{key=value,…} with sorted keys.
func seriesKey(name string, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key + "=" + labels[key]
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}
