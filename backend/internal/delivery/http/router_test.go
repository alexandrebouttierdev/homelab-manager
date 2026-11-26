package httpdelivery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"monoops/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func TestHealthEndpoints(t *testing.T) {
	router := NewRouter(zap.NewNop(), Services{Health: usecase.NewHealthUsecase(time.Now())}, false, "")
	for _, path := range []string{"/health", "/api/v1/health"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("got status %d, want %d", recorder.Code, http.StatusOK)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("got content type %q", got)
			}
		})
	}
}

func TestDetailedHealthReportsUnavailableDependencies(t *testing.T) {
	router := NewRouter(zap.NewNop(), Services{Health: usecase.NewHealthUsecase(time.Now())}, false, "")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["status"] != "degraded" || response["database"] != "unavailable" {
		t.Fatalf("unexpected health response: %#v", response)
	}
}

func TestStaticHandlerServesSPAAndDoesNotMaskUnknownAPI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("application"), 0o600); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(zap.NewNop(), Services{Health: usecase.NewHealthUsecase(time.Now())}, false, root)
	for _, test := range []struct {
		path       string
		statusCode int
		body       string
	}{
		{"/containers", http.StatusOK, "application"},
		{"/api/v1/unknown", http.StatusNotFound, `"code":"NOT_FOUND"`},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
		if recorder.Code != test.statusCode || !strings.Contains(recorder.Body.String(), test.body) {
			t.Errorf("%s returned %d and %q", test.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestLoginUnavailableWithoutDatabase(t *testing.T) {
	router := NewRouter(zap.NewNop(), Services{Health: usecase.NewHealthUsecase(time.Now())}, false, "")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"admin","password":"secure-password"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestManagementRoutesRequireSession(t *testing.T) {
	router := NewRouter(zap.NewNop(), Services{Health: usecase.NewHealthUsecase(time.Now())}, false, "")
	for _, path := range []string{"/api/v1/dashboard", "/api/v1/containers"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("got status %d, want %d", recorder.Code, http.StatusServiceUnavailable)
			}
		})
	}
}

func TestProtectedMutationRejectsCrossOrigin(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.Header.Set("Origin", "https://evil.example")
	sameOrigin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestTimeRangeRejectsEventStepsAndCursors(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		accepted map[string]bool
	}{
		{"event aggregation", "/api/v1/events?from=2026-11-20T00:00:00Z&to=2026-11-21T00:00:00Z&step=1m", eventQueryParameters},
		{"event label", "/api/v1/events?from=2026-11-20T00:00:00Z&to=2026-11-21T00:00:00Z&label=core:all", eventQueryParameters},
		{"cursor", "/api/v1/host/metrics/cpu.percent?from=2026-11-20T00:00:00Z&to=2026-11-21T00:00:00Z&cursor=opaque", metricQueryParameters},
		{"repeated limit", "/api/v1/host/metrics/cpu.percent?from=2026-11-20T00:00:00Z&to=2026-11-21T00:00:00Z&limit=1&limit=2", metricQueryParameters},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if _, _, _, err := timeRange(request, test.accepted); err == nil {
				t.Fatal("expected the query option to be rejected")
			}
		})
	}
}

func TestMetricOptions(t *testing.T) {
	valid := httptest.NewRequest(http.MethodGet, "/x?step=5m&label=core:all&label=interface:veth1a2b", nil)
	step, resolution, labels, err := metricOptions(valid)
	if err != nil || step != 5*time.Minute || resolution != "5m" || labels["core"] != "all" || labels["interface"] != "veth1a2b" {
		t.Fatalf("metricOptions = (%v, %q, %v, %v)", step, resolution, labels, err)
	}
	if step, resolution, _, err := metricOptions(httptest.NewRequest(http.MethodGet, "/x", nil)); err != nil || step != 0 || resolution != "raw" {
		t.Fatalf("default = (%v, %q, %v), want raw", step, resolution, err)
	}
	for _, query := range []string{
		"?step=2m", "?label=core", "?label=Core:all", "?label=core:", "?label=core:a&label=core:b",
		"?label=a:1&label=b:2&label=c:3&label=d:4&label=e:5",
		"?label=$.x:1",
	} {
		if _, _, _, err := metricOptions(httptest.NewRequest(http.MethodGet, "/x"+query, nil)); err == nil {
			t.Errorf("%s should be rejected", query)
		}
	}
}

func TestAPIMethodNotAllowedUsesErrorFormat(t *testing.T) {
	router := NewRouter(zap.NewNop(), Services{Health: usecase.NewHealthUsecase(time.Now())}, false, "")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/health", nil))
	if recorder.Code != http.StatusMethodNotAllowed || !strings.Contains(recorder.Body.String(), `"code":"METHOD_NOT_ALLOWED"`) {
		t.Fatalf("got %d %q, want 405 METHOD_NOT_ALLOWED", recorder.Code, recorder.Body.String())
	}
}

func TestLoginRejectsCrossOrigin(t *testing.T) {
	router := NewRouter(zap.NewNop(), Services{Health: usecase.NewHealthUsecase(time.Now())}, false, "")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"admin","password":"admin"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://evil.example")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestMetricHistoryValidatesPathParameters(t *testing.T) {
	router := chi.NewRouter()
	// A nil usecase proves validation happens before any data access.
	router.Get("/containers/{containerID}/metrics/{metricName}", metricHistoryHandler(nil, true))
	router.Get("/containers/{containerID}", getContainerHandler(nil))
	query := "?from=2026-11-22T10:00:00Z&to=2026-11-22T11:00:00Z"
	for _, path := range []string{
		"/containers/not-an-id/metrics/cpu.percent" + query,
		"/containers/" + strings.Repeat("a", 65) + "/metrics/cpu.percent" + query,
		"/containers/aaaaaaaaaaaa/metrics/CPU" + query,
		"/containers/aaaaaaaaaaaa/metrics/cpu" + query,
		"/containers/aaaaaaaaaaaa/metrics/" + strings.Repeat("a.", 40) + "x" + query,
		"/containers/../json",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound {
			t.Errorf("%s returned %d, want a rejection", path, recorder.Code)
		}
	}
	for _, valid := range []string{"cpu.percent", "network.rx_bytes_per_second", "temperature.celsius"} {
		if !metricNamePattern.MatchString(valid) {
			t.Errorf("%q should be a valid metric name", valid)
		}
	}
	for _, valid := range []string{"9369be23-5ad3-467d-b954-470b622a1442", "aaaaaaaaaaaa", strings.Repeat("f", 64)} {
		if !containerIDPattern.MatchString(valid) {
			t.Errorf("%q should be a valid container ID", valid)
		}
	}
}

func TestUpdateRetentionValidatesTheBody(t *testing.T) {
	// A nil usecase proves validation happens before any change.
	handler := updateRetentionHandler(nil)
	for _, body := range []string{``, `{}`, `{"metricsRetentionDays":0}`, `{"metricsRetentionDays":366}`, `{"metricsRetentionDays":"7"}`, `{"metricsRetentionDays":7,"extra":1}`} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/v1/settings/retention", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("body %q returned %d, want 400", body, recorder.Code)
		}
	}
}
