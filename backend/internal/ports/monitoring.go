package ports

import (
	"context"
	"time"

	"monoops/backend/internal/domain"
)

type SystemCollector interface {
	Collect(context.Context) (domain.HostInfo, []domain.MetricSample, error)
}

type DockerSource interface {
	ListContainers(context.Context) ([]domain.Container, error)
	ContainerStats(context.Context, string) ([]domain.MetricSample, error)
	// Inventory reads the images, volumes and networks of the Docker host.
	Inventory(context.Context) (domain.DockerInventory, error)
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
	// RollupMetrics aggregates the points of every 5-minute bucket that
	// ends at or before until (domain.MetricRollupBucket).
	RollupMetrics(ctx context.Context, until time.Time) error
	// ConvertLegacyMetrics moves the history of the former sample tables
	// into the series storage, a short batch at a time; it returns the rows
	// converted and whether nothing remains. RollupMetrics must not run
	// before it is done.
	ConvertLegacyMetrics(ctx context.Context, rawSince time.Time) (int, bool, error)
	// DeleteMetricsBefore deletes at most limit rows: raw points older than
	// rawBefore (once rolled up), then rollups ended before rollupBefore.
	DeleteMetricsBefore(ctx context.Context, rawBefore, rollupBefore time.Time, limit int) (int, error)
	// DeleteUnusedSeries deletes the series left without data.
	DeleteUnusedSeries(ctx context.Context) (int, error)
	DeleteEventsBefore(context.Context, time.Time, int) (int, error)
}

// ContainerLookup finds a container of the inventory by its id.
type ContainerLookup interface {
	GetContainer(ctx context.Context, id string) (domain.Container, error)
}

// ContainerLogReader reads the end of the log of a container (D-5).
type ContainerLogReader interface {
	ContainerLogs(ctx context.Context, dockerID string, query domain.ContainerLogQuery) (domain.ContainerLogs, error)
}

// ContainerController starts, stops or restarts a container (D-5). A
// failure is a *domain.ContainerActionError.
type ContainerController interface {
	ActOnContainer(ctx context.Context, dockerID string, verb domain.ContainerVerb) error
}
