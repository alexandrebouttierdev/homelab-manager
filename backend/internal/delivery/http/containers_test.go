package httpdelivery

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"go.uber.org/zap"
)

type httpFakeDocker struct {
	mu    sync.Mutex
	calls []string
}

func (f *httpFakeDocker) ContainerLogs(_ context.Context, dockerID string, query domain.ContainerLogQuery) (domain.ContainerLogs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "logs "+dockerID)
	at := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	return domain.ContainerLogs{Lines: []domain.ContainerLogLine{
		{Stream: domain.ContainerStdout, Time: &at, Message: "listening on :80"},
		{Stream: domain.ContainerStderr, Message: "warning"},
	}, Truncated: query.Tail > 1000}, nil
}

func (f *httpFakeDocker) ActOnContainer(_ context.Context, dockerID string, verb domain.ContainerVerb) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, string(verb)+" "+dockerID)
	return nil
}

func newContainersRouter(t *testing.T, enabled bool) (http.Handler, *httpFakeDocker, string, *http.Cookie) {
	t.Helper()
	resetLoginRateLimit(t)
	services, database := newTestServices(t, nil)
	repository := sqlite.NewMonitoringRepository(database)
	at := time.Now().UTC().Add(-time.Hour)
	if _, err := repository.ReconcileContainers(context.Background(), []domain.Container{{
		DockerID: "0123456789ab", Name: "web", Image: "nginx:latest", State: "running", Status: "Up", CreatedAt: at,
	}}, at); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.GetContainer(context.Background(), "0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	docker := &httpFakeDocker{}
	containers := usecase.NewContainerUsecase(repository, docker, docker, sqlite.NewAuditRepository(database.Client()), enabled, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		containers.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	services.Monitoring = usecase.NewMonitoringUsecase(nil, nil, repository, usecase.MonitoringConfig{}, zap.NewNop())
	services.Containers = containers
	router := NewRouter(zap.NewNop(), services, false, "")
	return router, docker, stored.ID.String(), signIn(t, router, "admin", "admin-password-1")
}

func TestContainerLogsRoute(t *testing.T) {
	router, docker, id, admin := newContainersRouter(t, false)
	viewer := signInWithRole(t, router, admin, "victor", "viewer")
	response := call(router, http.MethodGet, "/api/v1/containers/"+id+"/logs?tail=50", "", viewer)
	if response.Code != http.StatusOK {
		t.Fatalf("logs = %d %s", response.Code, response.Body)
	}
	var body struct {
		Container struct{ Name string } `json:"container"`
		Lines     []struct {
			Stream, Message string
			Time            *time.Time
		} `json:"lines"`
		Truncated bool `json:"truncated"`
		Tail      int  `json:"tail"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Container.Name != "web" || len(body.Lines) != 2 || body.Lines[1].Stream != "stderr" || body.Lines[1].Time != nil || body.Tail != 50 {
		t.Fatalf("logs body = %+v", body)
	}
	if docker.calls[0] != "logs 0123456789ab" {
		t.Fatalf("calls = %v", docker.calls)
	}
	for _, query := range []string{"tail=0", "tail=2001", "tail=x", "since=yesterday", "follow=1", "tail=1&tail=2"} {
		if response := call(router, http.MethodGet, "/api/v1/containers/"+id+"/logs?"+query, "", viewer); response.Code != http.StatusBadRequest {
			t.Fatalf("logs?%s = %d", query, response.Code)
		}
	}
	if response := call(router, http.MethodGet, "/api/v1/containers/00000000-0000-4000-8000-000000000000/logs", "", viewer); response.Code != http.StatusNotFound {
		t.Fatalf("unknown container = %d", response.Code)
	}
}

func TestContainerActionRoutes(t *testing.T) {
	router, docker, id, admin := newContainersRouter(t, true)
	viewer := signInWithRole(t, router, admin, "victor", "viewer")
	operator := signInWithRole(t, router, admin, "olivia", "operator")

	settings := call(router, http.MethodGet, "/api/v1/docker/container-actions", "", viewer)
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), `"enabled":true`) {
		t.Fatalf("settings = %d %s", settings.Code, settings.Body)
	}
	path := "/api/v1/containers/" + id + "/actions"
	if response := call(router, http.MethodPost, path, `{"action":"restart"}`, viewer); response.Code != http.StatusForbidden {
		t.Fatalf("viewer action = %d", response.Code)
	}
	if response := call(router, http.MethodPost, path, `{"action":"kill"}`, operator); response.Code != http.StatusBadRequest {
		t.Fatalf("kill = %d", response.Code)
	}
	response := call(router, http.MethodPost, path, `{"action":"restart"}`, operator)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"state":"running"`) {
		t.Fatalf("restart = %d %s", response.Code, response.Body)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		last := call(router, http.MethodGet, path+"/last", "", viewer)
		if last.Code == http.StatusOK && strings.Contains(last.Body.String(), `"state":"succeeded"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("last = %d %s", last.Code, last.Body)
		}
		time.Sleep(5 * time.Millisecond)
	}
	docker.mu.Lock()
	calls := strings.Join(docker.calls, ",")
	docker.mu.Unlock()
	if calls != "restart 0123456789ab" {
		t.Fatalf("calls = %s", calls)
	}
}

func TestContainerActionsDisabled(t *testing.T) {
	router, docker, id, admin := newContainersRouter(t, false)
	response := call(router, http.MethodPost, "/api/v1/containers/"+id+"/actions", `{"action":"stop"}`, admin)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "CONTAINER_ACTIONS_DISABLED") || len(docker.calls) != 0 {
		t.Fatalf("disabled = %d %s, calls %v", response.Code, response.Body, docker.calls)
	}
}

func TestTokensCannotReadContainerLogs(t *testing.T) {
	router, _, id, admin := newContainersRouter(t, false)
	token, _ := createToken(t, router, admin, `{"name":"ha","scopes":["containers:read"],"expiresInDays":30}`)
	if response := callWithToken(router, http.MethodGet, "/api/v1/containers/"+id, token); response.Code != http.StatusOK {
		t.Fatalf("container = %d", response.Code)
	}
	if response := callWithToken(router, http.MethodGet, "/api/v1/containers/"+id+"/logs", token); response.Code != http.StatusForbidden {
		t.Fatalf("logs by token = %d", response.Code)
	}
}
