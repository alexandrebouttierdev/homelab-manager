package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// MetricSample is one measure of a series (name, unit, labels) of the host
// (ContainerID nil) or of a container. Stored times have a millisecond
// precision.
type MetricSample struct {
	ContainerID *uuid.UUID
	Name        string
	Value       float64
	Unit        string
	Labels      map[string]string
	RecordedAt  time.Time
	// Aggregate is set on aggregated history samples only: Value is then the
	// mean of Count raw samples and RecordedAt the start of the bucket.
	Aggregate *MetricAggregate
}

type MetricAggregate struct {
	Min   float64
	Max   float64
	Count int
}

// MetricQuery selects a metric history. Step zero means raw samples.
type MetricQuery struct {
	ContainerID *string
	Name        string
	From, To    time.Time
	Limit       int
	Step        time.Duration
	// Labels keeps only series carrying all of these label values.
	Labels map[string]string
}

const (
	// RawMetricsRetention is how long raw points are kept (bounded by the
	// retention); older history is served from the rollups.
	RawMetricsRetention = 24 * time.Hour
	// MetricRollupBucket is the period of the stored rollups, aligned on the
	// Unix epoch: the history older than RawMetricsRetention has this
	// resolution at best.
	MetricRollupBucket = 5 * time.Minute
	// TemperatureMetric is read less often than the other host metrics
	// (TemperatureInterval); the current view adds its latest reading.
	TemperatureMetric = "temperature.celsius"
	// TemperatureInterval is the period of the temperature readings.
	TemperatureInterval = time.Minute
)

// ErrTooManyPoints means an aggregated history would exceed the query limit.
var ErrTooManyPoints = errors.New("too many points for the requested resolution")
