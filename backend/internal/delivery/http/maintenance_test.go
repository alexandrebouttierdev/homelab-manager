package httpdelivery

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"monoops/backend/internal/adapters/sqlite"
	"monoops/backend/internal/domain"
	"monoops/backend/internal/usecase"

	"go.uber.org/zap"
)

type recordingUnits struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingUnits) Act(_ context.Context, name string, verb domain.UnitVerb) (domain.UnitStableState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, string(verb)+" "+name)
	return domain.UnitStableState{ActiveState: "inactive", SubState: "dead"}, nil
}

func TestMaintenanceRoutes(t *testing.T) {
	resetLoginRateLimit(t)
	services, database := newTestServices(t, nil)
	units := &recordingUnits{}
	maintenance := usecase.NewMaintenanceUsecase(sqlite.NewMaintenanceRepository(database.Client()), units,
		sqlite.NewAuditRepository(database.Client()), zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		maintenance.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	services.Maintenance = maintenance
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	operator := signInWithRole(t, router, admin, "olivia", "operator")

	list := call(router, http.MethodGet, "/api/v1/maintenance/tasks", "", operator)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"kind":"journal-vacuum"`) ||
		!strings.Contains(list.Body.String(), `"sizes":[256,512,1024,2048]`) || !strings.Contains(list.Body.String(), `"unit":"monoops-journal-vacuum@512M.service"`) {
		t.Fatalf("list = %d %s", list.Code, list.Body)
	}
	body := `{"schedule":{"enabled":true,"frequency":"daily","weekday":0,"time":"02:30"},"maxSizeMiB":1024}`
	if response := call(router, http.MethodPut, "/api/v1/maintenance/tasks/journal-vacuum", body, operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator update = %d", response.Code)
	}
	response := call(router, http.MethodPut, "/api/v1/maintenance/tasks/journal-vacuum", body, admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"time":"02:30"`) || !strings.Contains(response.Body.String(), `"nextRunAt":"`) {
		t.Fatalf("update = %d %s", response.Code, response.Body)
	}
	for request, code := range map[string]int{
		`{"schedule":{"enabled":true,"frequency":"daily","weekday":0,"time":"02:30"},"maxSizeMiB":300}`:  http.StatusBadRequest,
		`{"schedule":{"enabled":true,"frequency":"hourly","weekday":0,"time":"02:30"},"maxSizeMiB":512}`: http.StatusBadRequest,
		`{"schedule":{"enabled":true,"frequency":"daily","weekday":0,"time":"2h"},"maxSizeMiB":512}`:     http.StatusBadRequest,
		`{"command":"rm -rf /"}`: http.StatusBadRequest,
	} {
		if response := call(router, http.MethodPut, "/api/v1/maintenance/tasks/journal-vacuum", request, admin); response.Code != code {
			t.Fatalf("%s = %d %s", request, response.Code, response.Body)
		}
	}
	if response := call(router, http.MethodPut, "/api/v1/maintenance/tasks/apt-upgrade", body, admin); response.Code != http.StatusNotFound {
		t.Fatalf("unknown task = %d", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/maintenance/tasks/journal-vacuum/run", "", admin); response.Code != http.StatusAccepted {
		t.Fatalf("run = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/maintenance/tasks", admin, `"origin":"manual","succeeded":true`)
	units.mu.Lock()
	calls := strings.Join(units.calls, ",")
	units.mu.Unlock()
	if calls != "start monoops-journal-vacuum@1024M.service" {
		t.Fatalf("calls = %s", calls)
	}
	audit := call(router, http.MethodGet, "/api/v1/audit?targetType=maintenance_task", "", admin)
	if !strings.Contains(audit.Body.String(), `"action":"run"`) || !strings.Contains(audit.Body.String(), `"action":"update"`) {
		t.Fatalf("audit = %s", audit.Body)
	}
}
