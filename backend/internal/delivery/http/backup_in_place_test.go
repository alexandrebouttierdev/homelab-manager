package httpdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"homelab-manager/backend/internal/adapters/secretbox"
	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/adapters/sshkey"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
	"homelab-manager/backend/internal/usecase"

	"go.uber.org/zap"
)

// archiveExecutor writes the archive of a dump run like the runner does.
type archiveExecutor struct{ root string }

func (e archiveExecutor) Execute(ctx context.Context, job ports.BackupJob, secrets ports.BackupSecrets) (domain.BackupResult, error) {
	switch job.Operation {
	case domain.BackupOpDump:
		if err := os.WriteFile(domain.BackupArchivePath(e.root, job.RunID), []byte("archive-bytes"), 0o600); err != nil {
			return domain.BackupResult{}, err
		}
		return domain.BackupResult{Details: &domain.BackupRunDetails{SnapshotID: job.Dump.SnapshotID, ArchivePath: job.Dump.Path, ArchiveBytes: 13}}, nil
	case domain.BackupOpRestoreInPlace:
		return domain.BackupResult{Details: &domain.BackupRunDetails{
			SnapshotID: job.Restore.SnapshotID, Target: "/", DryRun: job.Restore.DryRun, InPlace: true, FilesRestored: 1,
		}}, nil
	}
	return instantExecutor{}.Execute(ctx, job, secrets)
}

func newArchiveRouter(t *testing.T) (http.Handler, *http.Cookie) {
	t.Helper()
	resetLoginRateLimit(t)
	services, database := newTestServices(t, nil)
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	backups := usecase.NewBackupUsecase(sqlite.NewBackupRepository(database.Client()), archiveExecutor{root: root}, box, sshkey.Keys{}, zap.NewNop()).
		WithArchiveRoot(root)
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
	for deadline := time.Now().Add(5 * time.Second); ; {
		if _, err := backups.RequestBackup(context.Background(), domain.User{Username: "probe"}, ""); !errors.Is(err, domain.ErrBackupsNotReady) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("backups never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	if response := call(router, http.MethodPut, "/api/v1/backups/config", backupConfigBody, admin); response.Code != http.StatusOK {
		t.Fatalf("config = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/repository/init", "", admin); response.Code != http.StatusAccepted {
		t.Fatalf("init = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/backups/config", admin, `"initialized":true`)
	return router, admin
}

func TestInPlaceRestoreAndArchiveRoutes(t *testing.T) {
	router, admin := newArchiveRouter(t)
	operator := signInWithRole(t, router, admin, "olivia", "operator")
	real := `{"operation":"restore-in-place","restore":{"snapshotId":"3a253ad8","include":["/srv/app"],"dryRun":false}}`
	if response := call(router, http.MethodPost, "/api/v1/backups/operations", real, operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator in-place = %d", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/operations", real, admin); response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "BACKUP_SIMULATION_REQUIRED") {
		t.Fatalf("without simulation = %d %s", response.Code, response.Body)
	}
	protected := `{"operation":"restore-in-place","restore":{"snapshotId":"3a253ad8","include":["/etc/passwd"],"dryRun":true}}`
	if response := call(router, http.MethodPost, "/api/v1/backups/operations", protected, admin); response.Code != http.StatusBadRequest {
		t.Fatalf("protected path = %d %s", response.Code, response.Body)
	}
	simulation := strings.Replace(real, `"dryRun":false`, `"dryRun":true`, 1)
	if response := call(router, http.MethodPost, "/api/v1/backups/operations", simulation, admin); response.Code != http.StatusAccepted {
		t.Fatalf("simulation = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/backups/runs?limit=1", admin, `"inPlace":true`)
	waitForBody(t, router, "/api/v1/backups/runs?limit=1", admin, `"state":"succeeded"`)
	if response := call(router, http.MethodPost, "/api/v1/backups/operations", real, admin); response.Code != http.StatusAccepted {
		t.Fatalf("in-place after simulation = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/backups/runs?limit=1", admin, `"state":"succeeded"`)

	dump := `{"operation":"dump","dump":{"snapshotId":"3a253ad8","path":"/srv/app"}}`
	response := call(router, http.MethodPost, "/api/v1/backups/operations", dump, admin)
	if response.Code != http.StatusAccepted {
		t.Fatalf("dump = %d %s", response.Code, response.Body)
	}
	var started struct {
		Run struct{ ID string } `json:"run"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	waitForBody(t, router, "/api/v1/backups/runs/"+started.Run.ID, admin, `"state":"succeeded"`)
	if response := call(router, http.MethodGet, "/api/v1/backups/runs/"+started.Run.ID+"/archive", "", operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator archive = %d", response.Code)
	}
	archive := call(router, http.MethodGet, "/api/v1/backups/runs/"+started.Run.ID+"/archive", "", admin)
	if archive.Code != http.StatusOK || archive.Body.String() != "archive-bytes" || archive.Header().Get("Content-Type") != "application/gzip" ||
		!strings.HasPrefix(archive.Header().Get("Content-Disposition"), `attachment; filename="sauvegarde-`) {
		t.Fatalf("archive = %d %q %v", archive.Code, archive.Body, archive.Header())
	}
	var runs struct {
		Runs []struct {
			ID        string `json:"id"`
			Operation string `json:"operation"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(call(router, http.MethodGet, "/api/v1/backups/runs?limit=10", "", admin).Body.Bytes(), &runs); err != nil {
		t.Fatal(err)
	}
	for _, run := range runs.Runs {
		if run.Operation != "dump" {
			if response := call(router, http.MethodGet, "/api/v1/backups/runs/"+run.ID+"/archive", "", admin); response.Code != http.StatusGone {
				t.Fatalf("archive of a %s run = %d", run.Operation, response.Code)
			}
			break
		}
	}
}
