package httpdelivery

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"homelab-manager/backend/internal/adapters/secretbox"
	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
	"homelab-manager/backend/internal/usecase"
)

// instantExecutor succeeds at once with one snapshot.
type instantExecutor struct{}

func (instantExecutor) Execute(_ context.Context, job ports.BackupJob, _ domain.BackupCredentials) (domain.BackupResult, error) {
	if job.Operation == domain.BackupOpInit {
		return domain.BackupResult{}, nil
	}
	return domain.BackupResult{
		SnapshotID: "abcdef", Stats: &domain.BackupStats{FilesNew: 2, DataAdded: 1024, TotalFilesProcessed: 2},
		Snapshots: []domain.BackupSnapshot{{ID: "abcdef", ShortID: "abcdef", Time: time.Now().UTC(), Paths: []string{"/etc"}}},
	}, nil
}

func newBackupsRouter(t *testing.T) http.Handler {
	t.Helper()
	services, database := newTestServices(t, nil)
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	backups := usecase.NewBackupUsecase(sqlite.NewBackupRepository(database.Client()), instantExecutor{}, box, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		backups.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	services.Backups = backups
	return NewRouter(zap.NewNop(), services, false, "")
}

func waitForBody(t *testing.T, router http.Handler, path string, cookie *http.Cookie, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		body := call(router, http.MethodGet, path, "", cookie).Body.String()
		if strings.Contains(body, want) {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never contained %s: %s", path, want, body)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

const backupConfigBody = `{"repository":{"type":"local","location":"/mnt/backup"},"sources":["/etc","/srv"],"excludes":["*.tmp"],` +
	`"excludeCaches":true,"excludeIfPresent":".nobackup","schedule":{"enabled":true,"frequency":"weekly","weekday":0,"time":"04:30"}}`

func TestBackupRoutes(t *testing.T) {
	resetLoginRateLimit(t)
	router := newBackupsRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	operator := signInWithRole(t, router, admin, "olivia", "operator")
	viewer := signInWithRole(t, router, admin, "vera", "viewer")

	summary := call(router, http.MethodGet, "/api/v1/backups/summary", "", viewer).Body.String()
	if !strings.Contains(summary, `"configured":false`) || !strings.Contains(summary, `"nextRunAt":null`) {
		t.Fatalf("empty summary = %s", summary)
	}
	if response := call(router, http.MethodPut, "/api/v1/backups/config", backupConfigBody, operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator config = %d, want 403", response.Code)
	}
	response := call(router, http.MethodPut, "/api/v1/backups/config", backupConfigBody, admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"time":"04:30"`) ||
		!strings.Contains(response.Body.String(), `"credentials":{"password":false,"username":false,"secret":false}`) {
		t.Fatalf("config = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/runs", "", operator); response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "BACKUP_NOT_INITIALIZED") {
		t.Fatalf("backup before init = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/repository/init", "", operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator init = %d", response.Code)
	}
	response = call(router, http.MethodPost, "/api/v1/backups/repository/init", "", admin)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"password":"`) ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("init = %d %s", response.Code, response.Body)
	}
	password := response.Body.String()[strings.Index(response.Body.String(), `"password":"`)+12:]
	password = password[:strings.Index(password, `"`)]
	waitForBody(t, router, "/api/v1/backups/summary", viewer, `"initialized":true`)
	if body := call(router, http.MethodGet, "/api/v1/backups/config", "", admin).Body.String(); strings.Contains(body, password) ||
		!strings.Contains(body, `"password":true`) {
		t.Fatalf("the password must never be served again: %s", body)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/runs", "", viewer); response.Code != http.StatusForbidden {
		t.Fatalf("viewer backup = %d", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/runs", "", operator); response.Code != http.StatusAccepted {
		t.Fatalf("operator backup = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/backups/summary", viewer, `"snapshotId":"abcdef"`)
	if body := call(router, http.MethodGet, "/api/v1/backups/snapshots", "", viewer).Body.String(); !strings.Contains(body, `"shortId":"abcdef"`) {
		t.Fatalf("snapshots = %s", body)
	}
	runs := call(router, http.MethodGet, "/api/v1/backups/runs?limit=10", "", viewer).Body.String()
	if !strings.Contains(runs, `"operation":"init"`) || !strings.Contains(runs, `"filesNew":2`) || !strings.Contains(runs, `"requestedBy":"olivia"`) {
		t.Fatalf("runs = %s", runs)
	}
	audit := call(router, http.MethodGet, "/api/v1/audit?targetType=backup_run", "", admin).Body.String()
	if !strings.Contains(audit, `"actor":"olivia"`) {
		t.Fatalf("audit = %s", audit)
	}
}

func TestBackupRouteErrors(t *testing.T) {
	resetLoginRateLimit(t)
	router := newBackupsRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	for _, tc := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{http.MethodPut, "/api/v1/backups/config", strings.Replace(backupConfigBody, `"/etc"`, `"etc"`, 1), http.StatusBadRequest, "VALIDATION_ERROR"},
		{http.MethodPut, "/api/v1/backups/config", strings.Replace(backupConfigBody, "04:30", "4h30", 1), http.StatusBadRequest, "VALIDATION_ERROR"},
		{http.MethodPut, "/api/v1/backups/config", strings.Replace(backupConfigBody, `"local"`, `"sftp"`, 1), http.StatusBadRequest, "VALIDATION_ERROR"},
		{http.MethodPut, "/api/v1/backups/config", `{"unknown":1}`, http.StatusBadRequest, "VALIDATION_ERROR"},
		{http.MethodPost, "/api/v1/backups/repository/init", "", http.StatusConflict, "BACKUP_NOT_CONFIGURED"},
		{http.MethodGet, "/api/v1/backups/runs?limit=0", "", http.StatusBadRequest, "VALIDATION_ERROR"},
		{http.MethodGet, "/api/v1/backups/runs?offset=1", "", http.StatusBadRequest, "VALIDATION_ERROR"},
		{http.MethodGet, "/api/v1/backups/runs/not-a-uuid", "", http.StatusBadRequest, "VALIDATION_ERROR"},
		{http.MethodGet, "/api/v1/backups/runs/7c9e6679-7425-40de-944b-e07fc1f90ae7", "", http.StatusNotFound, "NOT_FOUND"},
	} {
		response := call(router, tc.method, tc.path, tc.body, admin)
		if response.Code != tc.status || !strings.Contains(response.Body.String(), `"code":"`+tc.code+`"`) {
			t.Errorf("%s %s = %d %s, want %d %s", tc.method, tc.path, response.Code, response.Body, tc.status, tc.code)
		}
	}
}

func TestBackupsUnavailableWithoutTheModule(t *testing.T) {
	resetLoginRateLimit(t)
	services, _ := newTestServices(t, nil)
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	if response := call(router, http.MethodGet, "/api/v1/backups/summary", "", admin); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("summary = %d", response.Code)
	}
}
