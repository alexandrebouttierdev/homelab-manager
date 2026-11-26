package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/migrations"

	"github.com/google/uuid"
)

func TestSQLiteIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database := openTestDatabase(t, ctx)
	client := database.Client()

	authRepository := NewAuthRepository(client)
	username := "integration-" + uuid.NewString()
	if created, err := authRepository.EnsureAdmin(ctx, username, "test-password-hash"); err != nil || !created {
		t.Fatalf("create integration user = (%v, %v), want created", created, err)
	}
	if created, err := authRepository.EnsureAdmin(ctx, username, "other-hash"); err != nil || created {
		t.Fatalf("ensure existing user = (%v, %v), want untouched", created, err)
	}
	if _, storedHash, err := authRepository.FindByUsername(ctx, username); err != nil || storedHash != "test-password-hash" {
		t.Fatalf("existing user password hash changed: (%q, %v)", storedHash, err)
	}
	user, _, err := authRepository.FindByUsername(ctx, username)
	if err != nil {
		t.Fatalf("find integration user: %v", err)
	}

	tokenHash := uuid.NewString()
	session, err := authRepository.Create(ctx, user.ID, tokenHash, "127.0.0.1", "integration-test", "password", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	activeSession, err := authRepository.FindActiveByTokenHash(ctx, tokenHash)
	if err != nil || activeSession.ID != session.ID {
		t.Fatalf("find active session = (%v, %v), want session %s", activeSession, err, session.ID)
	}

	repository := NewMonitoringRepository(database)
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
		Name: "cpu.percent", Value: 12.5, Unit: "percent", RecordedAt: old,
	}
	if err := repository.SaveSystemMetrics(ctx, []domain.MetricSample{systemSample}); err != nil {
		t.Fatalf("save system metrics: %v", err)
	}
	systemMetrics, err := repository.ListMetrics(ctx, domain.MetricQuery{ContainerID: nil, Name: "cpu.percent", From: old.Add(-time.Second), To: old.Add(time.Second), Limit: 10})
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
		ContainerID: &container.ID, Name: "cpu.percent",
		Value: 4.25, Unit: "percent", RecordedAt: old,
	}
	if err := repository.SaveContainerMetrics(ctx, []domain.MetricSample{containerSample}); err != nil {
		t.Fatalf("save container metrics: %v", err)
	}
	containerMetrics, err := repository.ListMetrics(ctx, domain.MetricQuery{ContainerID: &dockerID, Name: "cpu.percent", From: old.Add(-time.Second), To: old.Add(time.Second), Limit: 10})
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
	events, err := repository.ListEvents(ctx, &dockerID, now.Add(-time.Hour), now.Add(time.Hour), nil, 10)
	if err != nil || len(events) != 2 {
		t.Fatalf("list container events = (%v, %v), want discovery and restart events", events, err)
	}

	// Raw points are only purged once rolled up; their rollups stay.
	if deleted, err := repository.DeleteMetricsBefore(ctx, now.Add(-24*time.Hour), old.Add(-time.Hour), 10); err != nil || deleted != 0 {
		t.Fatalf("delete points not rolled up = (%d, %v), want 0", deleted, err)
	}
	if err := repository.RollupMetrics(ctx, now); err != nil {
		t.Fatalf("roll up: %v", err)
	}
	deletedMetrics, err := repository.DeleteMetricsBefore(ctx, now.Add(-24*time.Hour), old.Add(-time.Hour), 10)
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

func TestOpenReappliesMigrationsAndKeepsData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state", "monoops.db")

	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := NewAuthRepository(first.Client()).EnsureAdmin(ctx, "admin", "hash"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	second, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen with migrations already applied: %v", err)
	}
	defer second.Close()
	if _, _, err := NewAuthRepository(second.Client()).FindByUsername(ctx, "admin"); err != nil {
		t.Fatalf("admin not persisted across reopen: %v", err)
	}

	var journalMode string
	var foreignKeys, busyTimeoutMillis int
	if err := second.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil || journalMode != "wal" {
		t.Fatalf("journal_mode = (%q, %v), want wal", journalMode, err)
	}
	if err := second.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign_keys = (%d, %v), want 1", foreignKeys, err)
	}
	if err := second.db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeoutMillis); err != nil || busyTimeoutMillis != int(busyTimeout.Milliseconds()) {
		t.Fatalf("busy_timeout = (%d, %v), want %d", busyTimeoutMillis, err, busyTimeout.Milliseconds())
	}
	// Every embedded migration is recorded exactly once, however many there are.
	files, err := migrations.Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	wantMigrations := 0
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".sql" {
			wantMigrations++
		}
	}
	var migrationsApplied int
	if err := second.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migrationsApplied); err != nil || migrationsApplied != wantMigrations {
		t.Fatalf("schema_migrations rows = (%d, %v), want %d", migrationsApplied, err, wantMigrations)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("database file mode = %o, want 600", mode)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("expected an empty database path to be rejected")
	}
}

// Times reach the repository in arbitrary zones; range filters must still be
// chronological because SQLite compares the stored timestamps as text.
func TestMetricRangesAreTimezoneIndependent(t *testing.T) {
	ctx := context.Background()
	repository := NewMonitoringRepository(openTestDatabase(t, ctx))

	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}
	base := time.Date(2026, 3, 29, 0, 30, 0, 0, time.UTC) // around a DST change
	samples := make([]domain.MetricSample, 0, 6)
	for i := range 6 {
		recordedAt := base.Add(time.Duration(i) * 30 * time.Minute)
		if i%2 == 0 {
			recordedAt = recordedAt.In(paris)
		}
		if i == 3 {
			recordedAt = recordedAt.Add(500 * time.Millisecond) // fractional seconds
		}
		samples = append(samples, domain.MetricSample{
			Name: "memory.used_percent", Value: float64(i), Unit: "percent", RecordedAt: recordedAt,
		})
	}
	if err := repository.SaveSystemMetrics(ctx, samples); err != nil {
		t.Fatalf("save metrics: %v", err)
	}

	from := base.Add(30 * time.Minute).In(paris)
	to := base.Add(2 * time.Hour)
	got, err := repository.ListMetrics(ctx, domain.MetricQuery{ContainerID: nil, Name: "memory.used_percent", From: from, To: to, Limit: 100})
	if err != nil {
		t.Fatalf("list metrics: %v", err)
	}
	want := []float64{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got %d samples (%v), want values %v", len(got), got, want)
	}
	for i, sample := range got {
		if sample.Value != want[i] {
			t.Fatalf("sample %d value = %v, want %v (order must be chronological)", i, sample.Value, want[i])
		}
		if sample.RecordedAt.Location() != time.UTC {
			t.Fatalf("sample %d location = %v, want UTC", i, sample.RecordedAt.Location())
		}
		if !sample.RecordedAt.Equal(samples[int(want[i])].RecordedAt) {
			t.Fatalf("sample %d time = %v, want %v", i, sample.RecordedAt, samples[int(want[i])].RecordedAt)
		}
	}

	if err := repository.RollupMetrics(ctx, base.Add(3*time.Hour)); err != nil {
		t.Fatalf("roll up: %v", err)
	}
	deleted, err := repository.DeleteMetricsBefore(ctx, base.Add(time.Hour).In(paris), base.Add(-time.Hour), 100)
	if err != nil || deleted != 2 {
		t.Fatalf("delete before = (%d, %v), want 2", deleted, err)
	}
}

// Collectors, the retention purge and HTTP readers share the database.
func TestConcurrentWritersAndReaders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := NewMonitoringRepository(openTestDatabase(t, ctx))

	const workers, batches, perBatch = 4, 8, 25
	start := time.Now().UTC().Add(-time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, workers*2)
	for worker := range workers {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for batch := range batches {
				samples := make([]domain.MetricSample, 0, perBatch)
				for i := range perBatch {
					// One series per writer: a series has one point per millisecond.
					samples = append(samples, domain.MetricSample{
						Name: "cpu.percent", Value: float64(worker), Unit: "percent",
						Labels:     map[string]string{"core": strconv.Itoa(worker)},
						RecordedAt: start.Add(time.Duration(batch*perBatch+i) * time.Millisecond),
					})
				}
				if err := repository.SaveSystemMetrics(ctx, samples); err != nil {
					errs <- err
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			for range batches {
				if _, err := repository.ListMetrics(ctx, domain.MetricQuery{ContainerID: nil, Name: "cpu.percent", From: start, To: start.Add(time.Hour), Limit: 2000}); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent access failed: %v", err)
	}
	got, err := repository.ListMetrics(ctx, domain.MetricQuery{ContainerID: nil, Name: "cpu.percent", From: start, To: start.Add(time.Hour), Limit: 2000})
	if err != nil || len(got) != workers*batches*perBatch {
		t.Fatalf("stored samples = (%d, %v), want %d", len(got), err, workers*batches*perBatch)
	}
}

func openTestDatabase(t *testing.T, ctx context.Context) *Database {
	t.Helper()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "monoops.db"))
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})
	return database
}

// The current view must contain the whole latest cycle, however many series
// the host has, and nothing from older cycles.
func TestLatestMetricsReturnsTheWholeLatestCycle(t *testing.T) {
	ctx := context.Background()
	repository := NewMonitoringRepository(openTestDatabase(t, ctx))
	now := time.Now().UTC().Truncate(time.Second)

	const seriesPerCycle = 250 // more than the former 100-sample dashboard cap
	for cycle := range 3 {
		recordedAt := now.Add(time.Duration(cycle-2) * 10 * time.Second)
		samples := make([]domain.MetricSample, 0, seriesPerCycle)
		for series := range seriesPerCycle {
			samples = append(samples, domain.MetricSample{
				Name: "network.rx_bytes", Value: float64(cycle), Unit: "bytes",
				Labels: map[string]string{"interface": "veth" + strconv.Itoa(series)}, RecordedAt: recordedAt,
			})
		}
		if err := repository.SaveSystemMetrics(ctx, samples); err != nil {
			t.Fatalf("save cycle %d: %v", cycle, err)
		}
	}
	latest, err := repository.LatestMetrics(ctx, nil, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("latest metrics: %v", err)
	}
	if len(latest) != seriesPerCycle {
		t.Fatalf("latest cycle has %d samples, want %d", len(latest), seriesPerCycle)
	}
	for _, sample := range latest {
		if sample.Value != 2 || !sample.RecordedAt.Equal(now) {
			t.Fatalf("sample from an older cycle returned: %+v", sample)
		}
	}
	if empty, err := repository.LatestMetrics(ctx, nil, now.Add(time.Minute)); err != nil || len(empty) != 0 {
		t.Fatalf("no cycle after since = (%v, %v), want empty", empty, err)
	}

	dockerID := "abcdef012345"
	if _, err := repository.ReconcileContainers(ctx, []domain.Container{{
		DockerID: dockerID, Name: "web", Image: "nginx", State: "running", CreatedAt: now,
	}}, now); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	container, err := repository.GetContainer(ctx, dockerID)
	if err != nil {
		t.Fatalf("get container: %v", err)
	}
	if err := repository.SaveContainerMetrics(ctx, []domain.MetricSample{
		{ContainerID: &container.ID, Name: "cpu.percent", Value: 1, Unit: "percent", RecordedAt: now.Add(-15 * time.Second)},
		{ContainerID: &container.ID, Name: "cpu.percent", Value: 2, Unit: "percent", RecordedAt: now},
		{ContainerID: &container.ID, Name: "restart.count", Value: 0, Unit: "count", RecordedAt: now},
	}); err != nil {
		t.Fatalf("save container metrics: %v", err)
	}
	containerLatest, err := repository.LatestMetrics(ctx, &dockerID, now.Add(-time.Minute))
	if err != nil || len(containerLatest) != 2 {
		t.Fatalf("container latest = (%v, %v), want cpu.percent and restart.count of the last cycle", containerLatest, err)
	}
	missing := "ffffffffffff"
	if _, err := repository.LatestMetrics(ctx, &missing, now.Add(-time.Minute)); !errors.Is(err, domain.ErrContainerNotFound) {
		t.Fatalf("unknown container error = %v, want ErrContainerNotFound", err)
	}
}

func TestAggregatedHistoryAndLabelFilter(t *testing.T) {
	ctx := context.Background()
	repository := NewMonitoringRepository(openTestDatabase(t, ctx))
	start := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	samples := make([]domain.MetricSample, 0)
	// Two series every 10 s for 10 minutes: core=all at value i, core=0 at 100+i.
	for i := range 60 {
		at := start.Add(time.Duration(i) * 10 * time.Second)
		samples = append(samples,
			domain.MetricSample{Name: "cpu.percent", Value: float64(i), Unit: "percent", Labels: map[string]string{"core": "all"}, RecordedAt: at},
			domain.MetricSample{Name: "cpu.percent", Value: float64(100 + i), Unit: "percent", Labels: map[string]string{"core": "0"}, RecordedAt: at},
		)
	}
	if err := repository.SaveSystemMetrics(ctx, samples); err != nil {
		t.Fatalf("save: %v", err)
	}
	window := domain.MetricQuery{Name: "cpu.percent", From: start, To: start.Add(10 * time.Minute), Limit: 2000}

	filtered := window
	filtered.Labels = map[string]string{"core": "all"}
	raw, err := repository.ListMetrics(ctx, filtered)
	if err != nil || len(raw) != 60 || raw[0].Labels["core"] != "all" || raw[0].Aggregate != nil {
		t.Fatalf("raw filtered = (%d samples, %v), want 60 core=all raw samples", len(raw), err)
	}

	perFiveMinutes := filtered
	perFiveMinutes.Step = 5 * time.Minute
	buckets, err := repository.ListMetrics(ctx, perFiveMinutes)
	if err != nil || len(buckets) != 2 {
		t.Fatalf("5m buckets = (%v, %v), want 2", buckets, err)
	}
	first := buckets[0]
	if !first.RecordedAt.Equal(start) || first.Aggregate == nil || first.Aggregate.Count != 30 ||
		first.Value != 14.5 || first.Aggregate.Min != 0 || first.Aggregate.Max != 29 || first.Unit != "percent" {
		t.Fatalf("first bucket = %+v (aggregate %+v), want mean 14.5 of 30 samples from %v", first, first.Aggregate, start)
	}
	if !buckets[1].RecordedAt.Equal(start.Add(5*time.Minute)) || buckets[1].Value != 44.5 {
		t.Fatalf("second bucket = %+v, want mean 44.5 at +5 min", buckets[1])
	}

	allSeries := window
	allSeries.Step = time.Minute
	perMinute, err := repository.ListMetrics(ctx, allSeries)
	if err != nil || len(perMinute) != 20 {
		t.Fatalf("1m buckets for 2 series = (%d, %v), want 20", len(perMinute), err)
	}
	for i := 1; i < len(perMinute); i++ {
		if perMinute[i].RecordedAt.Before(perMinute[i-1].RecordedAt) {
			t.Fatal("aggregated samples must be in chronological order")
		}
	}

	tooSmall := allSeries
	tooSmall.Limit = 19
	if _, err := repository.ListMetrics(ctx, tooSmall); !errors.Is(err, domain.ErrTooManyPoints) {
		t.Fatalf("over-limit aggregation error = %v, want ErrTooManyPoints", err)
	}

	injected := window
	injected.Labels = map[string]string{`core") OR 1=1 --`: "x"}
	if _, err := repository.ListMetrics(ctx, injected); err == nil {
		t.Fatal("a label key outside [a-z0-9_] must be rejected")
	}
}

func TestSettingsRepositoryStoresRetentionWithAudit(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewSettingsRepository(database)

	if days, err := repository.RetentionDays(ctx); err != nil || days != nil {
		t.Fatalf("initial retention = (%v, %v), want nil (configuration applies)", days, err)
	}
	audit := domain.AuditEntry{Actor: "admin", TargetType: "settings", TargetID: "metrics_retention_days", Action: "update", Outcome: "success", RequestID: "req-1"}
	for _, days := range []int{14, 7} {
		if err := repository.SetRetentionDays(ctx, days, audit); err != nil {
			t.Fatalf("set %d: %v", days, err)
		}
	}
	if days, err := repository.RetentionDays(ctx); err != nil || days == nil || *days != 7 {
		t.Fatalf("stored retention = (%v, %v), want 7", days, err)
	}
	entries, err := database.Client().AuditLog.Query().All(ctx)
	if err != nil || len(entries) != 2 || entries[0].Actor != "admin" || entries[0].RequestID != "req-1" {
		t.Fatalf("audit entries = (%v, %v), want two entries by admin", entries, err)
	}
	if err := repository.SetRetentionDays(ctx, 400, audit); err == nil {
		t.Fatal("the database must reject a retention above 365 days")
	}
	if days, _ := repository.RetentionDays(ctx); days == nil || *days != 7 {
		t.Fatalf("a rejected update must leave the retention unchanged, got %v", days)
	}
}
