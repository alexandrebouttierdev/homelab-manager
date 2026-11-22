package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"homelab-manager/backend/ent"
	entcontainer "homelab-manager/backend/ent/container"
	entcontainermetricsample "homelab-manager/backend/ent/containermetricsample"
	entevent "homelab-manager/backend/ent/event"
	enthostinfo "homelab-manager/backend/ent/hostinfo"
	entsystemmetricsample "homelab-manager/backend/ent/systemmetricsample"
	"homelab-manager/backend/internal/domain"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/google/uuid"
)

type MonitoringRepository struct {
	client *ent.Client
	// db serves the aggregation queries that Ent cannot express.
	db *sql.DB
}

func NewMonitoringRepository(database *Database) *MonitoringRepository {
	return &MonitoringRepository{client: database.client, db: database.db}
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

func (r *MonitoringRepository) SaveSystemMetrics(ctx context.Context, samples []domain.MetricSample) error {
	const batchSize = 500
	for start := 0; start < len(samples); start += batchSize {
		end := min(start+batchSize, len(samples))
		bulk := make([]*ent.SystemMetricSampleCreate, 0, end-start)
		for _, sample := range samples[start:end] {
			create := r.client.SystemMetricSample.Create().
				SetID(sample.ID).SetName(sample.Name).SetValue(sample.Value).
				SetUnit(sample.Unit).SetRecordedAt(sample.RecordedAt)
			if sample.Labels != nil {
				create.SetLabels(sample.Labels)
			}
			bulk = append(bulk, create)
		}
		if _, err := r.client.SystemMetricSample.CreateBulk(bulk...).Save(ctx); err != nil {
			return err
		}
	}
	return nil
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
		create := tx.Event.Create().
			SetID(event.ID).SetType(event.Type).SetSeverity(entevent.Severity(event.Severity)).
			SetMessage(event.Message).SetOccurredAt(event.OccurredAt)
		if event.ContainerID != nil {
			create.SetContainerID(*event.ContainerID)
		}
		if event.Metadata != nil {
			create.SetMetadata(event.Metadata)
		}
		if _, err := create.Save(ctx); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *MonitoringRepository) SaveContainerMetrics(ctx context.Context, samples []domain.MetricSample) error {
	const batchSize = 500
	for start := 0; start < len(samples); start += batchSize {
		end := min(start+batchSize, len(samples))
		bulk := make([]*ent.ContainerMetricSampleCreate, 0, end-start)
		for _, sample := range samples[start:end] {
			if sample.ContainerID == nil {
				return errors.New("container metric is missing container ID")
			}
			create := r.client.ContainerMetricSample.Create().
				SetID(sample.ID).SetContainerID(*sample.ContainerID).
				SetName(sample.Name).SetValue(sample.Value).SetUnit(sample.Unit).
				SetRecordedAt(sample.RecordedAt)
			if sample.Labels != nil {
				create.SetLabels(sample.Labels)
			}
			bulk = append(bulk, create)
		}
		if _, err := r.client.ContainerMetricSample.CreateBulk(bulk...).Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *MonitoringRepository) ListContainers(ctx context.Context, includeRemoved bool, limit int) ([]domain.Container, error) {
	query := r.client.Container.Query()
	if !includeRemoved {
		query = query.Where(entcontainer.RemovedAtIsNil())
	}
	if limit < 1 || limit > 1000 {
		limit = 500
	}
	records, err := query.Order(entcontainer.ByName()).Limit(limit).All(ctx)
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

// maxHistoryLimit is the absolute cap on one history response.
const maxHistoryLimit = 2000

// labelKeyPattern restricts label keys used in JSON paths.
var labelKeyPattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

func (r *MonitoringRepository) ListMetrics(ctx context.Context, query domain.MetricQuery) ([]domain.MetricSample, error) {
	if query.Limit < 1 || query.Limit > maxHistoryLimit {
		query.Limit = 500
	}
	for key := range query.Labels {
		if !labelKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid label key %q", key)
		}
	}
	var containerID *uuid.UUID
	if query.ContainerID != nil {
		container, err := r.GetContainer(ctx, *query.ContainerID)
		if err != nil {
			return nil, err
		}
		containerID = &container.ID
	}
	if query.Step > 0 {
		return r.aggregateMetrics(ctx, containerID, query)
	}
	return r.rawMetrics(ctx, containerID, query)
}

func labelPredicates(labels map[string]string) []*entsql.Predicate {
	predicates := make([]*entsql.Predicate, 0, len(labels))
	for key, value := range labels {
		predicates = append(predicates, sqljson.ValueEQ("labels", value, sqljson.Path(key)))
	}
	return predicates
}

func (r *MonitoringRepository) rawMetrics(ctx context.Context, containerID *uuid.UUID, query domain.MetricQuery) ([]domain.MetricSample, error) {
	result := make([]domain.MetricSample, 0)
	labels := labelPredicates(query.Labels)
	if containerID == nil {
		builder := r.client.SystemMetricSample.Query().Where(
			entsystemmetricsample.RecordedAtGTE(query.From),
			entsystemmetricsample.RecordedAtLT(query.To),
		)
		if query.Name != "" {
			builder = builder.Where(entsystemmetricsample.Name(query.Name))
		}
		for _, predicate := range labels {
			builder = builder.Where(func(s *entsql.Selector) { s.Where(predicate) })
		}
		records, err := builder.Order(entsystemmetricsample.ByRecordedAt(entsql.OrderDesc())).Limit(query.Limit).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			result = append(result, domain.MetricSample{
				ID: record.ID, Name: record.Name, Value: record.Value, Unit: record.Unit,
				Labels: record.Labels, RecordedAt: record.RecordedAt,
			})
		}
		reverseMetrics(result)
		return result, nil
	}
	builder := r.client.ContainerMetricSample.Query().Where(
		entcontainermetricsample.ContainerID(*containerID),
		entcontainermetricsample.RecordedAtGTE(query.From),
		entcontainermetricsample.RecordedAtLT(query.To),
	)
	if query.Name != "" {
		builder = builder.Where(entcontainermetricsample.Name(query.Name))
	}
	for _, predicate := range labels {
		builder = builder.Where(func(s *entsql.Selector) { s.Where(predicate) })
	}
	records, err := builder.Order(entcontainermetricsample.ByRecordedAt(entsql.OrderDesc())).Limit(query.Limit).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		id := *containerID
		result = append(result, domain.MetricSample{
			ID: record.ID, ContainerID: &id, Name: record.Name, Value: record.Value,
			Unit: record.Unit, Labels: record.Labels, RecordedAt: record.RecordedAt,
		})
	}
	reverseMetrics(result)
	return result, nil
}

// aggregateMetrics groups samples per series (name, unit, labels) and per
// UTC bucket aligned on the Unix epoch. Timestamps are stored as
// "YYYY-MM-DD HH:MM:SS…+00:00", so the first 19 characters are a UTC
// SQLite datetime.
func (r *MonitoringRepository) aggregateMetrics(ctx context.Context, containerID *uuid.UUID, query domain.MetricQuery) ([]domain.MetricSample, error) {
	step := int64(query.Step / time.Second)
	if step < 1 {
		return nil, errors.New("aggregation step must be at least one second")
	}
	table := "system_metric_samples"
	where := []string{"name = ?", "recorded_at >= ?", "recorded_at < ?"}
	args := []any{step, step, query.Name, query.From, query.To}
	if containerID != nil {
		table = "container_metric_samples"
		where = append(where, "container_id = ?")
		args = append(args, containerID.String())
	}
	keys := make([]string, 0, len(query.Labels))
	for key := range query.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		where = append(where, "json_extract(labels, ?) = ?")
		args = append(args, `$."`+key+`"`, query.Labels[key])
	}
	args = append(args, query.Limit+1)
	statement := `SELECT name, COALESCE(unit, ''), labels,
			(CAST(strftime('%s', substr(recorded_at, 1, 19)) AS INTEGER) / ?) * ? AS bucket,
			AVG(value), MIN(value), MAX(value), COUNT(*)
		FROM ` + table + `
		WHERE ` + strings.Join(where, " AND ") + `
		GROUP BY name, unit, labels, bucket
		ORDER BY bucket, labels
		LIMIT ?`
	rows, err := r.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.MetricSample, 0)
	for rows.Next() {
		var (
			sample    domain.MetricSample
			rawLabels sql.NullString
			bucket    int64
			aggregate domain.MetricAggregate
		)
		if err := rows.Scan(&sample.Name, &sample.Unit, &rawLabels, &bucket,
			&sample.Value, &aggregate.Min, &aggregate.Max, &aggregate.Count); err != nil {
			return nil, err
		}
		if rawLabels.Valid && rawLabels.String != "" && rawLabels.String != "null" {
			if err := json.Unmarshal([]byte(rawLabels.String), &sample.Labels); err != nil {
				return nil, fmt.Errorf("decode metric labels: %w", err)
			}
		}
		sample.RecordedAt = time.Unix(bucket, 0).UTC()
		sample.Aggregate = &aggregate
		sample.ContainerID = containerID
		result = append(result, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) > query.Limit {
		return nil, domain.ErrTooManyPoints
	}
	return result, nil
}

func (r *MonitoringRepository) ListEvents(ctx context.Context, containerID *string, from, to time.Time, limit int) ([]domain.MonitorEvent, error) {
	query := r.client.Event.Query().WithContainer().
		Where(entevent.OccurredAtGTE(from), entevent.OccurredAtLT(to)).
		Order(entevent.ByOccurredAt(entsql.OrderDesc())).
		Limit(limit)
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

// maxSnapshotSamples bounds one cycle; a host produces about a hundred.
const maxSnapshotSamples = 5000

func (r *MonitoringRepository) LatestMetrics(ctx context.Context, containerID *string, since time.Time) ([]domain.MetricSample, error) {
	if containerID == nil {
		latest, err := r.client.SystemMetricSample.Query().
			Where(entsystemmetricsample.RecordedAtGTE(since)).
			Order(entsystemmetricsample.ByRecordedAt(entsql.OrderDesc())).
			First(ctx)
		if ent.IsNotFound(err) {
			return []domain.MetricSample{}, nil
		}
		if err != nil {
			return nil, err
		}
		records, err := r.client.SystemMetricSample.Query().
			Where(entsystemmetricsample.RecordedAt(latest.RecordedAt)).
			Order(entsystemmetricsample.ByName()).Limit(maxSnapshotSamples).All(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]domain.MetricSample, 0, len(records))
		for _, record := range records {
			result = append(result, domain.MetricSample{
				ID: record.ID, Name: record.Name, Value: record.Value, Unit: record.Unit,
				Labels: record.Labels, RecordedAt: record.RecordedAt,
			})
		}
		return result, nil
	}
	container, err := r.GetContainer(ctx, *containerID)
	if err != nil {
		return nil, err
	}
	latest, err := r.client.ContainerMetricSample.Query().
		Where(entcontainermetricsample.ContainerID(container.ID), entcontainermetricsample.RecordedAtGTE(since)).
		Order(entcontainermetricsample.ByRecordedAt(entsql.OrderDesc())).
		First(ctx)
	if ent.IsNotFound(err) {
		return []domain.MetricSample{}, nil
	}
	if err != nil {
		return nil, err
	}
	records, err := r.client.ContainerMetricSample.Query().
		Where(entcontainermetricsample.ContainerID(container.ID), entcontainermetricsample.RecordedAt(latest.RecordedAt)).
		Order(entcontainermetricsample.ByName()).Limit(maxSnapshotSamples).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.MetricSample, 0, len(records))
	for _, record := range records {
		id := container.ID
		result = append(result, domain.MetricSample{
			ID: record.ID, ContainerID: &id, Name: record.Name, Value: record.Value,
			Unit: record.Unit, Labels: record.Labels, RecordedAt: record.RecordedAt,
		})
	}
	return result, nil
}

func reverseMetrics(samples []domain.MetricSample) {
	for left, right := 0, len(samples)-1; left < right; left, right = left+1, right-1 {
		samples[left], samples[right] = samples[right], samples[left]
	}
}

func (r *MonitoringRepository) DeleteMetricsBefore(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit < 1 || limit > 10000 {
		limit = 1000
	}
	systemRows, err := r.client.SystemMetricSample.Query().
		Where(entsystemmetricsample.RecordedAtLT(before)).
		Order(entsystemmetricsample.ByRecordedAt()).Limit(limit).All(ctx)
	if err != nil {
		return 0, err
	}
	systemIDs := make([]uuid.UUID, 0, len(systemRows))
	for _, row := range systemRows {
		systemIDs = append(systemIDs, row.ID)
	}
	systemDeleted := 0
	if len(systemIDs) != 0 {
		systemDeleted, err = r.client.SystemMetricSample.Delete().
			Where(entsystemmetricsample.IDIn(systemIDs...)).Exec(ctx)
		if err != nil {
			return 0, err
		}
	}
	remaining := limit - systemDeleted
	if remaining <= 0 {
		return systemDeleted, nil
	}
	containerRows, err := r.client.ContainerMetricSample.Query().
		Where(entcontainermetricsample.RecordedAtLT(before)).
		Order(entcontainermetricsample.ByRecordedAt()).Limit(remaining).All(ctx)
	if err != nil {
		return systemDeleted, err
	}
	ids := make([]uuid.UUID, 0, len(containerRows))
	for _, row := range containerRows {
		ids = append(ids, row.ID)
	}
	containerDeleted := 0
	if len(ids) != 0 {
		containerDeleted, err = r.client.ContainerMetricSample.Delete().
			Where(entcontainermetricsample.IDIn(ids...)).Exec(ctx)
	}
	return systemDeleted + containerDeleted, err
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
