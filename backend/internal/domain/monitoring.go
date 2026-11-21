package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrContainerNotFound = errors.New("container not found")

type HostInfo struct {
	Hostname      string
	Distribution  string
	OSVersion     string
	KernelVersion string
	BootTime      *time.Time
	UptimeSeconds *uint64
}

type Container struct {
	ID            uuid.UUID
	DockerID      string
	Name          string
	Image         string
	State         string
	Status        string
	CreatedAt     time.Time
	StartedAt     *time.Time
	UptimeSeconds *uint64
	RestartCount  uint64
	RestartPolicy string
	Ports         []map[string]any
	Networks      []string
	Mounts        []map[string]any
	FirstSeenAt   time.Time
	LastSeenAt    time.Time
	RemovedAt     *time.Time
}

type MonitorEvent struct {
	ID          uuid.UUID
	ContainerID *uuid.UUID
	Type        string
	Severity    string
	Message     string
	Metadata    map[string]any
	OccurredAt  time.Time
}

type CollectorStatus struct {
	Status        string     `json:"status"`
	LastSuccessAt *time.Time `json:"lastSuccessAt"`
}
