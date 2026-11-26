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

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

// Metric storage (migration 021, DATA_MODEL.md). A series (host or
// container, name, unit, labels) is a row of metric_series; a point is
// (series, time in Unix milliseconds, value) in metric_points; the 5-minute
// rollups of metric_rollups keep the history once the raw points are
// purged. metric_rollup_state.rolled_until separates the two: every point
// before it is counted in the rollups, the later ones are not yet.

const (
	// maxHistoryLimit is the absolute cap on one history response.
	maxHistoryLimit = 2000
	// maxSnapshotSamples bounds one cycle; a host produces about a hundred.
	maxSnapshotSamples = 5000
	// maxLatestContainerSamples bounds LatestContainerMetrics: 500
	// containers of a few dozen samples each.
	maxLatestContainerSamples = 50000
	// maxQuerySeries bounds the series of one history query: SQLite binds
	// at most 32 766 parameters and the series are listed twice.
	maxQuerySeries = 10000
	// pointBatchSize rows of 3 parameters per INSERT, rollupBatchSize of 6.
	pointBatchSize  = 500
	rollupBatchSize = 500
	// rollupBucketSeconds is domain.MetricRollupBucket in seconds.
	rollupBucketSeconds = int64(domain.MetricRollupBucket / time.Second)
	// rollupChunkSeconds bounds the points aggregated by one transaction.
	rollupChunkSeconds = int64(time.Hour / time.Second)
	// legacyBatchSize rows of the former tables are converted per
	// transaction, so that the collections are never held long.
	legacyBatchSize = 2000
	// maxCachedSeries bounds the cache of the series identifiers.
	maxCachedSeries = 100000
)

// labelKeyPattern restricts the label keys of a history filter.
var labelKeyPattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// legacyTables are the sample tables of the migrations 001 to 020.
var legacyTables = []struct {
	name      string
	container bool
}{
	{name: "system_metric_samples"},
	{name: "container_metric_samples", container: true},
}

// mergeRollup adds a partial aggregate to an existing rollup: each point is
// counted once, so the sums and counts add up.
const mergeRollup = ` ON CONFLICT (series_id, bucket_start) DO UPDATE SET
	value_sum = value_sum + excluded.value_sum,
	value_min = MIN(value_min, excluded.value_min),
	value_max = MAX(value_max, excluded.value_max),
	sample_count = sample_count + excluded.sample_count`

type seriesKey struct {
	// container is the internal container UUID, "" for the host.
	container string
	name      string
	unit      string
	// labels is the canonical JSON of the labels, "" without labels.
	labels string
}

type seriesInfo struct {
	id         int64
	name       string
	unit       string
	labels     map[string]string
	labelsText string
}

type pointRow struct {
	series int64
	at     int64
	value  float64
}

type rollupKey struct {
	series int64
	bucket int64
}

type rollupRow struct {
	sum, min, max float64
	count         int64
}

func (r *rollupRow) add(value float64) {
	if r.count == 0 || value < r.min {
		r.min = value
	}
	if r.count == 0 || value > r.max {
		r.max = value
	}
	r.sum += value
	r.count++
}

// canonicalLabels encodes labels with sorted keys (encoding/json sorts map
// keys), so that one set of labels always gives the same series.
func canonicalLabels(labels map[string]string) (string, error) {
	if len(labels) == 0 {
		return "", nil
	}
	data, err := json.Marshal(labels)
	if err != nil {
		return "", fmt.Errorf("encode metric labels: %w", err)
	}
	return string(data), nil
}

func decodeLabels(text string) (map[string]string, error) {
	if text == "" || text == "null" {
		return nil, nil
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(text), &labels); err != nil {
		return nil, fmt.Errorf("decode metric labels: %w", err)
	}
	if len(labels) == 0 {
		return nil, nil
	}
	return labels, nil
}

// containerParam binds the container of a series: NULL for the host.
func containerParam(containerID *uuid.UUID) any {
	if containerID == nil {
		return nil
	}
	return containerID.String()
}

func millis(at time.Time) int64 { return at.UnixMilli() }

func fromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// floorBucket returns the start of the rollup bucket holding second.
func floorBucket(second int64) int64 {
	return second - ((second%rollupBucketSeconds)+rollupBucketSeconds)%rollupBucketSeconds
}

// placeholders returns "?, ?, …" for n parameters.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// seriesResolver finds or creates series inside one transaction. The
// identifiers it learns join the cache only once the transaction commits:
// a rolled back insertion frees its identifier for another series.
type seriesResolver struct {
	repo    *MonitoringRepository
	tx      *sql.Tx
	learned map[seriesKey]int64
}

func (r *MonitoringRepository) begin(ctx context.Context) (*seriesResolver, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin metric transaction: %w", err)
	}
	return &seriesResolver{repo: r, tx: tx, learned: map[seriesKey]int64{}}, nil
}

func (s *seriesResolver) id(ctx context.Context, key seriesKey) (int64, error) {
	if id, ok := s.learned[key]; ok {
		return id, nil
	}
	s.repo.seriesMu.Lock()
	id, ok := s.repo.series[key]
	s.repo.seriesMu.Unlock()
	if ok {
		return id, nil
	}
	var container any
	if key.container != "" {
		container = key.container
	}
	if _, err := s.tx.ExecContext(ctx, `INSERT OR IGNORE INTO metric_series (container_id, name, unit, labels)
		VALUES (?, ?, ?, ?)`, container, key.name, key.unit, key.labels); err != nil {
		return 0, fmt.Errorf("create metric series: %w", err)
	}
	if err := s.tx.QueryRowContext(ctx, `SELECT id FROM metric_series
		WHERE COALESCE(container_id, '') = ? AND name = ? AND unit = ? AND labels = ?`,
		key.container, key.name, key.unit, key.labels).Scan(&id); err != nil {
		return 0, fmt.Errorf("read metric series: %w", err)
	}
	s.learned[key] = id
	return id, nil
}

func (s *seriesResolver) rollback() { _ = s.tx.Rollback() }

func (s *seriesResolver) commit() error {
	if err := s.tx.Commit(); err != nil {
		return fmt.Errorf("commit metric transaction: %w", err)
	}
	s.repo.seriesMu.Lock()
	defer s.repo.seriesMu.Unlock()
	if len(s.repo.series)+len(s.learned) > maxCachedSeries {
		clear(s.repo.series)
	}
	for key, id := range s.learned {
		s.repo.series[key] = id
	}
	return nil
}

func sampleKey(sample domain.MetricSample, forContainer bool) (seriesKey, error) {
	labels, err := canonicalLabels(sample.Labels)
	if err != nil {
		return seriesKey{}, err
	}
	key := seriesKey{name: sample.Name, unit: sample.Unit, labels: labels}
	if forContainer {
		if sample.ContainerID == nil {
			return seriesKey{}, errors.New("container metric is missing container ID")
		}
		key.container = sample.ContainerID.String()
	}
	return key, nil
}

func (r *MonitoringRepository) SaveSystemMetrics(ctx context.Context, samples []domain.MetricSample) error {
	return r.savePoints(ctx, samples, false)
}

func (r *MonitoringRepository) SaveContainerMetrics(ctx context.Context, samples []domain.MetricSample) error {
	return r.savePoints(ctx, samples, true)
}

// savePoints writes one cycle in one transaction.
func (r *MonitoringRepository) savePoints(ctx context.Context, samples []domain.MetricSample, forContainer bool) error {
	if len(samples) == 0 {
		return nil
	}
	resolver, err := r.begin(ctx)
	if err != nil {
		return err
	}
	rows := make([]pointRow, 0, len(samples))
	for _, sample := range samples {
		key, err := sampleKey(sample, forContainer)
		if err != nil {
			resolver.rollback()
			return err
		}
		id, err := resolver.id(ctx, key)
		if err != nil {
			resolver.rollback()
			return err
		}
		rows = append(rows, pointRow{series: id, at: millis(sample.RecordedAt), value: sample.Value})
	}
	if err := insertPoints(ctx, resolver.tx, rows); err != nil {
		resolver.rollback()
		return err
	}
	return resolver.commit()
}

// insertPoints keeps the first point when a series has two at the same
// millisecond.
func insertPoints(ctx context.Context, tx *sql.Tx, rows []pointRow) error {
	for start := 0; start < len(rows); start += pointBatchSize {
		batch := rows[start:min(start+pointBatchSize, len(rows))]
		values := make([]string, len(batch))
		args := make([]any, 0, len(batch)*3)
		for i, row := range batch {
			values[i] = "(?, ?, ?)"
			args = append(args, row.series, row.at, row.value)
		}
		statement := `INSERT INTO metric_points (series_id, recorded_at, value) VALUES ` +
			strings.Join(values, ", ") + ` ON CONFLICT DO NOTHING`
		if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
			return fmt.Errorf("write metric points: %w", err)
		}
	}
	return nil
}

func upsertRollups(ctx context.Context, tx *sql.Tx, rollups map[rollupKey]*rollupRow) error {
	keys := make([]rollupKey, 0, len(rollups))
	for key := range rollups {
		keys = append(keys, key)
	}
	for start := 0; start < len(keys); start += rollupBatchSize {
		batch := keys[start:min(start+rollupBatchSize, len(keys))]
		values := make([]string, len(batch))
		args := make([]any, 0, len(batch)*6)
		for i, key := range batch {
			row := rollups[key]
			values[i] = "(?, ?, ?, ?, ?, ?)"
			args = append(args, key.series, key.bucket, row.sum, row.min, row.max, row.count)
		}
		statement := `INSERT INTO metric_rollups (series_id, bucket_start, value_sum, value_min, value_max, sample_count)
			VALUES ` + strings.Join(values, ", ") + mergeRollup
		if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
			return fmt.Errorf("write metric rollups: %w", err)
		}
	}
	return nil
}

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
	series, err := r.matchingSeries(ctx, containerID, query.Name, query.Labels)
	if err != nil || len(series) == 0 {
		return []domain.MetricSample{}, err
	}
	if query.Step > 0 {
		return r.aggregateMetrics(ctx, containerID, series, query)
	}
	return r.rawMetrics(ctx, containerID, series, query)
}

// matchingSeries lists the series of the host (containerID nil) or of a
// container with this name (any name when empty) carrying all the labels.
func (r *MonitoringRepository) matchingSeries(ctx context.Context, containerID *uuid.UUID, name string, labels map[string]string) ([]seriesInfo, error) {
	statement := `SELECT id, name, unit, labels FROM metric_series WHERE container_id IS ?`
	args := []any{containerParam(containerID)}
	if name != "" {
		statement += ` AND name = ?`
		args = append(args, name)
	}
	rows, err := r.db.QueryContext(ctx, statement+` ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("read metric series: %w", err)
	}
	defer rows.Close()
	result := make([]seriesInfo, 0)
	for rows.Next() {
		var info seriesInfo
		if err := rows.Scan(&info.id, &info.name, &info.unit, &info.labelsText); err != nil {
			return nil, fmt.Errorf("scan metric series: %w", err)
		}
		if info.labels, err = decodeLabels(info.labelsText); err != nil {
			return nil, err
		}
		if carriesLabels(info.labels, labels) {
			result = append(result, info)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read metric series: %w", err)
	}
	if len(result) > maxQuerySeries {
		return nil, fmt.Errorf("too many series for metric %q: %d", name, len(result))
	}
	return result, nil
}

func carriesLabels(labels, wanted map[string]string) bool {
	for key, value := range wanted {
		if got, ok := labels[key]; !ok || got != value {
			return false
		}
	}
	return true
}

func seriesArgs(series []seriesInfo) ([]any, map[int64]seriesInfo) {
	args := make([]any, len(series))
	byID := make(map[int64]seriesInfo, len(series))
	for i, info := range series {
		args[i] = info.id
		byID[info.id] = info
	}
	return args, byID
}

func newSample(info seriesInfo, containerID *uuid.UUID, value float64, at time.Time) domain.MetricSample {
	sample := domain.MetricSample{Name: info.name, Value: value, Unit: info.unit, Labels: info.labels, RecordedAt: at}
	if containerID != nil {
		id := *containerID
		sample.ContainerID = &id
	}
	return sample
}

// rawMetrics returns the Limit most recent points of [From, To), in
// chronological order.
func (r *MonitoringRepository) rawMetrics(ctx context.Context, containerID *uuid.UUID, series []seriesInfo, query domain.MetricQuery) ([]domain.MetricSample, error) {
	args, byID := seriesArgs(series)
	args = append(args, millis(query.From), millis(query.To), query.Limit)
	rows, err := r.db.QueryContext(ctx, `SELECT series_id, recorded_at, value FROM metric_points
		WHERE series_id IN (`+placeholders(len(series))+`) AND recorded_at >= ? AND recorded_at < ?
		ORDER BY recorded_at DESC, series_id DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("read metric history: %w", err)
	}
	defer rows.Close()
	result := make([]domain.MetricSample, 0)
	for rows.Next() {
		var id, at int64
		var value float64
		if err := rows.Scan(&id, &at, &value); err != nil {
			return nil, fmt.Errorf("scan metric history: %w", err)
		}
		result = append(result, newSample(byID[id], containerID, value, fromMillis(at)))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read metric history: %w", err)
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result, nil
}

// aggregateMetrics groups per series and per UTC bucket of Step aligned on
// the Unix epoch. Before rolled_until it reads the 5-minute rollups (a
// rollup counts whole when it starts in [From, To)), after it the raw
// points; a bucket that straddles both adds them up.
func (r *MonitoringRepository) aggregateMetrics(ctx context.Context, containerID *uuid.UUID, series []seriesInfo, query domain.MetricQuery) ([]domain.MetricSample, error) {
	step := int64(query.Step / time.Second)
	if step < 1 {
		return nil, errors.New("aggregation step must be at least one second")
	}
	ids, byID := seriesArgs(series)
	fromMs, toMs := millis(query.From), millis(query.To)
	// Rollup starts, in seconds, of [From, To).
	fromSecond := fromMs / 1000
	if fromMs%1000 > 0 {
		fromSecond++
	}
	toSecond := toMs / 1000
	if toMs%1000 > 0 {
		toSecond++
	}
	args := []any{step, step}
	args = append(args, ids...)
	args = append(args, fromSecond, toSecond)
	args = append(args, ids...)
	args = append(args, fromMs, toMs, query.Limit+1)
	in := placeholders(len(series))
	rows, err := r.db.QueryContext(ctx, `SELECT series_id, (b / ?) * ? AS bucket, SUM(s), MIN(mn), MAX(mx), SUM(c)
		FROM (
			SELECT series_id, bucket_start AS b, value_sum AS s, value_min AS mn, value_max AS mx, sample_count AS c
			FROM metric_rollups
			WHERE series_id IN (`+in+`) AND bucket_start >= ? AND bucket_start < ?
				AND bucket_start < (SELECT rolled_until FROM metric_rollup_state)
			UNION ALL
			SELECT series_id, recorded_at / 1000, value, value, value, 1
			FROM metric_points
			WHERE series_id IN (`+in+`) AND recorded_at >= ? AND recorded_at < ?
				AND recorded_at >= (SELECT rolled_until FROM metric_rollup_state) * 1000
		)
		GROUP BY series_id, bucket
		ORDER BY bucket, series_id
		LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("read aggregated metric history: %w", err)
	}
	defer rows.Close()
	type row struct {
		sample domain.MetricSample
		labels string
	}
	buffered := make([]row, 0)
	for rows.Next() {
		var (
			id, bucket, count int64
			sum               float64
			aggregate         domain.MetricAggregate
		)
		if err := rows.Scan(&id, &bucket, &sum, &aggregate.Min, &aggregate.Max, &count); err != nil {
			return nil, fmt.Errorf("scan aggregated metric history: %w", err)
		}
		info := byID[id]
		sample := newSample(info, containerID, sum/float64(count), time.Unix(bucket, 0).UTC())
		aggregate.Count = int(count)
		sample.Aggregate = &aggregate
		buffered = append(buffered, row{sample: sample, labels: info.labelsText})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read aggregated metric history: %w", err)
	}
	if len(buffered) > query.Limit {
		return nil, domain.ErrTooManyPoints
	}
	sort.SliceStable(buffered, func(i, j int) bool {
		if !buffered[i].sample.RecordedAt.Equal(buffered[j].sample.RecordedAt) {
			return buffered[i].sample.RecordedAt.Before(buffered[j].sample.RecordedAt)
		}
		return buffered[i].labels < buffered[j].labels
	})
	result := make([]domain.MetricSample, len(buffered))
	for i, item := range buffered {
		result[i] = item.sample
	}
	return result, nil
}

// LatestMetrics returns every sample of the most recent cycle recorded at
// or after since. For the host, the temperatures, read less often
// (domain.TemperatureInterval), come from their own latest reading when
// that cycle has none.
func (r *MonitoringRepository) LatestMetrics(ctx context.Context, containerID *string, since time.Time) ([]domain.MetricSample, error) {
	var id *uuid.UUID
	if containerID != nil {
		container, err := r.GetContainer(ctx, *containerID)
		if err != nil {
			return nil, err
		}
		id = &container.ID
	}
	samples, err := r.latestCycle(ctx, id, "", since, maxSnapshotSamples)
	if err != nil || id != nil || len(samples) == 0 || len(samples) >= maxSnapshotSamples {
		return samples, err
	}
	for _, sample := range samples {
		if sample.Name == domain.TemperatureMetric {
			return samples, nil
		}
	}
	temperatures, err := r.latestCycle(ctx, nil, domain.TemperatureMetric, since, maxSnapshotSamples-len(samples))
	if err != nil || len(temperatures) == 0 {
		return samples, err
	}
	samples = append(samples, temperatures...)
	sort.SliceStable(samples, func(i, j int) bool { return samples[i].Name < samples[j].Name })
	return samples, nil
}

// latestCycle reads the latest timestamp of the series of the host
// (containerID nil) or of a container, restricted to name when not empty,
// and every point at that timestamp.
func (r *MonitoringRepository) latestCycle(ctx context.Context, containerID *uuid.UUID, name string, since time.Time, limit int) ([]domain.MetricSample, error) {
	condition, joined := `container_id IS ?`, `s.container_id IS ?`
	filter := []any{containerParam(containerID)}
	if name != "" {
		condition, joined = condition+` AND name = ?`, joined+` AND s.name = ?`
		filter = append(filter, name)
	}
	var latest sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(recorded_at) FROM metric_points
		WHERE recorded_at >= ? AND series_id IN (SELECT id FROM metric_series WHERE `+condition+`)`,
		append([]any{millis(since)}, filter...)...).Scan(&latest); err != nil {
		return nil, fmt.Errorf("read latest metric cycle: %w", err)
	}
	if !latest.Valid {
		return []domain.MetricSample{}, nil
	}
	args := append([]any{latest.Int64}, filter...)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `SELECT s.name, s.unit, s.labels, p.value
		FROM metric_points p JOIN metric_series s ON s.id = p.series_id
		WHERE p.recorded_at = ? AND `+joined+`
		ORDER BY s.name, s.labels
		LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("read latest metric cycle: %w", err)
	}
	defer rows.Close()
	at := fromMillis(latest.Int64)
	result := make([]domain.MetricSample, 0)
	for rows.Next() {
		var info seriesInfo
		var value float64
		if err := rows.Scan(&info.name, &info.unit, &info.labelsText, &value); err != nil {
			return nil, fmt.Errorf("scan latest metric cycle: %w", err)
		}
		if info.labels, err = decodeLabels(info.labelsText); err != nil {
			return nil, err
		}
		result = append(result, newSample(info, containerID, value, at))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read latest metric cycle: %w", err)
	}
	return result, nil
}

// LatestContainerMetrics returns, in one query, the latest cycle of every
// container recorded at or after since; a container without one is absent.
func (r *MonitoringRepository) LatestContainerMetrics(ctx context.Context, since time.Time) (map[uuid.UUID][]domain.MetricSample, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT s.container_id, s.name, s.unit, s.labels, p.value, p.recorded_at
		FROM metric_points p
		JOIN metric_series s ON s.id = p.series_id
		JOIN (SELECT ls.container_id AS container_id, MAX(lp.recorded_at) AS latest
			FROM metric_points lp JOIN metric_series ls ON ls.id = lp.series_id
			WHERE lp.recorded_at >= ? AND ls.container_id IS NOT NULL
			GROUP BY ls.container_id) l
		ON s.container_id = l.container_id AND p.recorded_at = l.latest
		ORDER BY s.container_id, s.name
		LIMIT ?`, millis(since), maxLatestContainerSamples)
	if err != nil {
		return nil, fmt.Errorf("read latest container metrics: %w", err)
	}
	defer rows.Close()
	result := map[uuid.UUID][]domain.MetricSample{}
	for rows.Next() {
		var (
			containerText string
			info          seriesInfo
			value         float64
			at            int64
		)
		if err := rows.Scan(&containerText, &info.name, &info.unit, &info.labelsText, &value, &at); err != nil {
			return nil, fmt.Errorf("scan latest container metric: %w", err)
		}
		container, err := uuid.Parse(containerText)
		if err != nil {
			return nil, fmt.Errorf("decode container id: %w", err)
		}
		if info.labels, err = decodeLabels(info.labelsText); err != nil {
			return nil, err
		}
		result[container] = append(result[container], newSample(info, &container, value, fromMillis(at)))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read latest container metrics: %w", err)
	}
	return result, nil
}

// RollupMetrics aggregates into metric_rollups the points of every
// 5-minute bucket that ends at or before until, one hour per transaction,
// and moves rolled_until past them. A stretch without points is skipped.
func (r *MonitoringRepository) RollupMetrics(ctx context.Context, until time.Time) error {
	end := floorBucket(until.Unix())
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		done, err := r.rollupChunk(ctx, end)
		if err != nil || done {
			return err
		}
	}
}

func (r *MonitoringRepository) rollupChunk(ctx context.Context, end int64) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin metric rollup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var rolled int64
	if err := tx.QueryRowContext(ctx, `SELECT rolled_until FROM metric_rollup_state`).Scan(&rolled); err != nil {
		return false, fmt.Errorf("read metric rollup state: %w", err)
	}
	if rolled >= end {
		return true, nil
	}
	var first sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(recorded_at) FROM metric_points
		WHERE recorded_at >= ? AND recorded_at < ?`, rolled*1000, end*1000).Scan(&first); err != nil {
		return false, fmt.Errorf("read metric points to roll up: %w", err)
	}
	next := end
	if first.Valid {
		start := max(rolled, floorBucket(first.Int64/1000))
		next = min(start+rollupChunkSeconds, end)
		if _, err := tx.ExecContext(ctx, `INSERT INTO metric_rollups
			(series_id, bucket_start, value_sum, value_min, value_max, sample_count)
			SELECT series_id, (recorded_at / ?) * ?, SUM(value), MIN(value), MAX(value), COUNT(*)
			FROM metric_points
			WHERE recorded_at >= ? AND recorded_at < ?
			GROUP BY series_id, recorded_at / ?`+mergeRollup,
			rollupBucketSeconds*1000, rollupBucketSeconds, rolled*1000, next*1000, rollupBucketSeconds*1000); err != nil {
			return false, fmt.Errorf("roll up metric points: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE metric_rollup_state SET rolled_until = ?`, next); err != nil {
		return false, fmt.Errorf("write metric rollup state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit metric rollup: %w", err)
	}
	return next >= end, nil
}

// DeleteMetricsBefore deletes at most limit rows: the raw points older than
// rawBefore that the rollups already count, then the rollups that end at
// or before rollupBefore.
func (r *MonitoringRepository) DeleteMetricsBefore(ctx context.Context, rawBefore, rollupBefore time.Time, limit int) (int, error) {
	if limit < 1 || limit > 10000 {
		limit = 1000
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM metric_points WHERE (series_id, recorded_at) IN (
		SELECT series_id, recorded_at FROM metric_points
		WHERE recorded_at < MIN(?, (SELECT rolled_until FROM metric_rollup_state) * 1000)
		ORDER BY recorded_at
		LIMIT ?)`, millis(rawBefore), limit)
	if err != nil {
		return 0, fmt.Errorf("delete metric points: %w", err)
	}
	points, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	remaining := int64(limit) - points
	if remaining <= 0 {
		return int(points), nil
	}
	result, err = r.db.ExecContext(ctx, `DELETE FROM metric_rollups WHERE (series_id, bucket_start) IN (
		SELECT series_id, bucket_start FROM metric_rollups
		WHERE bucket_start <= ?
		ORDER BY bucket_start
		LIMIT ?)`, rollupBefore.Unix()-rollupBucketSeconds, remaining)
	if err != nil {
		return int(points), fmt.Errorf("delete metric rollups: %w", err)
	}
	rollups, err := result.RowsAffected()
	return int(points + rollups), err
}

// DeleteUnusedSeries deletes the series left without points nor rollups
// (an interface or a container gone for longer than the retention).
func (r *MonitoringRepository) DeleteUnusedSeries(ctx context.Context) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin series purge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM metric_series
		WHERE NOT EXISTS (SELECT 1 FROM metric_points p WHERE p.series_id = metric_series.id)
			AND NOT EXISTS (SELECT 1 FROM metric_rollups u WHERE u.series_id = metric_series.id)`)
	if err != nil {
		return 0, fmt.Errorf("delete unused metric series: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if deleted > 0 {
		// Cleared while the transaction holds the only connection: no
		// writer can read a deleted identifier from the cache meanwhile.
		r.seriesMu.Lock()
		clear(r.series)
		r.seriesMu.Unlock()
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit series purge: %w", err)
	}
	return int(deleted), nil
}

// ConvertLegacyMetrics moves the rows of the former sample tables into the
// new storage, legacyBatchSize rows per transaction, and drops each table
// once empty: a row recorded at or after rawSince becomes a raw point, an
// older one is added to its rollup. It returns the rows converted by this
// call and whether nothing remains; on an expired ctx the next call goes on.
//
// The rollups of the new points must wait for the end of the conversion
// (RollupMetrics), so that no converted point lands before rolled_until.
func (r *MonitoringRepository) ConvertLegacyMetrics(ctx context.Context, rawSince time.Time) (int, bool, error) {
	if r.legacyDone.Load() {
		return 0, true, nil
	}
	converted := 0
	for _, table := range legacyTables {
		for {
			if err := ctx.Err(); err != nil {
				return converted, false, err
			}
			count, err := r.convertLegacyBatch(ctx, table.name, table.container, millis(rawSince))
			if err != nil {
				return converted, false, err
			}
			converted += count
			if count == 0 {
				break
			}
		}
	}
	r.legacyDone.Store(true)
	return converted, true, nil
}

type legacyRow struct {
	rowid      int64
	container  sql.NullString
	name       string
	value      float64
	unit       sql.NullString
	labels     sql.NullString
	recordedAt time.Time
}

// convertLegacyBatch converts the first rows of table and returns their
// number; 0 means the table is gone (or has just been dropped).
func (r *MonitoringRepository) convertLegacyBatch(ctx context.Context, table string, container bool, rawSinceMs int64) (int, error) {
	resolver, err := r.begin(ctx)
	if err != nil {
		return 0, err
	}
	tx := resolver.tx
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&exists); err != nil {
		resolver.rollback()
		return 0, fmt.Errorf("look for %s: %w", table, err)
	}
	if exists == 0 {
		resolver.rollback()
		return 0, nil
	}
	rows, err := readLegacyRows(ctx, tx, table, container)
	if err != nil {
		resolver.rollback()
		return 0, err
	}
	if len(rows) == 0 {
		if _, err := tx.ExecContext(ctx, `DROP TABLE `+table); err != nil {
			resolver.rollback()
			return 0, fmt.Errorf("drop %s: %w", table, err)
		}
		return 0, resolver.commit()
	}
	points := make([]pointRow, 0, len(rows))
	rollups := map[rollupKey]*rollupRow{}
	for _, row := range rows {
		labels, err := decodeLabels(row.labels.String)
		if err != nil {
			resolver.rollback()
			return 0, err
		}
		canonical, err := canonicalLabels(labels)
		if err != nil {
			resolver.rollback()
			return 0, err
		}
		id, err := resolver.id(ctx, seriesKey{container: row.container.String, name: row.name, unit: row.unit.String, labels: canonical})
		if err != nil {
			resolver.rollback()
			return 0, err
		}
		at := millis(row.recordedAt)
		if at >= rawSinceMs {
			points = append(points, pointRow{series: id, at: at, value: row.value})
			continue
		}
		key := rollupKey{series: id, bucket: floorBucket(row.recordedAt.Unix())}
		if rollups[key] == nil {
			rollups[key] = &rollupRow{}
		}
		rollups[key].add(row.value)
	}
	if err := insertPoints(ctx, tx, points); err != nil {
		resolver.rollback()
		return 0, err
	}
	if err := upsertRollups(ctx, tx, rollups); err != nil {
		resolver.rollback()
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE rowid <= ?`, rows[len(rows)-1].rowid); err != nil {
		resolver.rollback()
		return 0, fmt.Errorf("delete converted rows of %s: %w", table, err)
	}
	if err := resolver.commit(); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func readLegacyRows(ctx context.Context, tx *sql.Tx, table string, container bool) ([]legacyRow, error) {
	containerColumn := "NULL"
	if container {
		containerColumn = "container_id"
	}
	rows, err := tx.QueryContext(ctx, `SELECT rowid, `+containerColumn+`, name, value, unit, labels, recorded_at
		FROM `+table+` ORDER BY rowid LIMIT ?`, legacyBatchSize)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", table, err)
	}
	defer rows.Close()
	result := make([]legacyRow, 0, legacyBatchSize)
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.rowid, &row.container, &row.name, &row.value, &row.unit, &row.labels, &row.recordedAt); err != nil {
			return nil, fmt.Errorf("scan %s: %w", table, err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", table, err)
	}
	return result, nil
}
