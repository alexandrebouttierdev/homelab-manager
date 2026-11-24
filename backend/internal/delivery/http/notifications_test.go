package httpdelivery

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"homelab-manager/backend/internal/adapters/notify"
	"homelab-manager/backend/internal/adapters/secretbox"
	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
	"homelab-manager/backend/internal/usecase"

	"go.uber.org/zap"
)

// webhookReceiver is a local webhook endpoint answering status.
type webhookReceiver struct {
	mu      sync.Mutex
	status  int
	headers []http.Header
	bodies  [][]byte
}

func (w *webhookReceiver) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.headers, w.bodies = append(w.headers, r.Header.Clone()), append(w.bodies, body)
	if w.status != 0 {
		rw.WriteHeader(w.status)
	}
}

func newNotificationsRouter(t *testing.T, withKey bool) (http.Handler, *sqlite.AlertRepository) {
	t.Helper()
	services, database := newTestServices(t, nil)
	alerts := sqlite.NewAlertRepository(database.Client())
	notifications := sqlite.NewNotificationRepository(database.Client())
	var box ports.SecretBox
	if withKey {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}
		opened, err := secretbox.New(key)
		if err != nil {
			t.Fatal(err)
		}
		box = opened
	}
	services.Notifications = usecase.NewNotificationUsecase(notifications, alerts, box, notify.New(),
		sqlite.NewAuditRepository(database.Client()), noAlertSources{}, zap.NewNop())
	services.Alerts = usecase.NewAlertUsecase(alerts, usecase.AlertSources{
		Metrics: noAlertSources{}, Collectors: noAlertSources{}, Units: noAlertSources{}, Retention: noAlertSources{},
	}, zap.NewNop()).WithNotifications(services.Notifications, notifications)
	return NewRouter(zap.NewNop(), services, false, ""), alerts
}

func TestSilenceRoutesByRole(t *testing.T) {
	resetLoginRateLimit(t)
	router, repository := newNotificationsRouter(t, true)
	open, _ := storeAlerts(t, repository)
	admin := signIn(t, router, "admin", "admin-password-1")
	viewer := signInWithRole(t, router, admin, "vera", "viewer")
	operator := signInWithRole(t, router, admin, "otto", "operator")

	body := fmt.Sprintf(`{"scope":"target","ruleId":%q,"targetKey":%q,"durationMinutes":60,"reason":"migration du disque"}`, diskRuleID, open.TargetKey)
	if response := call(router, http.MethodPost, "/api/v1/alert-silences", body, viewer); response.Code != http.StatusForbidden {
		t.Fatalf("viewer silence = %d", response.Code)
	}
	response := call(router, http.MethodPost, "/api/v1/alert-silences", body, operator)
	if response.Code != http.StatusCreated {
		t.Fatalf("operator silence = %d %s", response.Code, response.Body)
	}
	var created struct {
		Silence map[string]any `json:"silence"`
	}
	decodeBody(t, response, &created)
	if created.Silence["ruleName"] != "Disque presque plein" || created.Silence["createdBy"] != "otto" || created.Silence["active"] != true ||
		created.Silence["liftedAt"] != nil || created.Silence["targetKey"] != open.TargetKey {
		t.Fatalf("silence = %+v", created.Silence)
	}
	response = call(router, http.MethodGet, "/api/v1/alerts/"+open.ID.String(), "", viewer)
	if !strings.Contains(response.Body.String(), `"silenced":true`) || !strings.Contains(response.Body.String(), `"targetKey":"host:filesystem.percent{mountpoint=/}"`) {
		t.Fatalf("silenced alert = %s", response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/alert-silences", "", viewer); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "migration du disque") {
		t.Fatalf("viewer list = %d %s", response.Code, response.Body)
	}
	id := created.Silence["id"].(string)
	if response := call(router, http.MethodDelete, "/api/v1/alert-silences/"+id, "", viewer); response.Code != http.StatusForbidden {
		t.Fatalf("viewer lift = %d", response.Code)
	}
	response = call(router, http.MethodDelete, "/api/v1/alert-silences/"+id, "", operator)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"active":false`) || !strings.Contains(response.Body.String(), `"liftedBy":"otto"`) {
		t.Fatalf("lift = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/alerts/"+open.ID.String(), "", viewer); !strings.Contains(response.Body.String(), `"silenced":false`) {
		t.Fatalf("alert after the lift = %s", response.Body)
	}
	for name, invalid := range map[string]string{
		"short":        `{"scope":"all","durationMinutes":5,"reason":"x"}`,
		"no reason":    `{"scope":"all","durationMinutes":60,"reason":""}`,
		"upper uuid":   `{"scope":"rule","ruleId":"6F1C2A10-7A4E-4C4B-9A51-000000000001","durationMinutes":60,"reason":"x"}`,
		"unknown":      `{"scope":"all","durationMinutes":60,"reason":"x","alertId":"a"}`,
		"global rule":  fmt.Sprintf(`{"scope":"all","ruleId":%q,"durationMinutes":60,"reason":"x"}`, diskRuleID),
		"unknown rule": `{"scope":"rule","ruleId":"00000000-0000-0000-0000-000000000000","durationMinutes":60,"reason":"x"}`,
	} {
		want := http.StatusBadRequest
		if name == "unknown rule" {
			want = http.StatusNotFound
		}
		if response := call(router, http.MethodPost, "/api/v1/alert-silences", invalid, operator); response.Code != want {
			t.Errorf("%s = %d %s, want %d", name, response.Code, response.Body, want)
		}
	}
	if response := call(router, http.MethodDelete, "/api/v1/alert-silences/00000000-0000-0000-0000-000000000000", "", operator); response.Code != http.StatusNotFound {
		t.Fatalf("unknown silence = %d", response.Code)
	}
	if response := call(router, http.MethodGet, "/api/v1/audit?targetType=alert_silence", "", admin); strings.Count(response.Body.String(), `"targetType":"alert_silence"`) != 2 {
		t.Fatalf("audit = %s", response.Body)
	}
}

func TestNotificationChannelManagementAndTest(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newNotificationsRouter(t, true)
	receiver := &webhookReceiver{}
	server := httptest.NewServer(receiver)
	defer server.Close()
	admin := signIn(t, router, "admin", "admin-password-1")
	operator := signInWithRole(t, router, admin, "otto", "operator")
	if response := call(router, http.MethodGet, "/api/v1/notification-channels", "", operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator channels = %d", response.Code)
	}
	secret := "a-signing-secret-of-32-bytes-xyz"
	body := fmt.Sprintf(`{"name":"Récepteur","type":"webhook","enabled":true,"minSeverity":"warning","renotifyMinutes":30,
		"settings":{"url":%q},"secrets":{"signingSecret":%q}}`, server.URL+"/hook", secret)
	response := call(router, http.MethodPost, "/api/v1/notification-channels", body, admin)
	if response.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), secret) {
		t.Fatalf("the secret is served: %s", response.Body)
	}
	var created struct {
		Channel struct {
			ID      string `json:"id"`
			Secrets map[string]struct {
				Defined     bool    `json:"defined"`
				Fingerprint *string `json:"fingerprint"`
			} `json:"secrets"`
			Settings  map[string]any `json:"settings"`
			LastError *string        `json:"lastError"`
		} `json:"channel"`
	}
	decodeBody(t, response, &created)
	signing := created.Channel.Secrets["signingSecret"]
	if !signing.Defined || signing.Fingerprint == nil || *signing.Fingerprint != domain.SecretFingerprint(secret) || created.Channel.Settings["url"] != server.URL+"/hook" {
		t.Fatalf("channel = %+v", created.Channel)
	}
	path := "/api/v1/notification-channels/" + created.Channel.ID
	response = call(router, http.MethodPost, path+"/test", "", admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"lastSentAt":"`) {
		t.Fatalf("test = %d %s", response.Code, response.Body)
	}
	receiver.mu.Lock()
	if len(receiver.bodies) != 1 || !strings.Contains(string(receiver.bodies[0]), `"test":true`) ||
		receiver.headers[0].Get("X-Homelab-Signature") != domain.SignWebhook([]byte(secret), mustInt(t, receiver.headers[0].Get("X-Homelab-Timestamp")), receiver.bodies[0]) {
		t.Fatalf("webhook received %d bodies, headers %v", len(receiver.bodies), receiver.headers)
	}
	receiver.status = http.StatusInternalServerError
	receiver.mu.Unlock()
	response = call(router, http.MethodPost, path+"/test", "", admin)
	if response.Code != http.StatusBadGateway || errorCode(t, response) != "NOTIFICATION_FAILED" || strings.Contains(response.Body.String(), "127.0.0.1") {
		t.Fatalf("failed test = %d %s", response.Code, response.Body)
	}
	response = call(router, http.MethodGet, "/api/v1/notification-channels", "", admin)
	if !strings.Contains(response.Body.String(), `"lastError":"http_status"`) || !strings.Contains(response.Body.String(), `"lastErrorStatus":500`) ||
		!strings.Contains(response.Body.String(), `"secretsAvailable":true`) {
		t.Fatalf("list after a failure = %s", response.Body)
	}
	update := fmt.Sprintf(`{"name":"Récepteur","type":"webhook","enabled":false,"minSeverity":"critical","renotifyMinutes":0,"settings":{"url":%q},"secrets":{"signingSecret":null}}`, server.URL)
	response = call(router, http.MethodPut, path, update, admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"signingSecret":{"defined":false,"fingerprint":null}`) ||
		!strings.Contains(response.Body.String(), `"enabled":false`) {
		t.Fatalf("update = %d %s", response.Code, response.Body)
	}
	for name, invalid := range map[string]string{
		"no enabled":     `{"name":"x","type":"ntfy","minSeverity":"info","renotifyMinutes":0,"settings":{"serverUrl":"https://ntfy.sh","topic":"t"}}`,
		"unknown field":  `{"name":"x","type":"ntfy","enabled":true,"minSeverity":"info","renotifyMinutes":0,"settings":{"serverUrl":"https://ntfy.sh","topic":"t","extra":1}}`,
		"bad url":        `{"name":"x","type":"webhook","enabled":true,"minSeverity":"info","renotifyMinutes":0,"settings":{"url":"file:///etc/passwd"}}`,
		"missing token":  `{"name":"x","type":"telegram","enabled":true,"minSeverity":"info","renotifyMinutes":0,"settings":{"chatId":"1"}}`,
		"renotify 2":     `{"name":"x","type":"ntfy","enabled":true,"minSeverity":"info","renotifyMinutes":2,"settings":{"serverUrl":"https://ntfy.sh","topic":"t"}}`,
		"unknown secret": `{"name":"x","type":"ntfy","enabled":true,"minSeverity":"info","renotifyMinutes":0,"settings":{"serverUrl":"https://ntfy.sh","topic":"t"},"secrets":{"password":"x"}}`,
	} {
		if response := call(router, http.MethodPost, "/api/v1/notification-channels", invalid, admin); response.Code != http.StatusBadRequest {
			t.Errorf("%s = %d %s", name, response.Code, response.Body)
		}
	}
	changeType := `{"name":"x","type":"ntfy","enabled":true,"minSeverity":"info","renotifyMinutes":0,"settings":{"serverUrl":"https://ntfy.sh","topic":"t"}}`
	if response := call(router, http.MethodPut, path, changeType, admin); response.Code != http.StatusBadRequest {
		t.Fatalf("type change = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodDelete, path, "", admin); response.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, path+"/test", "", admin); response.Code != http.StatusNotFound {
		t.Fatalf("test of a deleted channel = %d", response.Code)
	}
	response = call(router, http.MethodGet, "/api/v1/audit?targetType=notification_channel", "", admin)
	for _, action := range []string{`"action":"create"`, `"action":"test"`, `"action":"update"`, `"action":"delete"`, `"outcome":"failure"`} {
		if !strings.Contains(response.Body.String(), action) {
			t.Errorf("audit misses %s: %s", action, response.Body)
		}
	}
}

func mustInt(t *testing.T, value string) int64 {
	t.Helper()
	var parsed int64
	if _, err := fmt.Sscan(value, &parsed); err != nil {
		t.Fatalf("timestamp %q: %v", value, err)
	}
	return parsed
}

func TestNotificationChannelsWithoutTheSecretsKey(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newNotificationsRouter(t, false)
	admin := signIn(t, router, "admin", "admin-password-1")
	withToken := `{"name":"g","type":"gotify","enabled":true,"minSeverity":"info","renotifyMinutes":0,"settings":{"serverUrl":"http://192.168.1.10"},"secrets":{"appToken":"abc"}}`
	if response := call(router, http.MethodPost, "/api/v1/notification-channels", withToken, admin); response.Code != http.StatusServiceUnavailable || errorCode(t, response) != "SECRETS_UNAVAILABLE" {
		t.Fatalf("create with a secret = %d %s", response.Code, response.Body)
	}
	withoutToken := `{"name":"n","type":"ntfy","enabled":true,"minSeverity":"info","renotifyMinutes":0,"settings":{"serverUrl":"https://ntfy.sh","topic":"homelab"}}`
	if response := call(router, http.MethodPost, "/api/v1/notification-channels", withoutToken, admin); response.Code != http.StatusCreated {
		t.Fatalf("create without secret = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/notification-channels", "", admin); !strings.Contains(response.Body.String(), `"secretsAvailable":false`) {
		t.Fatalf("list = %s", response.Body)
	}
}

func TestNotificationRoutesWithoutTheEngine(t *testing.T) {
	resetLoginRateLimit(t)
	services, _ := newTestServices(t, nil)
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	for _, path := range []string{"/api/v1/alert-silences", "/api/v1/notification-channels"} {
		if response := call(router, http.MethodGet, path, "", admin); response.Code != http.StatusServiceUnavailable || errorCode(t, response) != "ALERTS_UNAVAILABLE" {
			t.Errorf("%s = %d %s", path, response.Code, response.Body)
		}
	}
}
