package httpdelivery

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"monoops/backend/internal/adapters/sqlite"
	"monoops/backend/internal/domain"
	"monoops/backend/internal/usecase"

	"go.uber.org/zap"
)

func TestDockerInventoryRoute(t *testing.T) {
	resetLoginRateLimit(t)
	services, database := newTestServices(t, nil)
	services.Monitoring = usecase.NewMonitoringUsecase(nil, nil, sqlite.NewMonitoringRepository(database), usecase.MonitoringConfig{}, zap.NewNop())
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	viewer := signInWithRole(t, router, admin, "vera", "viewer")

	response := call(router, http.MethodGet, "/api/v1/docker/inventory", "", viewer)
	if response.Code != http.StatusOK {
		t.Fatalf("inventory = %d %s", response.Code, response.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "starting" || body["observedAt"] != nil || len(body["images"].([]any)) != 0 ||
		len(body["volumes"].([]any)) != 0 || len(body["networks"].([]any)) != 0 {
		t.Fatalf("inventory before the first cycle = %v", body)
	}
	if response := call(router, http.MethodGet, "/api/v1/docker/inventory?all=1", "", viewer); response.Code != http.StatusBadRequest {
		t.Fatalf("inventory with a parameter = %d, want 400", response.Code)
	}
	if response := call(router, http.MethodGet, "/api/v1/docker/inventory", "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("inventory without a session = %d, want 401", response.Code)
	}

	containers, _ := createToken(t, router, viewer, `{"name":"inventory","scopes":["containers:read"],"expiresInDays":30}`)
	if response := callWithToken(router, http.MethodGet, "/api/v1/docker/inventory", containers); response.Code != http.StatusOK {
		t.Fatalf("inventory with containers:read = %d", response.Code)
	}
	events, _ := createToken(t, router, viewer, `{"name":"events","scopes":["events:read"],"expiresInDays":30}`)
	if response := callWithToken(router, http.MethodGet, "/api/v1/docker/inventory", events); response.Code != http.StatusForbidden {
		t.Fatalf("inventory with events:read = %d, want 403", response.Code)
	}
}

func TestDockerInventoryResponse(t *testing.T) {
	at := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	response := dockerInventoryResponse(&domain.DockerInventory{
		Images:     []domain.DockerImage{{ID: "sha256:old", CreatedAt: at, SizeBytes: 10}},
		Volumes:    []domain.DockerVolume{{Name: "orphan", Driver: "local"}},
		Networks:   []domain.DockerNetwork{{ID: "n", Name: "none", Predefined: true}},
		Truncated:  true,
		ObservedAt: at,
	}, domain.CollectorStatus{Status: "ok", LastSuccessAt: &at})
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"tags":[]`, `"subnets":[]`, `"createdAt":null`, `"predefined":true`, `"truncated":true`, `"observedAt":"2026-11-25T09:00:00Z"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s lacks %s", data, want)
		}
	}
}
