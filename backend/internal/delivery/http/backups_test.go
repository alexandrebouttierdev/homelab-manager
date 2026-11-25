package httpdelivery

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"homelab-manager/backend/internal/adapters/secretbox"
	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/adapters/sshkey"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
	"homelab-manager/backend/internal/usecase"
)

// instantExecutor succeeds at once with one snapshot.
type instantExecutor struct{}

func (instantExecutor) Execute(_ context.Context, job ports.BackupJob, secrets ports.BackupSecrets) (domain.BackupResult, error) {
	copyCredentials := secrets.Copy
	switch job.Operation {
	case domain.BackupOpDiff:
		return domain.BackupResult{Details: &domain.BackupRunDetails{Diff: &domain.BackupDiff{
			From: job.Diff.From, To: job.Diff.To, ChangedFiles: 1, TotalChanges: 1,
			Changes: []domain.BackupDiffChange{{Path: "/etc/secret-name.conf", Modifier: "M"}},
		}}}, nil
	case domain.BackupOpCopy:
		if copyCredentials == nil || copyCredentials.Password == "" {
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureJob, Err: errors.New("no copy password")}
		}
		return domain.BackupResult{Details: &domain.BackupRunDetails{Copy: &domain.BackupCopyResult{
			Repository: *job.Copy, Initialized: job.CopyInit, SnapshotsCopied: 1, SnapshotsInTarget: 1,
		}}}, nil
	case domain.BackupOpInit, domain.BackupOpUnlock:
		return domain.BackupResult{}, nil
	case domain.BackupOpRestore:
		return domain.BackupResult{Details: &domain.BackupRunDetails{
			SnapshotID: job.Restore.SnapshotID, Target: domain.RestoreTarget(job.RunID), DryRun: job.Restore.DryRun, FilesRestored: 2,
		}}, nil
	case domain.BackupOpSnapshots:
		return domain.BackupResult{Snapshots: []domain.BackupSnapshot{{ID: "0123456789ab", ShortID: "01234567", Time: time.Now().UTC()}}}, nil
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
	backups := usecase.NewBackupUsecase(sqlite.NewBackupRepository(database.Client()), instantExecutor{}, box, sshkey.Keys{}, zap.NewNop())
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
	// Run accepts operations once it closed the runs left by a stop.
	for deadline := time.Now().Add(5 * time.Second); ; {
		if _, err := backups.RequestBackup(context.Background(), domain.User{Username: "probe"}, ""); !errors.Is(err, domain.ErrBackupsNotReady) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("backups never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
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
		{http.MethodPut, "/api/v1/backups/config", strings.Replace(backupConfigBody, `"local"`, `"ftp"`, 1), http.StatusBadRequest, "VALIDATION_ERROR"},
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

func TestBackupOperationRoutes(t *testing.T) {
	resetLoginRateLimit(t)
	router := newBackupsRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	operator := signInWithRole(t, router, admin, "olivia", "operator")
	body := strings.Replace(backupConfigBody, `"schedule"`, `"retention":{"enabled":true,"keepDaily":7,"keepWeekly":0,"keepMonthly":3,"keepYearly":0},"check":{"enabled":false,"subsetPercent":10},"schedule"`, 1)
	response := call(router, http.MethodPut, "/api/v1/backups/config", body, admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"retention":{"enabled":true,"keepDaily":7,"keepWeekly":0,"keepMonthly":3,"keepYearly":0}`) ||
		!strings.Contains(response.Body.String(), `"check":{"enabled":false,"subsetPercent":10}`) {
		t.Fatalf("config = %d %s", response.Code, response.Body)
	}
	// Connect an existing repository instead of initialising one.
	if response := call(router, http.MethodPost, "/api/v1/backups/repository/connect", `{"password":"an-old-password"}`, operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator connect = %d", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/repository/connect", `{"password":"an-old-password"}`, admin); response.Code != http.StatusAccepted {
		t.Fatalf("connect = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/backups/summary", admin, `"initialized":true`)
	if body := call(router, http.MethodGet, "/api/v1/backups/snapshots", "", admin).Body.String(); !strings.Contains(body, `"shortId":"01234567"`) {
		t.Fatalf("snapshots after connect = %s", body)
	}

	if response := call(router, http.MethodPost, "/api/v1/backups/operations", `{"operation":"unlock"}`, operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator operation = %d", response.Code)
	}
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"operation":"backup"}`, http.StatusBadRequest},
		{`{"operation":"restore","restore":{"snapshotId":"latest"}}`, http.StatusBadRequest},
		{`{"operation":"restore","restore":{"snapshotId":"0123456789ab","include":["etc"]}}`, http.StatusBadRequest},
		{`{"operation":"diff"}`, http.StatusBadRequest},
	} {
		if response := call(router, http.MethodPost, "/api/v1/backups/operations", tc.body, admin); response.Code != tc.code {
			t.Errorf("%s = %d %s, want %d", tc.body, response.Code, response.Body, tc.code)
		}
	}
	response = call(router, http.MethodPost, "/api/v1/backups/operations", `{"operation":"restore","restore":{"snapshotId":"0123456789ab","include":["/etc"],"dryRun":true}}`, admin)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"target":"/var/lib/homelab-backup/restore/`) {
		t.Fatalf("restore = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/backups/runs?limit=1", admin, `"filesRestored":2`)
	audit := call(router, http.MethodGet, "/api/v1/audit?action=restore", "", admin).Body.String()
	if !strings.Contains(audit, `"targetType":"backup_run"`) {
		t.Fatalf("audit = %s", audit)
	}
}

func TestBackupDiffAndCopyRoutes(t *testing.T) {
	resetLoginRateLimit(t)
	router := newBackupsRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	viewer := signInWithRole(t, router, admin, "vera", "viewer")
	if response := call(router, http.MethodPut, "/api/v1/backups/config", backupConfigBody, admin); response.Code != http.StatusOK {
		t.Fatalf("config = %d %s", response.Code, response.Body)
	}
	call(router, http.MethodPost, "/api/v1/backups/repository/init", "", admin)
	waitForBody(t, router, "/api/v1/backups/summary", admin, `"initialized":true`)

	// Diff: the paths are for the administrators only.
	response := call(router, http.MethodPost, "/api/v1/backups/operations", `{"operation":"diff","diff":{"from":"0123456789ab","to":"abcdef012345"}}`, admin)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"diff":{"from":"0123456789ab","to":"abcdef012345"`) {
		t.Fatalf("diff = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/backups/runs?limit=1", admin, `"path":"/etc/secret-name.conf"`)
	if body := call(router, http.MethodGet, "/api/v1/backups/runs?limit=1", "", viewer).Body.String(); strings.Contains(body, "secret-name") ||
		!strings.Contains(body, `"changedFiles":1`) || !strings.Contains(body, `"changes":[]`) {
		t.Fatalf("viewer diff = %s", body)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/operations", `{"operation":"diff","diff":{"from":"0123456789ab","to":"0123456789ab"}}`, admin); response.Code != http.StatusBadRequest {
		t.Fatalf("same snapshots = %d", response.Code)
	}

	// Copy repository.
	if response := call(router, http.MethodPost, "/api/v1/backups/copy/init", "", admin); response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "BACKUP_COPY_NOT_CONFIGURED") {
		t.Fatalf("init without copy = %d %s", response.Code, response.Body)
	}
	copyBody := `{"repository":{"type":"rest","location":"https://nas.lan:8000/copy"},"username":"u","secret":"copy-secret","afterBackup":true}`
	if response := call(router, http.MethodPut, "/api/v1/backups/copy", copyBody, viewer); response.Code != http.StatusForbidden {
		t.Fatalf("viewer copy = %d", response.Code)
	}
	if response := call(router, http.MethodPut, "/api/v1/backups/copy", `{"repository":{"type":"local","location":"/mnt/backup"}}`, admin); response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "copy.location") {
		t.Fatalf("copy onto the main repository = %d %s", response.Code, response.Body)
	}
	response = call(router, http.MethodPut, "/api/v1/backups/copy", copyBody, admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"copy":{"repository":{"type":"rest","location":"https://nas.lan:8000/copy"},"initialized":false`) ||
		!strings.Contains(response.Body.String(), `"credentials":{"password":false,"username":true,"secret":true},"afterBackup":true`) ||
		strings.Contains(response.Body.String(), "copy-secret") {
		t.Fatalf("set copy = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/operations", `{"operation":"copy"}`, admin); response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "BACKUP_COPY_NOT_INITIALIZED") {
		t.Fatalf("copy before init = %d %s", response.Code, response.Body)
	}
	response = call(router, http.MethodPost, "/api/v1/backups/copy/init", "", admin)
	if response.Code != http.StatusAccepted || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"password":"`) {
		t.Fatalf("init copy = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/backups/summary", viewer, `"snapshotsCopied":1`)
	waitForBody(t, router, "/api/v1/backups/config", admin, `"initialized":true,"initializedAt":"`)
	if response := call(router, http.MethodPost, "/api/v1/backups/copy/connect", `{"password":"x"}`, admin); response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "BACKUP_COPY_ALREADY_INITIALIZED") {
		t.Fatalf("connect an initialized copy = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/operations", `{"operation":"copy"}`, admin); response.Code != http.StatusAccepted {
		t.Fatalf("copy = %d %s", response.Code, response.Body)
	}
	waitForBody(t, router, "/api/v1/backups/runs?limit=1", admin, `"origin":"manual","requestedBy":"admin","state":"succeeded"`)
	response = call(router, http.MethodPost, "/api/v1/backups/copy/remove", "", admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"copy":null`) {
		t.Fatalf("remove copy = %d %s", response.Code, response.Body)
	}
	audit := call(router, http.MethodGet, "/api/v1/audit?targetType=backup_config", "", admin).Body.String()
	for _, action := range []string{"update_copy", "init_copy", "remove_copy"} {
		if !strings.Contains(audit, `"action":"`+action+`"`) {
			t.Fatalf("audit misses %s: %s", action, audit)
		}
	}
}

func TestBackupSFTPRoutes(t *testing.T) {
	resetLoginRateLimit(t)
	router := newBackupsRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	viewer := signInWithRole(t, router, admin, "vera", "viewer")
	_, hostPublic, err := (sshkey.Keys{}).Generate("root@nas")
	if err != nil {
		t.Fatal(err)
	}
	hostKey := strings.Join(strings.Fields(hostPublic)[:2], " ")
	sftpBody := `{"repository":{"type":"sftp","location":"sftp://backup@nas.lan:2222/srv/restic","hostKey":"` + hostPublic + `"},` +
		`"sources":["/etc"],"excludes":[],"excludeCaches":true,"excludeIfPresent":"","schedule":{"enabled":false,"frequency":"daily","weekday":0,"time":"03:00"}}`

	if response := call(router, http.MethodPut, "/api/v1/backups/config", sftpBody, admin); response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "BACKUP_SSH_KEY_MISSING") {
		t.Fatalf("SFTP before the SSH key = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/backups/ssh-key", "", viewer); response.Code != http.StatusForbidden {
		t.Fatalf("viewer generates the SSH key = %d", response.Code)
	}
	response := call(router, http.MethodPost, "/api/v1/backups/ssh-key", "", admin)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"sshKey":{"publicKey":"ssh-ed25519 `) ||
		!strings.Contains(body, ` homelab-manager-backup","fingerprint":"SHA256:`) || strings.Contains(body, "PRIVATE") {
		t.Fatalf("generate = %d %s", response.Code, body)
	}
	bad := strings.Replace(sftpBody, hostPublic, "nope", 1)
	if response := call(router, http.MethodPut, "/api/v1/backups/config", bad, admin); response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "hostKey") {
		t.Fatalf("invalid host key = %d %s", response.Code, response.Body)
	}
	response = call(router, http.MethodPut, "/api/v1/backups/config", sftpBody, admin)
	body = response.Body.String()
	if response.Code != http.StatusOK ||
		!strings.Contains(body, `"repository":{"type":"sftp","location":"sftp://backup@nas.lan:2222/srv/restic","hostKey":"`+hostKey+`","hostKeyFingerprint":"SHA256:`) {
		t.Fatalf("SFTP config = %d %s", response.Code, body)
	}
	if body := call(router, http.MethodGet, "/api/v1/backups/config", "", viewer).Body.String(); !strings.Contains(body, `"sshKey":{"publicKey":"ssh-ed25519 `) {
		t.Fatalf("viewer config = %s", body)
	}
}

// TestBackupSQLiteDatabasesRoute covers decision D-3: the list is stored,
// served, kept when a request leaves it out, and validated.
func TestBackupSQLiteDatabasesRoute(t *testing.T) {
	resetLoginRateLimit(t)
	router := newBackupsRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	withDatabases := strings.Replace(backupConfigBody, `"excludes":["*.tmp"],`, `"excludes":["*.tmp"],"sqliteDatabases":["/srv/app/data.db"],`, 1)
	response := call(router, http.MethodPut, "/api/v1/backups/config", withDatabases, admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"sqliteDatabases":["/srv/app/data.db"]`) {
		t.Fatalf("config = %d %s", response.Code, response.Body)
	}
	response = call(router, http.MethodPut, "/api/v1/backups/config", backupConfigBody, admin)
	if !strings.Contains(response.Body.String(), `"sqliteDatabases":["/srv/app/data.db"]`) {
		t.Fatalf("an absent list was not kept: %s", response.Body)
	}
	cleared := strings.Replace(backupConfigBody, `"excludes":["*.tmp"],`, `"excludes":["*.tmp"],"sqliteDatabases":[],`, 1)
	if response := call(router, http.MethodPut, "/api/v1/backups/config", cleared, admin); !strings.Contains(response.Body.String(), `"sqliteDatabases":[]`) {
		t.Fatalf("cleared = %s", response.Body)
	}
	invalid := strings.Replace(backupConfigBody, `"excludes":["*.tmp"],`, `"excludes":["*.tmp"],"sqliteDatabases":["data.db"],`, 1)
	if response := call(router, http.MethodPut, "/api/v1/backups/config", invalid, admin); response.Code != http.StatusBadRequest {
		t.Fatalf("relative path = %d", response.Code)
	}
}
