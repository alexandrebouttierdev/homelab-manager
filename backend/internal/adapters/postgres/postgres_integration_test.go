package postgres

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"homelab-manager/backend/ent"
	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

func TestPostgresIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, databaseURL := openIntegrationDatabase(t, ctx)

	if err := ApplyMigrations(ctx, databaseURL); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}

	authRepository := NewAuthRepository(client)
	username := "integration-" + uuid.NewString()
	if err := authRepository.EnsureAdmin(ctx, username, "test-password-hash"); err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	user, _, err := authRepository.FindByUsername(ctx, username)
	if err != nil {
		t.Fatalf("find integration user: %v", err)
	}

	tokenHash := uuid.NewString()
	session, err := authRepository.Create(ctx, user.ID, tokenHash, "127.0.0.1", "integration-test", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	activeSession, err := authRepository.FindActiveByTokenHash(ctx, tokenHash)
	if err != nil || activeSession.ID != session.ID {
		t.Fatalf("find active session = (%v, %v), want session %s", activeSession, err, session.ID)
	}

	repository := NewMonitoringRepository(client)
	host := domain.HostInfo{
		Hostname: "integration-host", Distribution: "Debian", OSVersion: "13",
		KernelVersion: "test-kernel",
	}
	if err := repository.SaveHost(ctx, host); err != nil {
		t.Fatalf("save host: %v", err)
	}
	if err := repository.SaveHost(ctx, host); err != nil {
		t.Fatalf("update host: %v", err)
	}
	savedHost, err := repository.GetHost(ctx)
	if err != nil || savedHost.Hostname != host.Hostname {
		t.Fatalf("get host = (%v, %v), want hostname %q", savedHost, err, host.Hostname)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	old := now.Add(-48 * time.Hour)
	systemSample := domain.MetricSample{
		ID: uuid.New(), Name: "cpu.percent", Value: 12.5, Unit: "percent", RecordedAt: old,
	}
	if err := repository.SaveSystemMetrics(ctx, []domain.MetricSample{systemSample}); err != nil {
		t.Fatalf("save system metrics: %v", err)
	}
	systemMetrics, err := repository.ListMetrics(ctx, nil, "cpu.percent", old.Add(-time.Second), old.Add(time.Second), 10)
	if err != nil || len(systemMetrics) != 1 || systemMetrics[0].Value != systemSample.Value {
		t.Fatalf("list system metrics = (%v, %v), want saved sample", systemMetrics, err)
	}

	dockerID := "integration-" + uuid.NewString()
	observedAt := now.Add(-time.Minute)
	discoveryEvents, err := repository.ReconcileContainers(ctx, []domain.Container{{
		DockerID: dockerID, Name: "integration-container", Image: "test:latest",
		State: "running", Status: "Up", RestartCount: 0, CreatedAt: observedAt,
	}}, observedAt)
	if err != nil || len(discoveryEvents) != 1 {
		t.Fatalf("discover container = (%v, %v), want one event", discoveryEvents, err)
	}

	containers, err := repository.ListContainers(ctx, false, 10)
	if err != nil || len(containers) != 1 {
		t.Fatalf("list containers = (%v, %v), want one container", containers, err)
	}
	container, err := repository.GetContainer(ctx, dockerID)
	if err != nil || container.Name != "integration-container" {
		t.Fatalf("get container = (%v, %v), want discovered container", container, err)
	}

	containerSample := domain.MetricSample{
		ID: uuid.New(), ContainerID: &container.ID, Name: "cpu.percent",
		Value: 4.25, Unit: "percent", RecordedAt: old,
	}
	if err := repository.SaveContainerMetrics(ctx, []domain.MetricSample{containerSample}); err != nil {
		t.Fatalf("save container metrics: %v", err)
	}
	containerMetrics, err := repository.ListMetrics(ctx, &dockerID, "cpu.percent", old.Add(-time.Second), old.Add(time.Second), 10)
	if err != nil || len(containerMetrics) != 1 || containerMetrics[0].Value != containerSample.Value {
		t.Fatalf("list container metrics = (%v, %v), want saved sample", containerMetrics, err)
	}

	changedAt := now
	changedEvents, err := repository.ReconcileContainers(ctx, []domain.Container{{
		DockerID: dockerID, Name: "integration-container", Image: "test:latest",
		State: "exited", Status: "Exited", RestartCount: 1, CreatedAt: observedAt,
	}}, changedAt)
	if err != nil || len(changedEvents) != 1 || changedEvents[0].Type != "container.restarted" {
		t.Fatalf("update container = (%v, %v), want restart event", changedEvents, err)
	}
	events, err := repository.ListEvents(ctx, &dockerID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	if err != nil || len(events) != 2 {
		t.Fatalf("list container events = (%v, %v), want discovery and restart events", events, err)
	}

	deletedMetrics, err := repository.DeleteMetricsBefore(ctx, now.Add(-24*time.Hour), 10)
	if err != nil || deletedMetrics != 2 {
		t.Fatalf("delete old metrics = (%d, %v), want 2", deletedMetrics, err)
	}
	deletedEvents, err := repository.DeleteEventsBefore(ctx, now.Add(time.Hour), 10)
	if err != nil || deletedEvents != 2 {
		t.Fatalf("delete events = (%d, %v), want 2", deletedEvents, err)
	}

	if err := authRepository.RevokeByTokenHash(ctx, tokenHash); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	if _, err := authRepository.FindActiveByTokenHash(ctx, tokenHash); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("find revoked session error = %v, want ErrSessionNotFound", err)
	}
}

func openIntegrationDatabase(t *testing.T, ctx context.Context) (*ent.Client, string) {
	t.Helper()
	rawURL := os.Getenv("HOMELAB_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set HOMELAB_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		t.Fatalf("HOMELAB_TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	databaseName := strings.TrimPrefix(parsed.Path, "/")
	if strings.Contains(databaseName, "/") || !strings.HasSuffix(databaseName, "_test") {
		t.Fatalf("HOMELAB_TEST_DATABASE_URL must target a database whose name ends in _test")
	}

	adminDB, err := sql.Open("postgres", rawURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := adminDB.PingContext(ctx); err != nil {
		adminDB.Close()
		t.Fatalf("connect to test database: %v", err)
	}

	schema := "homelab_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		adminDB.Close()
		t.Fatalf("create isolated integration schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adminDB.ExecContext(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("drop isolated integration schema: %v", err)
		}
		if err := adminDB.Close(); err != nil {
			t.Errorf("close integration database connection: %v", err)
		}
	})

	query := parsed.Query()
	options := query.Get("options")
	if options != "" {
		options += " "
	}
	query.Set("options", options+"-c search_path="+schema)
	parsed.RawQuery = query.Encode()
	isolatedURL := parsed.String()
	if err := ApplyMigrations(ctx, isolatedURL); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	client, err := ent.Open("postgres", isolatedURL)
	if err != nil {
		t.Fatalf("open Ent client: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close Ent client: %v", err)
		}
	})
	return client, isolatedURL
}
