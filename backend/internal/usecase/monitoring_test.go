package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

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
func (testMonitoringRepository) CreateEvent(context.Context, domain.MonitorEvent) error { return nil }
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
