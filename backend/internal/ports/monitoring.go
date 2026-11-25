package ports

import (
	"context"
	"time"

	"homelab-manager/backend/internal/domain"
)

type SystemCollector interface {
	Collect(context.Context) (domain.HostInfo, []domain.MetricSample, error)
}

type DockerSource interface {
	ListContainers(context.Context) ([]domain.Container, error)
	ContainerStats(context.Context, string) ([]domain.MetricSample, error)
}

type MonitoringRepository interface {
	SaveHost(context.Context, domain.HostInfo) error
	GetHost(context.Context) (domain.HostInfo, error)
	SaveSystemMetrics(context.Context, []domain.MetricSample) error
	ReconcileContainers(context.Context, []domain.Container, time.Time) ([]domain.MonitorEvent, error)
	SaveContainerMetrics(context.Context, []domain.MetricSample) error
	ListContainers(context.Context, bool, int) ([]domain.Container, error)
	// ListContainersAfter returns at most limit containers in (name, id)
	// order, starting after the cursor when it is not nil.
	ListContainersAfter(ctx context.Context, includeRemoved bool, after *domain.ContainerCursor, limit int) ([]domain.Container, error)
	GetContainer(context.Context, string) (domain.Container, error)
	// ListMetrics returns a raw history (the Limit most recent samples, in
	// chronological order) or, when Step > 0, one aggregated sample per series
	// and bucket, failing with domain.ErrTooManyPoints beyond Limit.
	ListMetrics(context.Context, domain.MetricQuery) ([]domain.MetricSample, error)
	// LatestMetrics returns every sample of the most recent collection cycle
	// recorded at or after since (one timestamp per cycle), for the host when
	// containerID is nil, otherwise for that container.
	LatestMetrics(ctx context.Context, containerID *string, since time.Time) ([]domain.MetricSample, error)
	// ListEvents returns at most limit events of [from, to), most recent
	// first (ties broken by id, descending), only those after the cursor
	// in that order when it is not nil.
	ListEvents(ctx context.Context, containerID *string, from, to time.Time, before *domain.EventCursor, limit int) ([]domain.MonitorEvent, error)
	DeleteMetricsBefore(context.Context, time.Time, int) (int, error)
	DeleteEventsBefore(context.Context, time.Time, int) (int, error)
}
