package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type MetricSample struct {
	ID          uuid.UUID
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

// ErrTooManyPoints means an aggregated history would exceed the query limit.
var ErrTooManyPoints = errors.New("too many points for the requested resolution")
