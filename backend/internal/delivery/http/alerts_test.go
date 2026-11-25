package httpdelivery

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// noAlertSources never runs: the routes are tested on stored alerts.
type noAlertSources struct{}

func (noAlertSources) LatestMetrics(context.Context, *string, time.Time) ([]domain.MetricSample, error) {
	return nil, nil
}
func (noAlertSources) ListContainers(context.Context, bool, int) ([]domain.Container, error) {
	return nil, nil
}
func (noAlertSources) LatestContainerMetrics(context.Context, time.Time) (map[uuid.UUID][]domain.MetricSample, error) {
	return nil, nil
}
func (noAlertSources) Status() (domain.CollectorStatus, domain.CollectorStatus) {
	return domain.CollectorStatus{}, domain.CollectorStatus{}
}
func (noAlertSources) Inventory() usecase.UnitInventory { return usecase.UnitInventory{} }
func (noAlertSources) Retention() time.Duration         { return time.Hour }

const diskRuleID = "6f1c2a10-7a4e-4c4b-9a51-000000000001"

func newAlertsRouter(t *testing.T) (http.Handler, *sqlite.AlertRepository) {
	t.Helper()
	services, database := newTestServices(t, nil)
	repository := sqlite.NewAlertRepository(database.Client())
	services.Alerts = usecase.NewAlertUsecase(repository, usecase.AlertSources{
		Metrics: noAlertSources{}, Collectors: noAlertSources{}, Units: noAlertSources{}, Retention: noAlertSources{},
	}, zap.NewNop())
	return NewRouter(zap.NewNop(), services, false, ""), repository
}

// signInWithRole creates an account of the role and signs it in.
func signInWithRole(t *testing.T, router http.Handler, admin *http.Cookie, username, role string) *http.Cookie {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"displayName":%q,"role":%q,"password":"initial-password-1"}`, username, username, role)
	if response := call(router, http.MethodPost, "/api/v1/users", body, admin); response.Code != http.StatusCreated {
		t.Fatalf("create %s = %d %s", username, response.Code, response.Body)
	}
	first := signIn(t, router, username, "initial-password-1")
	if response := call(router, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"initial-password-1","newPassword":"own-password-123"}`, first); response.Code != http.StatusOK {
		t.Fatalf("%s password = %d", username, response.Code)
	}
	return signIn(t, router, username, "own-password-123")
}

func storeAlerts(t *testing.T, repository *sqlite.AlertRepository) (open, resolved domain.Alert) {
	t.Helper()
	value, threshold := 91.5, 85.0
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	open = domain.Alert{
		ID: uuid.New(), RuleID: uuid.MustParse(diskRuleID), RuleName: "Disque presque plein", RuleType: domain.AlertRuleHostMetric,
		Severity: domain.AlertWarning, TargetKey: "host:filesystem.percent{mountpoint=/}", State: domain.AlertFiring,
		Target: domain.AlertTarget{Kind: domain.AlertTargetHost, Metric: "filesystem.percent", Labels: map[string]string{"mountpoint": "/"}},
		Value:  &value, Threshold: &threshold, FiredAt: at,
	}
	resolved = domain.Alert{
		ID: uuid.New(), RuleID: uuid.MustParse("6f1c2a10-7a4e-4c4b-9a51-000000000006"), RuleName: "Unité systemd en échec", RuleType: domain.AlertRuleUnitFailed,
		Severity: domain.AlertCritical, TargetKey: "unit:backup.service", State: domain.AlertFiring,
		Target: domain.AlertTarget{Kind: domain.AlertTargetUnit, Unit: "backup.service"}, Observed: "failed (exit-code)", FiredAt: at.Add(-time.Hour),
	}
	if err := repository.ApplyAlertEvaluation(context.Background(), []domain.Alert{open, resolved}, nil); err != nil {
		t.Fatal(err)
	}
	if err := repository.ApplyAlertEvaluation(context.Background(), nil, []domain.AlertResolutionChange{{ID: resolved.ID, Resolution: domain.ResolutionConditionCleared, At: at}}); err != nil {
		t.Fatal(err)
	}
	return open, resolved
}

func TestAlertRoutesByRole(t *testing.T) {
	resetLoginRateLimit(t)
	router, repository := newAlertsRouter(t)
	open, resolved := storeAlerts(t, repository)
	if response := call(router, http.MethodGet, "/api/v1/alerts", "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", response.Code)
	}
	admin := signIn(t, router, "admin", "admin-password-1")
	viewer := signInWithRole(t, router, admin, "vera", "viewer")
	operator := signInWithRole(t, router, admin, "otto", "operator")

	response := call(router, http.MethodGet, "/api/v1/alerts", "", viewer)
	if response.Code != http.StatusOK {
		t.Fatalf("list = %d %s", response.Code, response.Body)
	}
	var list struct {
		Alerts []map[string]any `json:"alerts"`
	}
	decodeBody(t, response, &list)
	if len(list.Alerts) != 2 || list.Alerts[0]["id"] != open.ID.String() {
		t.Fatalf("alerts = %+v", list.Alerts)
	}
	first := list.Alerts[0]
	target, _ := first["target"].(map[string]any)
	if first["state"] != "firing" || first["severity"] != "warning" || first["value"] != 91.5 || first["threshold"] != 85.0 ||
		first["ruleId"] != diskRuleID || first["ruleType"] != "host_metric" || target["kind"] != "host" || target["metric"] != "filesystem.percent" {
		t.Fatalf("open alert = %+v", first)
	}
	for _, key := range []string{"acknowledgedAt", "acknowledgedBy", "resolvedAt", "resolution", "observed"} {
		if value, present := first[key]; !present || value != nil {
			t.Errorf("%s = %v (present %v), want null", key, value, present)
		}
	}
	unit := list.Alerts[1]
	if unit["value"] != nil || unit["observed"] != "failed (exit-code)" || unit["resolution"] != "condition_cleared" || unit["resolvedAt"] == nil {
		t.Fatalf("resolved alert = %+v", unit)
	}

	response = call(router, http.MethodGet, "/api/v1/alerts?state=open", "", viewer)
	decodeBody(t, response, &list)
	if len(list.Alerts) != 1 {
		t.Fatalf("open alerts = %d", len(list.Alerts))
	}
	response = call(router, http.MethodGet, "/api/v1/alerts/summary", "", viewer)
	var summary struct {
		Open                     map[string]int `json:"open"`
		ResolvedLast24h          int            `json:"resolvedLast24h"`
		MeanResolutionSeconds24h *float64       `json:"meanResolutionSeconds24h"`
	}
	decodeBody(t, response, &summary)
	if response.Code != http.StatusOK || summary.Open["warning"] != 1 || summary.Open["critical"] != 0 || summary.Open["info"] != 0 ||
		summary.ResolvedLast24h != 1 || summary.MeanResolutionSeconds24h == nil || *summary.MeanResolutionSeconds24h != 3600 {
		t.Fatalf("summary = %d %+v", response.Code, summary)
	}
	if response := call(router, http.MethodGet, "/api/v1/alerts/"+open.ID.String(), "", viewer); response.Code != http.StatusOK {
		t.Fatalf("detail = %d", response.Code)
	}

	acknowledge := "/api/v1/alerts/" + open.ID.String() + "/acknowledge"
	if response := call(router, http.MethodPost, acknowledge, "", viewer); response.Code != http.StatusForbidden {
		t.Fatalf("viewer acknowledge = %d", response.Code)
	}
	response = call(router, http.MethodPost, acknowledge, "", operator)
	var acknowledged struct {
		Alert map[string]any `json:"alert"`
	}
	decodeBody(t, response, &acknowledged)
	if response.Code != http.StatusOK || acknowledged.Alert["state"] != "acknowledged" || acknowledged.Alert["acknowledgedBy"] != "otto" || acknowledged.Alert["acknowledgedAt"] == nil {
		t.Fatalf("operator acknowledge = %d %+v", response.Code, acknowledged.Alert)
	}
	if response := call(router, http.MethodPost, "/api/v1/alerts/"+resolved.ID.String()+"/acknowledge", "", operator); response.Code != http.StatusConflict || errorCode(t, response) != "ALERT_ALREADY_RESOLVED" {
		t.Fatalf("acknowledge a resolved alert = %d %s", response.Code, response.Body)
	}

	if response := call(router, http.MethodGet, "/api/v1/alert-rules", "", viewer); response.Code != http.StatusOK {
		t.Fatalf("viewer rules = %d", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/alert-rules", `{}`, operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator create rule = %d", response.Code)
	}
	if response := call(router, http.MethodDelete, "/api/v1/alert-rules/"+diskRuleID, "", operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator delete rule = %d", response.Code)
	}
}

func TestAlertQueryAndIdentifierErrors(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newAlertsRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	for path, want := range map[string]struct {
		status int
		code   string
	}{
		"/api/v1/alerts?state=closed":                 {http.StatusBadRequest, "VALIDATION_ERROR"},
		"/api/v1/alerts?limit=1001":                   {http.StatusBadRequest, "VALIDATION_ERROR"},
		"/api/v1/alerts?limit=abc":                    {http.StatusBadRequest, "VALIDATION_ERROR"},
		"/api/v1/alerts?from=yesterday":               {http.StatusBadRequest, "VALIDATION_ERROR"},
		"/api/v1/alerts?state=open&state=resolved":    {http.StatusBadRequest, "VALIDATION_ERROR"},
		"/api/v1/alerts?cursor=x":                     {http.StatusBadRequest, "VALIDATION_ERROR"},
		"/api/v1/alerts/not-a-uuid":                   {http.StatusBadRequest, "VALIDATION_ERROR"},
		"/api/v1/alerts/" + uuid.NewString():          {http.StatusNotFound, "NOT_FOUND"},
		"/api/v1/alert-rules/" + uuid.NewString():     {http.StatusNotFound, "NOT_FOUND"},
		"/api/v1/alert-rules/6F1C2A10-7A4E-4C4B-9A51": {http.StatusBadRequest, "VALIDATION_ERROR"},
	} {
		response := call(router, http.MethodGet, path, "", admin)
		if response.Code != want.status || errorCode(t, response) != want.code {
			t.Errorf("%s = %d %s, want %d %s", path, response.Code, response.Body, want.status, want.code)
		}
	}
	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	if response := call(router, http.MethodGet, "/api/v1/alerts?state=all&from="+from+"&to="+to+"&limit=10", "", admin); response.Code != http.StatusOK {
		t.Fatalf("valid query = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/alerts/"+uuid.NewString()+"/acknowledge", "", admin); response.Code != http.StatusNotFound {
		t.Fatalf("acknowledge an unknown alert = %d", response.Code)
	}
}

func TestAlertRuleManagement(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newAlertsRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")

	response := call(router, http.MethodGet, "/api/v1/alert-rules", "", admin)
	var rules struct {
		Rules   []map[string]any `json:"rules"`
		Catalog struct {
			HostMetrics      map[string][]string `json:"hostMetrics"`
			ContainerMetrics map[string][]string `json:"containerMetrics"`
			ContainerStates  []string            `json:"containerStates"`
			Collectors       []string            `json:"collectors"`
		} `json:"catalog"`
	}
	decodeBody(t, response, &rules)
	if response.Code != http.StatusOK || len(rules.Rules) != 7 {
		t.Fatalf("rules = %d %d", response.Code, len(rules.Rules))
	}
	if labels := rules.Catalog.HostMetrics["filesystem.percent"]; len(labels) != 2 || rules.Catalog.ContainerMetrics["memory.percent"] == nil ||
		len(rules.Catalog.ContainerStates) != 7 || len(rules.Catalog.Collectors) != 3 {
		t.Fatalf("catalog = %+v", rules.Catalog)
	}
	var disk map[string]any
	for _, rule := range rules.Rules {
		if rule["id"] == diskRuleID {
			disk = rule
		}
	}
	params, _ := disk["params"].(map[string]any)
	if disk["default"] != true || disk["enabled"] != true || params["metric"] == nil || params["forSeconds"] == nil {
		t.Fatalf("first rule = %+v", disk)
	}

	body := `{"name":"Conteneur web arrêté","type":"container_state","severity":"critical","enabled":true,
		"params":{"states":["exited","dead"],"forSeconds":60}}`
	response = call(router, http.MethodPost, "/api/v1/alert-rules", body, admin)
	var created struct {
		Rule map[string]any `json:"rule"`
	}
	decodeBody(t, response, &created)
	if response.Code != http.StatusCreated || created.Rule["default"] != false || created.Rule["type"] != "container_state" {
		t.Fatalf("create = %d %s", response.Code, response.Body)
	}
	id, _ := created.Rule["id"].(string)

	for name, invalid := range map[string]string{
		"unknown field":   `{"name":"x","type":"collector","severity":"info","enabled":true,"params":{"collector":"docker"},"extra":1}`,
		"unknown param":   `{"name":"x","type":"collector","severity":"info","enabled":true,"params":{"collector":"docker","command":"rm"}}`,
		"invalid rule":    `{"name":"x","type":"collector","severity":"info","enabled":true,"params":{"collector":"cron"}}`,
		"bad container":   `{"name":"x","type":"container_state","severity":"info","enabled":true,"params":{"states":["exited"],"containerId":"web"}}`,
		"missing enabled": `{"name":"x","type":"collector","severity":"info","params":{"collector":"docker"}}`,
		"not json":        `name=x`,
	} {
		if response := call(router, http.MethodPost, "/api/v1/alert-rules", invalid, admin); response.Code != http.StatusBadRequest || errorCode(t, response) != "VALIDATION_ERROR" {
			t.Errorf("%s = %d %s", name, response.Code, response.Body)
		}
	}

	update := `{"name":"Disque","type":"host_metric","severity":"critical","enabled":false,
		"params":{"metric":"filesystem.percent","labels":{"mountpoint":"/"},"operator":">=","threshold":90,"forSeconds":600}}`
	response = call(router, http.MethodPut, "/api/v1/alert-rules/"+diskRuleID, update, admin)
	var updated struct {
		Rule map[string]any `json:"rule"`
	}
	decodeBody(t, response, &updated)
	params, _ = updated.Rule["params"].(map[string]any)
	if response.Code != http.StatusOK || updated.Rule["default"] != true || updated.Rule["enabled"] != false || params["threshold"] != 90.0 {
		t.Fatalf("update = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/alert-rules/"+diskRuleID, "", admin); !strings.Contains(response.Body.String(), `"name":"Disque"`) {
		t.Fatalf("read after update = %s", response.Body)
	}
	if response := call(router, http.MethodPut, "/api/v1/alert-rules/"+uuid.NewString(), update, admin); response.Code != http.StatusNotFound {
		t.Fatalf("update an unknown rule = %d", response.Code)
	}
	if response := call(router, http.MethodDelete, "/api/v1/alert-rules/"+diskRuleID, "", admin); response.Code != http.StatusConflict || errorCode(t, response) != "DEFAULT_RULE" {
		t.Fatalf("delete a default rule = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodDelete, "/api/v1/alert-rules/"+id, "", admin); response.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodDelete, "/api/v1/alert-rules/"+id, "", admin); response.Code != http.StatusNotFound {
		t.Fatalf("delete twice = %d", response.Code)
	}
	if response := call(router, http.MethodGet, "/api/v1/audit?action=create", "", admin); !strings.Contains(response.Body.String(), `"targetType":"alert_rule"`) {
		t.Fatalf("audit = %s", response.Body)
	}
}

func TestAlertRoutesWithoutTheEngine(t *testing.T) {
	resetLoginRateLimit(t)
	services, _ := newTestServices(t, nil)
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	for _, path := range []string{"/api/v1/alerts", "/api/v1/alert-rules"} {
		if response := call(router, http.MethodGet, path, "", admin); response.Code != http.StatusServiceUnavailable || errorCode(t, response) != "ALERTS_UNAVAILABLE" {
			t.Errorf("%s = %d %s", path, response.Code, response.Body)
		}
	}
}

func TestBackupAlertRuleOverHTTP(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newAlertsRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	response := call(router, http.MethodPost, "/api/v1/alert-rules",
		`{"name":"Sauvegarde hebdomadaire","type":"backup","severity":"critical","enabled":true,"params":{"maxAgeHours":170}}`, admin)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"maxAgeHours":170`) {
		t.Fatalf("create = %d %s", response.Code, response.Body)
	}
	response = call(router, http.MethodPost, "/api/v1/alert-rules",
		`{"name":"x","type":"backup","severity":"critical","enabled":true,"params":{"maxAgeHours":0}}`, admin)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid age = %d %s", response.Code, response.Body)
	}
	if body := call(router, http.MethodGet, "/api/v1/alert-rules", "", admin).Body.String(); !strings.Contains(body, `"maxAgeHours":26`) {
		t.Fatalf("default backup rule missing: %s", body)
	}
}
