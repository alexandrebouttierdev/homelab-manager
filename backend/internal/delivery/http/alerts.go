package httpdelivery

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

var (
	alertQueryParameters = map[string]bool{"state": true, "from": true, "to": true, "limit": true}
	lowercaseUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type alertRuleParamsDTO struct {
	Metric      string            `json:"metric,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Operator    string            `json:"operator,omitempty"`
	Threshold   *float64          `json:"threshold,omitempty"`
	ContainerID *string           `json:"containerId,omitempty"`
	States      []string          `json:"states,omitempty"`
	UnitPattern string            `json:"unitPattern,omitempty"`
	Collector   string            `json:"collector,omitempty"`
	ForSeconds  int               `json:"forSeconds"`
}

type alertRuleDTO struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Type      string             `json:"type"`
	Severity  string             `json:"severity"`
	Enabled   bool               `json:"enabled"`
	Default   bool               `json:"default"`
	Params    alertRuleParamsDTO `json:"params"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

type alertRuleRequest struct {
	Name     string             `json:"name"`
	Type     string             `json:"type"`
	Severity string             `json:"severity"`
	Enabled  *bool              `json:"enabled"`
	Params   alertRuleParamsDTO `json:"params"`
}

type alertTargetDTO struct {
	Kind          string            `json:"kind"`
	Metric        *string           `json:"metric"`
	Labels        map[string]string `json:"labels"`
	ContainerID   *string           `json:"containerId"`
	ContainerName *string           `json:"containerName"`
	Unit          *string           `json:"unit"`
	Collector     *string           `json:"collector"`
}

type alertDTO struct {
	ID             string         `json:"id"`
	RuleID         string         `json:"ruleId"`
	RuleName       string         `json:"ruleName"`
	RuleType       string         `json:"ruleType"`
	Severity       string         `json:"severity"`
	State          string         `json:"state"`
	TargetKey      string         `json:"targetKey"`
	Target         alertTargetDTO `json:"target"`
	Value          *float64       `json:"value"`
	Threshold      *float64       `json:"threshold"`
	Observed       *string        `json:"observed"`
	FiredAt        time.Time      `json:"firedAt"`
	AcknowledgedAt *time.Time     `json:"acknowledgedAt"`
	AcknowledgedBy *string        `json:"acknowledgedBy"`
	ResolvedAt     *time.Time     `json:"resolvedAt"`
	Resolution     *string        `json:"resolution"`
	Silenced       bool           `json:"silenced"`
	LastNotifiedAt *time.Time     `json:"lastNotifiedAt"`
}

type alertCatalogDTO struct {
	HostMetrics      map[string][]string `json:"hostMetrics"`
	ContainerMetrics map[string][]string `json:"containerMetrics"`
	ContainerStates  []string            `json:"containerStates"`
	Collectors       []string            `json:"collectors"`
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func toAlertRuleDTO(rule domain.AlertRule) alertRuleDTO {
	p := rule.Params
	params := alertRuleParamsDTO{
		Metric: p.Metric, Labels: p.Labels, Operator: string(p.Operator), Threshold: p.Threshold,
		States: p.States, UnitPattern: p.UnitPattern, Collector: p.Collector, ForSeconds: p.ForSeconds,
	}
	if p.ContainerID != nil {
		id := p.ContainerID.String()
		params.ContainerID = &id
	}
	return alertRuleDTO{
		ID: rule.ID.String(), Name: rule.Name, Type: string(rule.Type), Severity: string(rule.Severity),
		Enabled: rule.Enabled, Default: rule.Default, Params: params, CreatedAt: rule.CreatedAt, UpdatedAt: rule.UpdatedAt,
	}
}

func toAlertDTO(alert domain.Alert) alertDTO {
	target := alertTargetDTO{
		Kind: string(alert.Target.Kind), Metric: optional(alert.Target.Metric), Labels: alert.Target.Labels,
		ContainerName: optional(alert.Target.ContainerName), Unit: optional(alert.Target.Unit), Collector: optional(alert.Target.Collector),
	}
	if alert.Target.ContainerID != nil {
		target.ContainerID = optional(alert.Target.ContainerID.String())
	}
	if target.Labels == nil {
		target.Labels = map[string]string{}
	}
	return alertDTO{
		ID: alert.ID.String(), RuleID: alert.RuleID.String(), RuleName: alert.RuleName, RuleType: string(alert.RuleType),
		Severity: string(alert.Severity), State: string(alert.State), TargetKey: alert.TargetKey, Target: target,
		Value: alert.Value, Threshold: alert.Threshold, Observed: optional(alert.Observed), FiredAt: alert.FiredAt,
		AcknowledgedAt: alert.AcknowledgedAt, AcknowledgedBy: optional(alert.AcknowledgedBy),
		ResolvedAt: alert.ResolvedAt, Resolution: optional(string(alert.Resolution)),
		Silenced: alert.Silenced, LastNotifiedAt: alert.LastNotifiedAt,
	}
}

// alertRuleInput maps a request to a rule; the domain validates it.
func alertRuleInput(request alertRuleRequest) (domain.AlertRule, bool) {
	if request.Enabled == nil {
		return domain.AlertRule{}, false
	}
	p := request.Params
	params := domain.AlertRuleParams{
		Metric: p.Metric, Labels: p.Labels, Operator: domain.AlertOperator(p.Operator), Threshold: p.Threshold,
		States: p.States, UnitPattern: p.UnitPattern, Collector: p.Collector, ForSeconds: p.ForSeconds,
	}
	if p.ContainerID != nil {
		id, err := uuid.Parse(*p.ContainerID)
		if err != nil || !lowercaseUUIDPattern.MatchString(*p.ContainerID) {
			return domain.AlertRule{}, false
		}
		params.ContainerID = &id
	}
	return domain.AlertRule{
		Name: request.Name, Type: domain.AlertRuleType(request.Type), Severity: domain.AlertSeverity(request.Severity),
		Enabled: *request.Enabled, Params: params,
	}, true
}

// requireAlerts answers 503 when the engine is not running (no database).
func requireAlerts(alerts *usecase.AlertUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if alerts == nil {
				writeError(w, r, http.StatusServiceUnavailable, "ALERTS_UNAVAILABLE", "Alerts are unavailable.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func alertFilterFromQuery(r *http.Request) (domain.AlertFilter, error) {
	query := r.URL.Query()
	for key, values := range query {
		if !alertQueryParameters[key] || len(values) != 1 {
			return domain.AlertFilter{}, domain.ErrInvalidAlertFilter
		}
	}
	filter := domain.AlertFilter{State: query.Get("state")}
	for name, destination := range map[string]**time.Time{"from": &filter.From, "to": &filter.To} {
		if value := query.Get(name); value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return domain.AlertFilter{}, domain.ErrInvalidAlertFilter
			}
			*destination = &parsed
		}
	}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 {
			return domain.AlertFilter{}, domain.ErrInvalidAlertFilter
		}
		filter.Limit = limit
	}
	return filter.Validate()
}

func listAlertsHandler(alerts *usecase.AlertUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := alertFilterFromQuery(r)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "state must be open, resolved or all; from and to RFC 3339 timestamps with from before to; limit 1 to 1000.")
			return
		}
		list, err := alerts.Alerts(r.Context(), filter)
		if err != nil {
			writeAlertError(w, r, err)
			return
		}
		response := make([]alertDTO, 0, len(list))
		for _, alert := range list {
			response = append(response, toAlertDTO(alert))
		}
		writeJSON(w, http.StatusOK, map[string]any{"alerts": response})
	}
}

func alertSummaryHandler(alerts *usecase.AlertUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := alerts.Summary(r.Context())
		if err != nil {
			writeAlertError(w, r, err)
			return
		}
		open := map[string]int{}
		for _, severity := range domain.AlertSeverities {
			open[string(severity)] = summary.Open[severity]
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"open": open, "resolvedLast24h": summary.ResolvedLast24h, "meanResolutionSeconds24h": summary.MeanResolutionSeconds24h,
		})
	}
}

func alertHandler(alerts *usecase.AlertUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "alertID")
		if !ok {
			return
		}
		alert, err := alerts.Alert(r.Context(), id)
		if err != nil {
			writeAlertError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"alert": toAlertDTO(alert)})
	}
}

func acknowledgeAlertHandler(alerts *usecase.AlertUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "alertID")
		if !ok {
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		alert, err := alerts.Acknowledge(r.Context(), actor, id, middleware.GetReqID(r.Context()))
		if err != nil {
			writeAlertError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"alert": toAlertDTO(alert)})
	}
}

func listAlertRulesHandler(alerts *usecase.AlertUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules, err := alerts.Rules(r.Context())
		if err != nil {
			writeAlertError(w, r, err)
			return
		}
		response := make([]alertRuleDTO, 0, len(rules))
		for _, rule := range rules {
			response = append(response, toAlertRuleDTO(rule))
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": response, "catalog": alertCatalogDTO{
			HostMetrics: domain.HostAlertMetrics(), ContainerMetrics: domain.ContainerAlertMetrics(),
			ContainerStates: domain.AlertContainerStates(), Collectors: domain.AlertCollectors(),
		}})
	}
}

func alertRuleHandler(alerts *usecase.AlertUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "ruleID")
		if !ok {
			return
		}
		rule, err := alerts.Rule(r.Context(), id)
		if err != nil {
			writeAlertError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rule": toAlertRuleDTO(rule)})
	}
}

// decodeAlertRule reads the request body; false means a response was
// written.
func decodeAlertRule(w http.ResponseWriter, r *http.Request) (domain.AlertRule, domain.User, bool) {
	var request alertRuleRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
		return domain.AlertRule{}, domain.User{}, false
	}
	input, ok := alertRuleInput(request)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "enabled is required and containerId must be a lowercase UUID.")
		return domain.AlertRule{}, domain.User{}, false
	}
	actor, ok := sessionUser(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
		return domain.AlertRule{}, domain.User{}, false
	}
	return input, actor, true
}

func createAlertRuleHandler(alerts *usecase.AlertUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		input, actor, ok := decodeAlertRule(w, r)
		if !ok {
			return
		}
		rule, err := alerts.CreateRule(r.Context(), actor, input, middleware.GetReqID(r.Context()))
		if err != nil {
			writeAlertError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"rule": toAlertRuleDTO(rule)})
	}
}

func updateAlertRuleHandler(alerts *usecase.AlertUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "ruleID")
		if !ok {
			return
		}
		input, actor, ok := decodeAlertRule(w, r)
		if !ok {
			return
		}
		rule, err := alerts.UpdateRule(r.Context(), actor, id, input, middleware.GetReqID(r.Context()))
		if err != nil {
			writeAlertError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rule": toAlertRuleDTO(rule)})
	}
}

func deleteAlertRuleHandler(alerts *usecase.AlertUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "ruleID")
		if !ok {
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		if err := alerts.DeleteRule(r.Context(), actor, id, middleware.GetReqID(r.Context())); err != nil {
			writeAlertError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func writeAlertError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidAlertRule):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, domain.ErrInvalidAlertFilter):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The alert filter is invalid.")
	case errors.Is(err, domain.ErrAlertRuleNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The alert rule does not exist.")
	case errors.Is(err, domain.ErrAlertNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The alert does not exist.")
	case errors.Is(err, domain.ErrDefaultAlertRule):
		writeError(w, r, http.StatusConflict, "DEFAULT_RULE", "A default rule can be disabled, not deleted.")
	case errors.Is(err, domain.ErrTooManyAlertRules):
		writeError(w, r, http.StatusConflict, "TOO_MANY_RULES", "At most 200 alert rules.")
	case errors.Is(err, domain.ErrAlertResolved):
		writeError(w, r, http.StatusConflict, "ALERT_ALREADY_RESOLVED", "The alert is already resolved.")
	default:
		writeError(w, r, http.StatusInternalServerError, "ALERTS_FAILED", "Could not read or update the alerts.")
	}
}
