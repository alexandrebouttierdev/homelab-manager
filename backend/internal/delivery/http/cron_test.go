package httpdelivery

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"homelab-manager/backend/internal/adapters/crontab"
	"homelab-manager/backend/internal/usecase"
)

func newCronRouter(t *testing.T) http.Handler {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc", "cron.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	crontabText := "17 * * * * root cd / && run-parts --report /etc/cron.hourly\n@daily alice /opt/backup.sh token=secret-value\n"
	if err := os.WriteFile(filepath.Join(root, "etc", "crontab"), []byte(crontabText), 0o644); err != nil {
		t.Fatal(err)
	}
	services, _ := newTestServices(t, nil)
	services.Cron = usecase.NewCronUsecase(crontab.New(root), nil, nil, zap.NewNop())
	return NewRouter(zap.NewNop(), services, false, "")
}

func TestCronRoutes(t *testing.T) {
	resetLoginRateLimit(t)
	router := newCronRouter(t)
	if response := call(router, http.MethodGet, "/api/v1/cron", "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", response.Code)
	}
	viewer := signInViewer(t, router)
	response := call(router, http.MethodGet, "/api/v1/cron", "", viewer)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"description":"Toutes les heures, à la minute 17"`) ||
		!strings.Contains(body, `"expression":"@daily"`) || !strings.Contains(body, `"journalAvailable":false`) {
		t.Fatalf("cron = %d %s", response.Code, body)
	}
	if strings.Contains(body, "secret-value") {
		t.Fatal("a secret of a crontab is served")
	}
	response = call(router, http.MethodGet, "/api/v1/cron/inspect?expression="+strings.ReplaceAll("0 9 * * 1-5", " ", "%20"), "", viewer)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"description":"Du lundi au vendredi à 09:00"`) {
		t.Fatalf("inspect = %d %s", response.Code, response.Body)
	}
	for path, code := range map[string]string{
		"/api/v1/cron/inspect?expression=61%20*%20*%20*%20*": "CRON_EXPRESSION_INVALID",
		"/api/v1/cron/inspect?expression=x&expression=y":     "VALIDATION_ERROR",
		"/api/v1/cron/inspect?other=1":                       "VALIDATION_ERROR",
	} {
		if response := call(router, http.MethodGet, path, "", viewer); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), code) {
			t.Errorf("%s = %d %s, want 400 %s", path, response.Code, response.Body, code)
		}
	}
}
