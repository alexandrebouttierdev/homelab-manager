package sqlite

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"monoops/backend/ent"
	entcontainer "monoops/backend/ent/container"
	entevent "monoops/backend/ent/event"
	enthostinfo "monoops/backend/ent/hostinfo"
	"monoops/backend/internal/domain"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

type MonitoringRepository struct {
	client *ent.Client
	// db serves the metric queries (metric_store.go), which Ent does not
	// model.
	db *sql.DB

	// seriesMu guards series, the identifiers of the known metric series.
	// It is only taken inside a transaction: the single connection of the
	// pool (sqlite.go) already serialises the writers, the mutex keeps the
	// map itself safe.
	seriesMu sync.Mutex
	series   map[seriesKey]int64
	// legacyDone is set once the former sample tables are converted.
	legacyDone atomic.Bool
}

func NewMonitoringRepository(database *Database) *MonitoringRepository {
	return &MonitoringRepository{client: database.client, db: database.db, series: map[seriesKey]int64{}}
}

func (r *MonitoringRepository) SaveHost(ctx context.Context, host domain.HostInfo) error {
	record, err := r.client.HostInfo.Query().Where(enthostinfo.SingletonKey("local")).Only(ctx)
	if ent.IsNotFound(err) {
		create := r.client.HostInfo.Create().
			SetID(uuid.New()).SetSingletonKey("local").SetHostname(host.Hostname).
			SetDistribution(host.Distribution).SetOsVersion(host.OSVersion).
			SetKernelVersion(host.KernelVersion).SetUpdatedAt(time.Now().UTC())
		if host.BootTime != nil {
			create.SetBootTime(*host.BootTime)
		}
		_, err = create.Save(ctx)
		return err
	}
	if err != nil {
		return err
	}
	update := r.client.HostInfo.UpdateOneID(record.ID).
		SetHostname(host.Hostname).SetDistribution(host.Distribution).
		SetOsVersion(host.OSVersion).SetKernelVersion(host.KernelVersion).
		SetUpdatedAt(time.Now().UTC())
	if host.BootTime != nil {
		update.SetBootTime(*host.BootTime)
	} else {
		update.ClearBootTime()
	}
	_, err = update.Save(ctx)
	return err
}

func (r *MonitoringRepository) GetHost(ctx context.Context) (domain.HostInfo, error) {
	record, err := r.client.HostInfo.Query().Where(enthostinfo.SingletonKey("local")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.HostInfo{}, nil
		}
		return domain.HostInfo{}, err
	}
	host := domain.HostInfo{
		Hostname: record.Hostname, Distribution: record.Distribution,
		OSVersion: record.OsVersion, KernelVersion: record.KernelVersion,
		BootTime: record.BootTime,
	}
	if record.BootTime != nil {
		uptime := uint64(max(time.Since(*record.BootTime).Seconds(), 0))
		host.UptimeSeconds = &uptime
	}
	return host, nil
}

func (r *MonitoringRepository) ReconcileContainers(ctx context.Context, current []domain.Container, observedAt time.Time) ([]domain.MonitorEvent, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rollback := func(err error) ([]domain.MonitorEvent, error) {
		_ = tx.Rollback()
		return nil, err
	}
	existing, err := tx.Container.Query().Where(entcontainer.RemovedAtIsNil()).All(ctx)
	if err != nil {
		return rollback(err)
	}
	known := make(map[string]*ent.Container, len(existing))
	for _, record := range existing {
		known[record.DockerID] = record
	}
	seen := make(map[string]struct{}, len(current))
	events := make([]domain.MonitorEvent, 0)
	for _, item := range current {
		seen[item.DockerID] = struct{}{}
		record, exists := known[item.DockerID]
		if !exists {
			create := tx.Container.Create().
				SetID(uuid.New()).SetDockerID(item.DockerID).SetName(item.Name).
				SetImage(item.Image).SetState(item.State).SetStatus(item.Status).
				SetFirstSeenAt(observedAt).SetLastSeenAt(observedAt).
				SetRestartCount(item.RestartCount).SetUpdatedAt(observedAt)
			if !item.CreatedAt.IsZero() {
				create.SetCreatedAt(item.CreatedAt)
			}
			if item.StartedAt != nil {
				create.SetStartedAt(*item.StartedAt)
			}
			if item.RestartPolicy != "" {
				create.SetRestartPolicy(item.RestartPolicy)
			}
			if item.Ports != nil {
				create.SetPorts(item.Ports)
			}
			if item.Networks != nil {
				create.SetNetworks(item.Networks)
			}
			if item.Mounts != nil {
				create.SetMounts(item.Mounts)
			}
			record, err = create.Save(ctx)
			if err != nil {
				return rollback(err)
			}
			events = append(events, domain.MonitorEvent{
				ID: uuid.New(), ContainerID: &record.ID, Type: "container.discovered",
				Severity: "info", Message: "Container discovered", OccurredAt: observedAt,
			})
		} else {
			previousState, previousRestart := record.State, record.RestartCount
			update := tx.Container.UpdateOneID(record.ID).
				SetName(item.Name).SetImage(item.Image).SetState(item.State).
				SetStatus(item.Status).SetLastSeenAt(observedAt).ClearRemovedAt().
				SetRestartCount(item.RestartCount).SetUpdatedAt(observedAt)
			if item.StartedAt != nil {
				update.SetStartedAt(*item.StartedAt)
			} else {
				update.ClearStartedAt()
			}
			if item.RestartPolicy != "" {
				update.SetRestartPolicy(item.RestartPolicy)
			}
			if item.Ports != nil {
				update.SetPorts(item.Ports)
			}
			if item.Networks != nil {
				update.SetNetworks(item.Networks)
			}
			if item.Mounts != nil {
				update.SetMounts(item.Mounts)
			}
			record, err = update.Save(ctx)
			if err != nil {
				return rollback(err)
			}
			if previousState != item.State || previousRestart != item.RestartCount {
				eventType := "container.state_changed"
				if item.RestartCount > previousRestart {
					eventType = "container.restarted"
				}
				events = append(events, domain.MonitorEvent{
					ID: uuid.New(), ContainerID: &record.ID, Type: eventType,
					Severity: "info", Message: "Container state or restart count changed",
					Metadata:   map[string]any{"previousState": previousState, "state": item.State},
					OccurredAt: observedAt,
				})
			}
		}
	}
	for _, record := range existing {
		if _, ok := seen[record.DockerID]; ok {
			continue
		}
		removed, err := tx.Container.UpdateOneID(record.ID).
			SetState("removed").SetRemovedAt(observedAt).SetUpdatedAt(observedAt).Save(ctx)
		if err != nil {
			return rollback(err)
		}
		events = append(events, domain.MonitorEvent{
			ID: uuid.New(), ContainerID: &removed.ID, Type: "container.removed",
			Severity: "warning", Message: "Container removed", OccurredAt: observedAt,
		})
	}
	for _, event := range events {
		if err := writeEvent(ctx, tx, event); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *MonitoringRepository) ListContainers(ctx context.Context, includeRemoved bool, limit int) ([]domain.Container, error) {
	return r.ListContainersAfter(ctx, includeRemoved, nil, limit)
}

func (r *MonitoringRepository) ListContainersAfter(ctx context.Context, includeRemoved bool, after *domain.ContainerCursor, limit int) ([]domain.Container, error) {
	query := r.client.Container.Query()
	if !includeRemoved {
		query = query.Where(entcontainer.RemovedAtIsNil())
	}
	if after != nil {
		query = query.Where(entcontainer.Or(
			entcontainer.NameGT(after.Name),
			entcontainer.And(entcontainer.Name(after.Name), entcontainer.IDGT(after.ID)),
		))
	}
	if limit < 1 || limit > 1000 {
		limit = 500
	}
	records, err := query.Order(entcontainer.ByName(), entcontainer.ByID()).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Container, 0, len(records))
	for _, record := range records {
		result = append(result, containerDomain(record))
	}
	return result, nil
}

func (r *MonitoringRepository) GetContainer(ctx context.Context, id string) (domain.Container, error) {
	var record *ent.Container
	var err error
	if parsed, parseErr := uuid.Parse(id); parseErr == nil {
		record, err = r.client.Container.Get(ctx, parsed)
	} else {
		record, err = r.client.Container.Query().Where(entcontainer.DockerID(id)).Only(ctx)
	}
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.Container{}, domain.ErrContainerNotFound
		}
		return domain.Container{}, err
	}
	return containerDomain(record), nil
}

func (r *MonitoringRepository) ListEvents(ctx context.Context, containerID *string, from, to time.Time, before *domain.EventCursor, limit int) ([]domain.MonitorEvent, error) {
	query := r.client.Event.Query().WithContainer().
		Where(entevent.OccurredAtGTE(from), entevent.OccurredAtLT(to)).
		Order(entevent.ByOccurredAt(entsql.OrderDesc()), entevent.ByID(entsql.OrderDesc())).
		Limit(limit)
	if before != nil {
		at := before.OccurredAt.UTC()
		query = query.Where(entevent.Or(
			entevent.OccurredAtLT(at),
			entevent.And(entevent.OccurredAt(at), entevent.IDLT(before.ID)),
		))
	}
	if containerID != nil {
		container, err := r.GetContainer(ctx, *containerID)
		if err != nil {
			return nil, err
		}
		query = query.Where(entevent.HasContainerWith(entcontainer.ID(container.ID)))
	}
	records, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.MonitorEvent, 0, len(records))
	for _, record := range records {
		event := domain.MonitorEvent{
			ID: record.ID, Type: record.Type, Severity: string(record.Severity),
			Message: record.Message, Metadata: record.Metadata, OccurredAt: record.OccurredAt,
		}
		if record.Edges.Container != nil {
			id := record.Edges.Container.ID
			event.ContainerID = &id
		}
		result = append(result, event)
	}
	return result, nil
}

func (r *MonitoringRepository) DeleteEventsBefore(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit < 1 || limit > 10000 {
		limit = 1000
	}
	records, err := r.client.Event.Query().
		Where(entevent.OccurredAtLT(before)).
		Order(entevent.ByOccurredAt()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}
	ids := make([]uuid.UUID, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return r.client.Event.Delete().Where(entevent.IDIn(ids...)).Exec(ctx)
}

func containerDomain(record *ent.Container) domain.Container {
	result := domain.Container{
		ID: record.ID, DockerID: record.DockerID, Name: record.Name, Image: record.Image,
		State: record.State, Status: record.Status,
		StartedAt: record.StartedAt, FirstSeenAt: record.FirstSeenAt, LastSeenAt: record.LastSeenAt,
		RemovedAt: record.RemovedAt, RestartCount: record.RestartCount, RestartPolicy: record.RestartPolicy,
		Ports: record.Ports, Networks: record.Networks, Mounts: record.Mounts,
	}
	if record.CreatedAt != nil {
		result.CreatedAt = *record.CreatedAt
	}
	if record.StartedAt != nil && record.State == "running" {
		if elapsed := time.Since(*record.StartedAt); elapsed >= 0 {
			uptime := uint64(elapsed.Seconds())
			result.UptimeSeconds = &uptime
		}
	}
	return result
}
