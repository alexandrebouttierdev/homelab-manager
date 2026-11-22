package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"

	"go.uber.org/zap"
)

type testSystemCollector struct{}

func (c *testSystemCollector) Collect(context.Context) (domain.HostInfo, []domain.MetricSample, error) {
	return domain.HostInfo{Hostname: "test"}, nil, nil
}

type unavailableDocker struct{}

func (unavailableDocker) ListContainers(context.Context) ([]domain.Container, error) {
	return nil, errors.New("Docker unavailable")
}

func (unavailableDocker) ContainerStats(context.Context, string) ([]domain.MetricSample, error) {
	return nil, errors.New("Docker unavailable")
}

type testMonitoringRepository struct{}

func (testMonitoringRepository) SaveHost(context.Context, domain.HostInfo) error { return nil }
func (testMonitoringRepository) GetHost(context.Context) (domain.HostInfo, error) {
	return domain.HostInfo{}, nil
}
func (testMonitoringRepository) SaveSystemMetrics(context.Context, []domain.MetricSample) error {
	return nil
}
func (testMonitoringRepository) ReconcileContainers(context.Context, []domain.Container, time.Time) ([]domain.MonitorEvent, error) {
	return nil, nil
}
func (testMonitoringRepository) SaveContainerMetrics(context.Context, []domain.MetricSample) error {
	return nil
}
func (testMonitoringRepository) ListContainers(context.Context, bool, int) ([]domain.Container, error) {
	return nil, nil
}
func (testMonitoringRepository) GetContainer(context.Context, string) (domain.Container, error) {
	return domain.Container{}, domain.ErrContainerNotFound
}
func (testMonitoringRepository) ListMetrics(context.Context, *string, string, time.Time, time.Time, int) ([]domain.MetricSample, error) {
	return nil, nil
}
func (testMonitoringRepository) LatestMetrics(context.Context, *string, time.Time) ([]domain.MetricSample, error) {
	return nil, nil
}
func (testMonitoringRepository) ListEvents(context.Context, *string, time.Time, time.Time, int) ([]domain.MonitorEvent, error) {
	return nil, nil
}
func (testMonitoringRepository) DeleteMetricsBefore(context.Context, time.Time, int) (int, error) {
	return 0, nil
}
func (testMonitoringRepository) DeleteEventsBefore(context.Context, time.Time, int) (int, error) {
	return 0, nil
}

func TestMonitoringRunKeepsSystemCollectionIndependentAndStops(t *testing.T) {
	system := &testSystemCollector{}
	monitoring := NewMonitoringUsecase(
		system, unavailableDocker{}, testMonitoringRepository{},
		MonitoringConfig{
			SystemInterval: time.Hour, DockerStatsInterval: time.Hour,
			DockerScanInterval: time.Hour, Retention: 24 * time.Hour,
		},
		zap.NewNop(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		monitoring.Run(ctx)
		close(done)
	}()
	deadline := time.After(time.Second)
	for {
		database, systemStatus, dockerStatus := monitoring.HealthStatus()
		if database.Status == "ok" && systemStatus.Status == "ok" && dockerStatus.Status == "unavailable" {
			break
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatalf("unexpected worker health: database=%q system=%q docker=%q", database.Status, systemStatus.Status, dockerStatus.Status)
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitoring workers did not stop after cancellation")
	}
}

func TestStatusReportsStaleAfterThreeMissedCycles(t *testing.T) {
	monitoring := NewMonitoringUsecase(&testSystemCollector{}, unavailableDocker{}, testMonitoringRepository{},
		MonitoringConfig{SystemInterval: 10 * time.Second, DockerScanInterval: 30 * time.Second}, zap.NewNop())
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	monitoring.now = func() time.Time { return now }

	recent := now.Add(-29 * time.Second)
	old := now.Add(-91 * time.Second)
	monitoring.setSystemStatus(StatusOK, &recent)
	monitoring.setDockerStatus(StatusOK, &old)
	system, docker := monitoring.Status()
	if system.Status != StatusOK {
		t.Fatalf("system status = %q, want ok (29 s < 3 × 10 s)", system.Status)
	}
	if docker.Status != StatusStale || docker.LastSuccessAt == nil || !docker.LastSuccessAt.Equal(old) {
		t.Fatalf("docker status = %+v, want stale with the last success kept", docker)
	}

	monitoring.setDockerStatus(StatusUnavailable, nil)
	if _, docker := monitoring.Status(); docker.Status != StatusUnavailable {
		t.Fatalf("an unavailable collector must stay unavailable, got %q", docker.Status)
	}
}

type statsDocker struct{ recordedAt time.Time }

func (statsDocker) ListContainers(context.Context) ([]domain.Container, error) { return nil, nil }
func (d statsDocker) ContainerStats(context.Context, string) ([]domain.MetricSample, error) {
	return []domain.MetricSample{{Name: "cpu.percent", Value: 1, Unit: "percent", RecordedAt: d.recordedAt}}, nil
}

type capturingRepository struct {
	testMonitoringRepository
	containers []domain.Container
	saved      []domain.MetricSample
}

func (r *capturingRepository) ListContainers(context.Context, bool, int) ([]domain.Container, error) {
	return r.containers, nil
}
func (r *capturingRepository) SaveContainerMetrics(_ context.Context, samples []domain.MetricSample) error {
	r.saved = append(r.saved, samples...)
	return nil
}

func TestContainerRestartCountSharesTheStatsCycle(t *testing.T) {
	statsAt := time.Date(2026, 11, 22, 12, 0, 3, 0, time.UTC)
	cycleAt := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	running := domain.Container{ID: uuid.New(), DockerID: "aaa", State: "running", RestartCount: 4}
	stopped := domain.Container{ID: uuid.New(), DockerID: "bbb", State: "exited", RestartCount: 1}
	repository := &capturingRepository{containers: []domain.Container{running, stopped}}
	monitoring := NewMonitoringUsecase(&testSystemCollector{}, statsDocker{recordedAt: statsAt}, repository,
		MonitoringConfig{DockerStatsInterval: 15 * time.Second}, zap.NewNop())
	monitoring.now = func() time.Time { return cycleAt }

	if err := monitoring.collectContainerMetrics(context.Background()); err != nil {
		t.Fatalf("collect: %v", err)
	}
	restartAt := map[uuid.UUID]time.Time{}
	for _, sample := range repository.saved {
		if sample.Name == "restart.count" {
			restartAt[*sample.ContainerID] = sample.RecordedAt
		}
	}
	if !restartAt[running.ID].Equal(statsAt) {
		t.Fatalf("running container restart.count at %v, want the stats timestamp %v", restartAt[running.ID], statsAt)
	}
	if !restartAt[stopped.ID].Equal(cycleAt) {
		t.Fatalf("stopped container restart.count at %v, want the cycle timestamp %v", restartAt[stopped.ID], cycleAt)
	}
}

type backlogRepository struct {
	testMonitoringRepository
	metrics, events int
	before          time.Time
}

func (r *backlogRepository) DeleteMetricsBefore(_ context.Context, before time.Time, limit int) (int, error) {
	r.before = before
	deleted := min(limit, r.metrics)
	r.metrics -= deleted
	return deleted, nil
}
func (r *backlogRepository) DeleteEventsBefore(_ context.Context, _ time.Time, limit int) (int, error) {
	deleted := min(limit, r.events)
	r.events -= deleted
	return deleted, nil
}

// A day of host samples is ~870 000 rows: the purge must drain any backlog,
// not stop after a fixed number of batches.
func TestPurgeDrainsTheWholeBacklog(t *testing.T) {
	repository := &backlogRepository{metrics: 870_500, events: 2_500}
	monitoring := NewMonitoringUsecase(&testSystemCollector{}, unavailableDocker{}, repository,
		MonitoringConfig{Retention: 30 * 24 * time.Hour}, zap.NewNop())
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	monitoring.now = func() time.Time { return now }

	if err := monitoring.purgeMetrics(context.Background()); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if repository.metrics != 0 || repository.events != 0 {
		t.Fatalf("remaining metrics=%d events=%d, want the backlog fully purged", repository.metrics, repository.events)
	}
	if want := now.Add(-30 * 24 * time.Hour); !repository.before.Equal(want) {
		t.Fatalf("purge cutoff = %v, want %v", repository.before, want)
	}
}

func TestPurgeStopsWhenItsDeadlineExpires(t *testing.T) {
	repository := &backlogRepository{metrics: 10_000}
	monitoring := NewMonitoringUsecase(&testSystemCollector{}, unavailableDocker{}, repository,
		MonitoringConfig{Retention: time.Hour}, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := monitoring.purgeMetrics(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("purge error = %v, want context cancellation", err)
	}
}
