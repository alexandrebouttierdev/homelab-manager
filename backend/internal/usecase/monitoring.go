package usecase

import (
	"context"
	"errors"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MonitoringConfig struct {
	SystemInterval      time.Duration
	DockerStatsInterval time.Duration
	DockerScanInterval  time.Duration
	Retention           time.Duration
}

type MonitoringUsecase struct {
	system ports.SystemCollector
	docker ports.DockerSource
	repo   ports.MonitoringRepository
	config MonitoringConfig
	logger *zap.Logger

	mu             sync.RWMutex
	systemStatus   domain.CollectorStatus
	dockerStatus   domain.CollectorStatus
	databaseStatus domain.CollectorStatus
}

type ContainerCounts struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Stopped int `json:"stopped"`
	Other   int `json:"other"`
}

type DashboardSnapshot struct {
	Host       domain.HostInfo
	Containers ContainerCounts
	Metrics    []domain.MetricSample
	Collectors map[string]domain.CollectorStatus
}

const dockerStatsConcurrency = 8

func NewMonitoringUsecase(system ports.SystemCollector, docker ports.DockerSource, repository ports.MonitoringRepository, config MonitoringConfig, logger *zap.Logger) *MonitoringUsecase {
	return &MonitoringUsecase{
		system: system, docker: docker, repo: repository, config: config, logger: logger,
		systemStatus:   domain.CollectorStatus{Status: "starting"},
		dockerStatus:   domain.CollectorStatus{Status: "starting"},
		databaseStatus: domain.CollectorStatus{Status: "starting"},
	}
}

func (u *MonitoringUsecase) Run(ctx context.Context) {
	var workers sync.WaitGroup
	workers.Add(4)
	go u.runPeriodic(ctx, &workers, u.config.SystemInterval, "system", u.collectSystem)
	go u.runPeriodic(ctx, &workers, u.config.DockerScanInterval, "docker discovery", u.discoverContainers)
	go u.runPeriodic(ctx, &workers, u.config.DockerStatsInterval, "docker stats", u.collectContainerMetrics)
	go u.runPeriodic(ctx, &workers, 24*time.Hour, "metrics retention", u.purgeMetrics)
	<-ctx.Done()
	workers.Wait()
}

func (u *MonitoringUsecase) runPeriodic(ctx context.Context, workers *sync.WaitGroup, interval time.Duration, name string, work func(context.Context) error) {
	defer workers.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		workCtx, cancel := context.WithTimeout(ctx, min(max(interval, 10*time.Second), 30*time.Second))
		err := work(workCtx)
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			u.logger.Warn("monitoring cycle failed", zap.String("collector", name), zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (u *MonitoringUsecase) collectSystem(ctx context.Context) error {
	host, samples, err := u.system.Collect(ctx)
	if err != nil {
		u.setSystemStatus("unavailable", nil)
		return err
	}
	if err := u.repo.SaveHost(ctx, host); err != nil {
		u.setDatabaseStatus("unavailable", nil)
		u.setSystemStatus("unavailable", nil)
		return err
	}
	if err := u.repo.SaveSystemMetrics(ctx, samples); err != nil {
		u.setDatabaseStatus("unavailable", nil)
		u.setSystemStatus("unavailable", nil)
		return err
	}
	now := time.Now().UTC()
	u.setDatabaseStatus("ok", &now)
	u.setSystemStatus("ok", &now)
	return nil
}

func (u *MonitoringUsecase) discoverContainers(ctx context.Context) error {
	containers, err := u.docker.ListContainers(ctx)
	if err != nil {
		u.setDockerStatus("unavailable", nil)
		return err
	}
	if _, err := u.repo.ReconcileContainers(ctx, containers, time.Now().UTC()); err != nil {
		u.setDatabaseStatus("unavailable", nil)
		u.setDockerStatus("unavailable", nil)
		return err
	}
	now := time.Now().UTC()
	u.setDatabaseStatus("ok", &now)
	u.setDockerStatus("ok", &now)
	return nil
}

func (u *MonitoringUsecase) collectContainerMetrics(ctx context.Context) error {
	containers, err := u.repo.ListContainers(ctx, false, 1000)
	if err != nil {
		u.setDatabaseStatus("unavailable", nil)
		return err
	}
	samples := make([]domain.MetricSample, 0, len(containers)*12)
	running := make([]domain.Container, 0, len(containers))
	for _, container := range containers {
		containerID := container.ID
		samples = append(samples, domain.MetricSample{
			ID: uuid.New(), ContainerID: &containerID, Name: "restart.count",
			Value: float64(container.RestartCount), Unit: "count", RecordedAt: time.Now().UTC(),
		})
		if container.State == "running" {
			running = append(running, container)
		}
	}
	for start := 0; start < len(running); start += dockerStatsConcurrency {
		end := min(start+dockerStatsConcurrency, len(running))
		var workers sync.WaitGroup
		var samplesMu sync.Mutex
		for _, container := range running[start:end] {
			container := container
			workers.Add(1)
			go func() {
				defer workers.Done()
				if err := ctx.Err(); err != nil {
					return
				}
				metrics, err := u.docker.ContainerStats(ctx, container.DockerID)
				if err != nil {
					u.logger.Warn("container statistics unavailable",
						zap.String("container_id", container.DockerID), zap.Error(err))
					return
				}
				containerID := container.ID
				for i := range metrics {
					metrics[i].ContainerID = &containerID
				}
				samplesMu.Lock()
				samples = append(samples, metrics...)
				samplesMu.Unlock()
			}()
		}
		workers.Wait()
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := u.repo.SaveContainerMetrics(ctx, samples); err != nil {
		u.setDatabaseStatus("unavailable", nil)
		return err
	}
	now := time.Now().UTC()
	u.setDatabaseStatus("ok", &now)
	return nil
}

func (u *MonitoringUsecase) purgeMetrics(ctx context.Context) error {
	before := time.Now().UTC().Add(-u.config.Retention)
	for batch := 0; batch < 100; batch++ {
		deletedMetrics, err := u.repo.DeleteMetricsBefore(ctx, before, 1000)
		if err != nil {
			u.setDatabaseStatus("unavailable", nil)
			return err
		}
		deletedEvents, err := u.repo.DeleteEventsBefore(ctx, before, 1000)
		if err != nil {
			u.setDatabaseStatus("unavailable", nil)
			return err
		}
		if deletedMetrics == 0 && deletedEvents == 0 {
			break
		}
	}
	now := time.Now().UTC()
	u.setDatabaseStatus("ok", &now)
	return nil
}

func (u *MonitoringUsecase) Host(ctx context.Context) (domain.HostInfo, error) {
	return u.repo.GetHost(ctx)
}

func (u *MonitoringUsecase) Containers(ctx context.Context, includeRemoved bool, limit int) ([]domain.Container, error) {
	return u.repo.ListContainers(ctx, includeRemoved, limit)
}

func (u *MonitoringUsecase) Container(ctx context.Context, id string) (domain.Container, error) {
	return u.repo.GetContainer(ctx, id)
}

func (u *MonitoringUsecase) Metrics(ctx context.Context, containerID *string, metric string, from, to time.Time, limit int) ([]domain.MetricSample, error) {
	return u.repo.ListMetrics(ctx, containerID, metric, from, to, limit)
}

func (u *MonitoringUsecase) Events(ctx context.Context, containerID *string, from, to time.Time, limit int) ([]domain.MonitorEvent, error) {
	return u.repo.ListEvents(ctx, containerID, from, to, limit)
}

func (u *MonitoringUsecase) Status() (domain.CollectorStatus, domain.CollectorStatus) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.systemStatus, u.dockerStatus
}

func (u *MonitoringUsecase) setSystemStatus(status string, last *time.Time) {
	u.mu.Lock()
	u.systemStatus = updatedStatus(u.systemStatus, status, last)
	u.mu.Unlock()
}

func (u *MonitoringUsecase) setDockerStatus(status string, last *time.Time) {
	u.mu.Lock()
	u.dockerStatus = updatedStatus(u.dockerStatus, status, last)
	u.mu.Unlock()
}

func (u *MonitoringUsecase) setDatabaseStatus(status string, last *time.Time) {
	u.mu.Lock()
	u.databaseStatus = updatedStatus(u.databaseStatus, status, last)
	u.mu.Unlock()
}

func updatedStatus(previous domain.CollectorStatus, status string, last *time.Time) domain.CollectorStatus {
	if last == nil {
		last = previous.LastSuccessAt
	}
	return domain.CollectorStatus{Status: status, LastSuccessAt: last}
}

func (u *MonitoringUsecase) HealthStatus() (domain.CollectorStatus, domain.CollectorStatus, domain.CollectorStatus) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.databaseStatus, u.systemStatus, u.dockerStatus
}

func (u *MonitoringUsecase) Dashboard(ctx context.Context) (DashboardSnapshot, error) {
	host, err := u.repo.GetHost(ctx)
	if err != nil {
		return DashboardSnapshot{}, err
	}
	containers, err := u.repo.ListContainers(ctx, false, 1000)
	if err != nil {
		return DashboardSnapshot{}, err
	}
	status := ContainerCounts{Total: len(containers)}
	for _, container := range containers {
		switch container.State {
		case "running":
			status.Running++
		case "exited", "created":
			status.Stopped++
		default:
			status.Other++
		}
	}
	now := time.Now().UTC()
	samples, err := u.repo.ListMetrics(ctx, nil, "", now.Add(-5*time.Minute), now, 100)
	if err != nil {
		return DashboardSnapshot{}, err
	}
	systemStatus, dockerStatus := u.Status()
	return DashboardSnapshot{
		Host: host, Containers: status, Metrics: samples,
		Collectors: map[string]domain.CollectorStatus{"system": systemStatus, "docker": dockerStatus},
	}, nil
}

func (u *MonitoringUsecase) Settings() map[string]any {
	systemStatus, dockerStatus := u.Status()
	return map[string]any{
		"systemCollectIntervalSeconds":   int(u.config.SystemInterval.Seconds()),
		"dockerCollectIntervalSeconds":   int(u.config.DockerStatsInterval.Seconds()),
		"dockerDiscoveryIntervalSeconds": int(u.config.DockerScanInterval.Seconds()),
		"metricsRetentionDays":           int(u.config.Retention.Hours() / 24),
		"collectors":                     map[string]domain.CollectorStatus{"system": systemStatus, "docker": dockerStatus},
	}
}

func (u *MonitoringUsecase) RecordCollectorEvent(ctx context.Context, containerID *uuid.UUID, eventType, message string) error {
	return u.repo.CreateEvent(ctx, domain.MonitorEvent{
		ID: uuid.New(), ContainerID: containerID, Type: eventType,
		Severity: "warning", Message: message, OccurredAt: time.Now().UTC(),
	})
}
