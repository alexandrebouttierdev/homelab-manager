package sqlite

import (
	"context"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

func countRows(t *testing.T, database *Database, table string) int {
	t.Helper()
	var count int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func tableExists(t *testing.T, database *Database, table string) bool {
	t.Helper()
	var count int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		t.Fatalf("look for %s: %v", table, err)
	}
	return count == 1
}

// cpuCycles saves one cpu.percent point every 10 s from start, valued i.
func cpuCycles(t *testing.T, repository *MonitoringRepository, start time.Time, count int) {
	t.Helper()
	samples := make([]domain.MetricSample, 0, count)
	for i := range count {
		samples = append(samples, domain.MetricSample{
			Name: "cpu.percent", Value: float64(i), Unit: "percent",
			Labels: map[string]string{"core": "all"}, RecordedAt: start.Add(time.Duration(i) * 10 * time.Second),
		})
	}
	if err := repository.SaveSystemMetrics(context.Background(), samples); err != nil {
		t.Fatalf("save: %v", err)
	}
}

// Once the raw points are purged, the aggregated history comes from the
// rollups with the same values; a bucket still raw is read from the points.
func TestRollupsKeepTheAggregatedHistoryAfterTheRawPurge(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewMonitoringRepository(database)
	start := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	cpuCycles(t, repository, start, 360) // one hour, 12 buckets of 30 points
	query := domain.MetricQuery{Name: "cpu.percent", From: start, To: start.Add(time.Hour), Limit: 100, Step: time.Hour}
	before, err := repository.ListMetrics(ctx, query)
	if err != nil || len(before) != 1 || before[0].Aggregate.Count != 360 || before[0].Value != 179.5 {
		t.Fatalf("hourly history from the points = (%+v, %v), want the mean 179.5 of 360 points", before, err)
	}

	// Roll up the first 40 minutes only: the history then mixes both.
	if err := repository.RollupMetrics(ctx, start.Add(40*time.Minute)); err != nil {
		t.Fatalf("roll up: %v", err)
	}
	if got := countRows(t, database, "metric_rollups"); got != 8 {
		t.Fatalf("rollups = %d, want 8 buckets of 5 minutes", got)
	}
	mixed, err := repository.ListMetrics(ctx, query)
	if err != nil || len(mixed) != 1 || mixed[0].Aggregate.Count != 360 || mixed[0].Value != 179.5 ||
		mixed[0].Aggregate.Min != 0 || mixed[0].Aggregate.Max != 359 || mixed[0].Labels["core"] != "all" {
		t.Fatalf("hourly history from rollups and points = (%+v, %v), want the same mean", mixed, err)
	}

	if err := repository.RollupMetrics(ctx, start.Add(2*time.Hour)); err != nil {
		t.Fatalf("roll up: %v", err)
	}
	deleted, err := repository.DeleteMetricsBefore(ctx, start.Add(2*time.Hour), start.Add(-time.Hour), 10000)
	if err != nil || deleted != 360 || countRows(t, database, "metric_points") != 0 {
		t.Fatalf("raw purge = (%d, %v), want the 360 points", deleted, err)
	}
	after, err := repository.ListMetrics(ctx, query)
	if err != nil || len(after) != 1 || after[0].Aggregate.Count != 360 || after[0].Value != 179.5 {
		t.Fatalf("hourly history from the rollups = (%+v, %v), want the same mean", after, err)
	}
	fiveMinutes := query
	fiveMinutes.Step = 5 * time.Minute
	buckets, err := repository.ListMetrics(ctx, fiveMinutes)
	if err != nil || len(buckets) != 12 || buckets[1].Value != 44.5 || !buckets[1].RecordedAt.Equal(start.Add(5*time.Minute)) {
		t.Fatalf("5-minute history = (%d buckets, %v), want 12 with a mean of 44.5 in the second", len(buckets), err)
	}
	// Finer steps cannot split a rollup: one point per 5 minutes.
	perMinute := query
	perMinute.Step = time.Minute
	if coarse, err := repository.ListMetrics(ctx, perMinute); err != nil || len(coarse) != 12 {
		t.Fatalf("1-minute history of the rollups = (%d, %v), want 12 points", len(coarse), err)
	}
	if raw, err := repository.ListMetrics(ctx, domain.MetricQuery{Name: "cpu.percent", From: start, To: start.Add(time.Hour), Limit: 100}); err != nil || len(raw) != 0 {
		t.Fatalf("raw history after the purge = (%d, %v), want none", len(raw), err)
	}

	// The rollups follow the retention.
	if deleted, err := repository.DeleteMetricsBefore(ctx, start.Add(2*time.Hour), start.Add(30*time.Minute), 10000); err != nil || deleted != 6 {
		t.Fatalf("rollup purge = (%d, %v), want the 6 buckets ended by +30 min", deleted, err)
	}
}

func TestRollupSkipsGapsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewMonitoringRepository(database)
	first := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	second := first.Add(10 * 24 * time.Hour)
	cpuCycles(t, repository, first, 30)
	cpuCycles(t, repository, second, 30)
	until := second.Add(time.Hour)
	if err := repository.RollupMetrics(ctx, until); err != nil {
		t.Fatalf("roll up: %v", err)
	}
	if err := repository.RollupMetrics(ctx, until); err != nil {
		t.Fatalf("roll up again: %v", err)
	}
	var count, total int
	if err := database.db.QueryRow(`SELECT COUNT(*), SUM(sample_count) FROM metric_rollups`).Scan(&count, &total); err != nil {
		t.Fatal(err)
	}
	if count != 2 || total != 60 {
		t.Fatalf("rollups = %d holding %d points, want 2 buckets of 30 points each", count, total)
	}
	var rolled int64
	if err := database.db.QueryRow(`SELECT rolled_until FROM metric_rollup_state`).Scan(&rolled); err != nil || rolled != until.Unix() {
		t.Fatalf("rolled_until = (%d, %v), want %d", rolled, err, until.Unix())
	}
}

func TestOnePointPerSeriesAndMillisecond(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewMonitoringRepository(database)
	at := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	samples := []domain.MetricSample{
		{Name: "temperature.celsius", Value: 40, Unit: "celsius", Labels: map[string]string{"sensor": "x"}, RecordedAt: at},
		{Name: "temperature.celsius", Value: 41, Unit: "celsius", Labels: map[string]string{"sensor": "x"}, RecordedAt: at},
		{Name: "temperature.celsius", Value: 42, Unit: "celsius", Labels: map[string]string{"sensor": "y"}, RecordedAt: at},
	}
	if err := repository.SaveSystemMetrics(ctx, samples); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := countRows(t, database, "metric_points"); got != 2 {
		t.Fatalf("points = %d, want 2 (the duplicate is dropped)", got)
	}
	if got := countRows(t, database, "metric_series"); got != 2 {
		t.Fatalf("series = %d, want 2", got)
	}
}

// The current host view adds the latest temperature reading to a cycle
// without one, as long as it is recent enough.
func TestLatestHostMetricsAddTheLatestTemperatures(t *testing.T) {
	ctx := context.Background()
	repository := NewMonitoringRepository(openTestDatabase(t, ctx))
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	readAt := now.Add(-40 * time.Second)
	if err := repository.SaveSystemMetrics(ctx, []domain.MetricSample{
		{Name: "memory.used", Value: 1, Unit: "bytes", RecordedAt: readAt},
		{Name: domain.TemperatureMetric, Value: 55, Unit: "celsius", Labels: map[string]string{"sensor": "cpu"}, RecordedAt: readAt},
		{Name: "memory.used", Value: 2, Unit: "bytes", RecordedAt: now},
		{Name: "cpu.load1", Value: 0.5, Unit: "processes", RecordedAt: now},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	latest, err := repository.LatestMetrics(ctx, nil, now.Add(-5*time.Minute))
	if err != nil || len(latest) != 3 {
		t.Fatalf("latest = (%+v, %v), want the last cycle and the temperature", latest, err)
	}
	names := []string{latest[0].Name, latest[1].Name, latest[2].Name}
	if names[0] != "cpu.load1" || names[1] != "memory.used" || names[2] != domain.TemperatureMetric {
		t.Fatalf("latest names = %v, want them sorted", names)
	}
	if latest[1].Value != 2 || !latest[2].RecordedAt.Equal(readAt) || latest[2].Value != 55 {
		t.Fatalf("latest = %+v, want memory of the last cycle and the temperature read at %v", latest, readAt)
	}
	stale, err := repository.LatestMetrics(ctx, nil, now.Add(-10*time.Second))
	if err != nil || len(stale) != 2 {
		t.Fatalf("latest without a recent temperature = (%+v, %v), want the cycle only", stale, err)
	}
}

func TestDeleteUnusedSeries(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewMonitoringRepository(database)
	old := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	if err := repository.SaveSystemMetrics(ctx, []domain.MetricSample{
		{Name: "network.rx_bytes", Value: 1, Unit: "bytes", Labels: map[string]string{"interface": "veth1"}, RecordedAt: old},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := repository.RollupMetrics(ctx, old.Add(time.Hour)); err != nil {
		t.Fatalf("roll up: %v", err)
	}
	if deleted, err := repository.DeleteUnusedSeries(ctx); err != nil || deleted != 0 {
		t.Fatalf("delete a series with data = (%d, %v), want 0", deleted, err)
	}
	if _, err := repository.DeleteMetricsBefore(ctx, old.Add(time.Hour), old.Add(time.Hour), 100); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted, err := repository.DeleteUnusedSeries(ctx); err != nil || deleted != 1 {
		t.Fatalf("delete an empty series = (%d, %v), want 1", deleted, err)
	}
	// The cached identifier is gone too: the series is created again.
	if err := repository.SaveSystemMetrics(ctx, []domain.MetricSample{
		{Name: "network.rx_bytes", Value: 2, Unit: "bytes", Labels: map[string]string{"interface": "veth1"}, RecordedAt: old.Add(2 * time.Hour)},
	}); err != nil {
		t.Fatalf("save after the purge: %v", err)
	}
	if got := countRows(t, database, "metric_series"); got != 1 {
		t.Fatalf("series = %d, want 1", got)
	}
}

// The former sample tables are converted in batches: recent rows become raw
// points, older ones rollups, then the tables are dropped.
func TestConvertLegacyMetrics(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	repository := NewMonitoringRepository(database)
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	rawSince := now.Add(-24 * time.Hour)

	dockerID := "abcdef012345"
	if _, err := repository.ReconcileContainers(ctx, []domain.Container{{
		DockerID: dockerID, Name: "web", Image: "nginx", State: "running", CreatedAt: now,
	}}, now); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	container, err := repository.GetContainer(ctx, dockerID)
	if err != nil {
		t.Fatal(err)
	}
	// More rows than one batch: 2 days of a host series every 60 s, with
	// fractional seconds, half of them older than rawSince.
	insertHost, err := database.db.Prepare(`INSERT INTO system_metric_samples (id, name, value, unit, labels, recorded_at) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer insertHost.Close()
	start := now.Add(-48 * time.Hour)
	const hostRows = 2880
	for i := range hostRows {
		at := start.Add(time.Duration(i)*time.Minute + 250*time.Millisecond)
		if _, err := insertHost.Exec(uuid.NewString(), "cpu.percent", float64(i%10), "percent", `{"core":"all"}`, at); err != nil {
			t.Fatalf("insert legacy host row: %v", err)
		}
	}
	if _, err := database.db.Exec(`INSERT INTO container_metric_samples (id, container_id, name, value, unit, labels, recorded_at) VALUES (?, ?, ?, ?, ?, NULL, ?), (?, ?, ?, ?, ?, NULL, ?)`,
		uuid.NewString(), container.ID.String(), "memory.usage_bytes", 100.0, "bytes", now.Add(-30*time.Hour),
		uuid.NewString(), container.ID.String(), "memory.usage_bytes", 200.0, "bytes", now.Add(-time.Hour)); err != nil {
		t.Fatalf("insert legacy container rows: %v", err)
	}

	// A deadline in the middle: the next call goes on where it stopped.
	expired, cancel := context.WithCancel(ctx)
	cancel()
	if _, done, err := repository.ConvertLegacyMetrics(expired, rawSince); done || err == nil {
		t.Fatalf("conversion with an expired context = (%v, %v), want unfinished", done, err)
	}
	converted, done, err := repository.ConvertLegacyMetrics(ctx, rawSince)
	if err != nil || !done || converted != hostRows+2 {
		t.Fatalf("conversion = (%d, %v, %v), want %d rows, done", converted, done, err, hostRows+2)
	}
	if tableExists(t, database, "system_metric_samples") || tableExists(t, database, "container_metric_samples") {
		t.Fatal("the former tables must be dropped once converted")
	}
	if again, done, err := repository.ConvertLegacyMetrics(ctx, rawSince); err != nil || !done || again != 0 {
		t.Fatalf("second conversion = (%d, %v, %v), want nothing to do", again, done, err)
	}

	// Recent rows are raw points with their milliseconds.
	recent, err := repository.ListMetrics(ctx, domain.MetricQuery{Name: "cpu.percent", From: rawSince, To: now, Limit: 2000,
		Labels: map[string]string{"core": "all"}})
	if err != nil || len(recent) != 1440 {
		t.Fatalf("raw points since rawSince = (%d, %v), want 1440", len(recent), err)
	}
	if recent[0].RecordedAt.Nanosecond() != 250*int(time.Millisecond) {
		t.Fatalf("converted time = %v, want its 250 ms kept", recent[0].RecordedAt)
	}
	// Older rows are rollups: every row is counted exactly once.
	if err := repository.RollupMetrics(ctx, now); err != nil {
		t.Fatalf("roll up: %v", err)
	}
	var points int
	if err := database.db.QueryRow(`SELECT SUM(sample_count) FROM metric_rollups`).Scan(&points); err != nil || points != hostRows+2 {
		t.Fatalf("points counted in the rollups = (%d, %v), want %d", points, err, hostRows+2)
	}
	daily, err := repository.ListMetrics(ctx, domain.MetricQuery{Name: "cpu.percent", From: start, To: now, Limit: 2000, Step: time.Hour})
	if err != nil || len(daily) != 48 || daily[0].Value != 4.5 || daily[0].Aggregate.Count != 60 {
		t.Fatalf("hourly history = (%d, %v), want 48 hours of mean 4.5", len(daily), err)
	}
	containerHistory, err := repository.ListMetrics(ctx, domain.MetricQuery{ContainerID: &dockerID, Name: "memory.usage_bytes",
		From: now.Add(-48 * time.Hour), To: now, Limit: 100, Step: time.Hour})
	if err != nil || len(containerHistory) != 2 || containerHistory[0].Value != 100 || containerHistory[1].Value != 200 {
		t.Fatalf("container history = (%+v, %v), want both converted rows", containerHistory, err)
	}
}
