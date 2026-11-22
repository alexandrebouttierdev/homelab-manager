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

	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func TestHealthEndpoints(t *testing.T) {
	router := NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()), nil, nil, false, "")
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
	router := NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()), nil, nil, false, "")
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
	router := NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()), nil, nil, false, root)
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
	router := NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()), nil, nil, false, "")
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
	router := NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()), nil, nil, false, "")
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

func TestTimeRangeRejectsUnimplementedAggregationAndCursors(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"aggregation", "?from=2026-11-20T00:00:00Z&to=2026-11-21T00:00:00Z&step=1m"},
		{"cursor", "?from=2026-11-20T00:00:00Z&to=2026-11-21T00:00:00Z&cursor=opaque"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/events"+test.query, nil)
			if _, _, _, err := timeRange(request); err == nil {
				t.Fatal("expected unsupported query option to be rejected")
			}
		})
	}
}

func TestAPIMethodNotAllowedUsesErrorFormat(t *testing.T) {
	router := NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()), nil, nil, false, "")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/health", nil))
	if recorder.Code != http.StatusMethodNotAllowed || !strings.Contains(recorder.Body.String(), `"code":"METHOD_NOT_ALLOWED"`) {
		t.Fatalf("got %d %q, want 405 METHOD_NOT_ALLOWED", recorder.Code, recorder.Body.String())
	}
}

func TestLoginRejectsCrossOrigin(t *testing.T) {
	router := NewRouter(zap.NewNop(), usecase.NewHealthUsecase(time.Now()), nil, nil, false, "")
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
