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
	now    func() time.Time

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

const (
	dockerStatsConcurrency = 8
	// currentWindow bounds how old the latest cycle may be to be "current".
	currentWindow = 5 * time.Minute
	// staleAfterCycles: a collector whose last success is older than this
	// many intervals is reported "stale" instead of "ok".
	staleAfterCycles = 3
	// retentionInterval: the purge runs at start, then hourly, so each run
	// only removes about an hour of expired samples.
	retentionInterval = time.Hour
	purgeBatchSize    = 1000
)

// Collector status values exposed by the API.
const (
	StatusStarting    = "starting"
	StatusOK          = "ok"
	StatusStale       = "stale"
	StatusUnavailable = "unavailable"
)

func NewMonitoringUsecase(system ports.SystemCollector, docker ports.DockerSource, repository ports.MonitoringRepository, config MonitoringConfig, logger *zap.Logger) *MonitoringUsecase {
	return &MonitoringUsecase{
		system: system, docker: docker, repo: repository, config: config, logger: logger,
		now:            time.Now,
		systemStatus:   domain.CollectorStatus{Status: StatusStarting},
		dockerStatus:   domain.CollectorStatus{Status: StatusStarting},
		databaseStatus: domain.CollectorStatus{Status: StatusStarting},
	}
}

func (u *MonitoringUsecase) Run(ctx context.Context) {
	var workers sync.WaitGroup
	workers.Add(4)
	go u.runPeriodic(ctx, &workers, u.config.SystemInterval, "system", u.collectSystem)
	go u.runPeriodic(ctx, &workers, u.config.DockerScanInterval, "docker discovery", u.discoverContainers)
	go u.runPeriodic(ctx, &workers, u.config.DockerStatsInterval, "docker stats", u.collectContainerMetrics)
	go u.runPeriodic(ctx, &workers, retentionInterval, "metrics retention", u.purgeMetrics)
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
		u.setSystemStatus(StatusUnavailable, nil)
		return err
	}
	if err := u.repo.SaveHost(ctx, host); err != nil {
		u.setDatabaseStatus(StatusUnavailable, nil)
		u.setSystemStatus(StatusUnavailable, nil)
		return err
	}
	if err := u.repo.SaveSystemMetrics(ctx, samples); err != nil {
		u.setDatabaseStatus(StatusUnavailable, nil)
		u.setSystemStatus(StatusUnavailable, nil)
		return err
	}
	now := u.now().UTC()
	u.setDatabaseStatus(StatusOK, &now)
	u.setSystemStatus(StatusOK, &now)
	return nil
}

func (u *MonitoringUsecase) discoverContainers(ctx context.Context) error {
	containers, err := u.docker.ListContainers(ctx)
	if err != nil {
		u.setDockerStatus(StatusUnavailable, nil)
		return err
	}
	if _, err := u.repo.ReconcileContainers(ctx, containers, u.now().UTC()); err != nil {
		u.setDatabaseStatus(StatusUnavailable, nil)
		u.setDockerStatus(StatusUnavailable, nil)
		return err
	}
	now := u.now().UTC()
	u.setDatabaseStatus(StatusOK, &now)
	u.setDockerStatus(StatusOK, &now)
	return nil
}

func (u *MonitoringUsecase) collectContainerMetrics(ctx context.Context) error {
	containers, err := u.repo.ListContainers(ctx, false, 1000)
	if err != nil {
		u.setDatabaseStatus(StatusUnavailable, nil)
		return err
	}
	samples := make([]domain.MetricSample, 0, len(containers)*12)
	running := make([]domain.Container, 0, len(containers))
	cycleAt := u.now().UTC()
	for _, container := range containers {
		if container.State == "running" {
			running = append(running, container)
			continue
		}
		// Stopped containers have no stats; their cycle is the restart count.
		samples = append(samples, restartCountSample(container, cycleAt))
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
				// Same timestamp as the stats, so the restart count belongs
				// to the container's latest cycle.
				recordedAt := cycleAt
				if len(metrics) > 0 {
					recordedAt = metrics[0].RecordedAt
				}
				metrics = append(metrics, restartCountSample(container, recordedAt))
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
		u.setDatabaseStatus(StatusUnavailable, nil)
		return err
	}
	now := u.now().UTC()
	u.setDatabaseStatus(StatusOK, &now)
	return nil
}

func restartCountSample(container domain.Container, recordedAt time.Time) domain.MetricSample {
	containerID := container.ID
	return domain.MetricSample{
		ID: uuid.New(), ContainerID: &containerID, Name: "restart.count",
		Value: float64(container.RestartCount), Unit: "count", RecordedAt: recordedAt,
	}
}

func (u *MonitoringUsecase) purgeMetrics(ctx context.Context) error {
	// Small batches keep each write short (the single SQLite connection is
	// shared); the loop drains the whole backlog unless the cycle deadline
	// expires, in which case the next run continues.
	before := u.now().UTC().Add(-u.config.Retention)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		deletedMetrics, err := u.repo.DeleteMetricsBefore(ctx, before, purgeBatchSize)
		if err != nil {
			u.setDatabaseStatus(StatusUnavailable, nil)
			return err
		}
		deletedEvents, err := u.repo.DeleteEventsBefore(ctx, before, purgeBatchSize)
		if err != nil {
			u.setDatabaseStatus(StatusUnavailable, nil)
			return err
		}
		if deletedMetrics < purgeBatchSize && deletedEvents < purgeBatchSize {
			break
		}
	}
	now := u.now().UTC()
	u.setDatabaseStatus(StatusOK, &now)
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

// CurrentMetrics returns the latest collection cycle of the host (containerID
// nil) or of one container, if it is younger than currentWindow.
func (u *MonitoringUsecase) CurrentMetrics(ctx context.Context, containerID *string) ([]domain.MetricSample, error) {
	return u.repo.LatestMetrics(ctx, containerID, u.now().UTC().Add(-currentWindow))
}

// Status returns the system and Docker collector statuses, downgrading "ok"
// to "stale" when the last success is older than staleAfterCycles intervals.
func (u *MonitoringUsecase) Status() (domain.CollectorStatus, domain.CollectorStatus) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	now := u.now()
	return freshness(u.systemStatus, u.config.SystemInterval, now),
		freshness(u.dockerStatus, u.config.DockerScanInterval, now)
}

func freshness(status domain.CollectorStatus, interval time.Duration, now time.Time) domain.CollectorStatus {
	if status.Status != StatusOK || status.LastSuccessAt == nil || interval <= 0 {
		return status
	}
	if now.Sub(*status.LastSuccessAt) > staleAfterCycles*interval {
		status.Status = StatusStale
	}
	return status
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
	system, docker := u.Status()
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.databaseStatus, system, docker
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
	samples, err := u.CurrentMetrics(ctx, nil)
	if err != nil {
		return DashboardSnapshot{}, err
	}
	systemStatus, dockerStatus := u.Status()
	return DashboardSnapshot{
		Host: host, Containers: status, Metrics: samples,
		Collectors: map[string]domain.CollectorStatus{"system": systemStatus, "docker": dockerStatus},
	}, nil
}

// EffectiveSettings are the active, non-secret collection settings.
type EffectiveSettings struct {
	SystemCollectIntervalSeconds   int
	DockerCollectIntervalSeconds   int
	DockerDiscoveryIntervalSeconds int
	MetricsRetentionDays           int
	System                         domain.CollectorStatus
	Docker                         domain.CollectorStatus
}

func (u *MonitoringUsecase) Settings() EffectiveSettings {
	systemStatus, dockerStatus := u.Status()
	return EffectiveSettings{
		SystemCollectIntervalSeconds:   int(u.config.SystemInterval.Seconds()),
		DockerCollectIntervalSeconds:   int(u.config.DockerStatsInterval.Seconds()),
		DockerDiscoveryIntervalSeconds: int(u.config.DockerScanInterval.Seconds()),
		MetricsRetentionDays:           int(u.config.Retention.Hours() / 24),
		System:                         systemStatus,
		Docker:                         dockerStatus,
	}
}
