package domain

import (
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
}
