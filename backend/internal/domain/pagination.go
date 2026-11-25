package domain

import (
	"time"

	"github.com/google/uuid"
)

// ContainerCursor is the last container of a page of the inventory. The
// next page starts right after it in (name, id) order, so containers that
// share a name keep a stable order.
type ContainerCursor struct {
	Name string
	ID   uuid.UUID
}

// EventCursor is the last event of a page. The next page holds the events
// that come after it in (occurredAt, id) descending order, so events of the
// same instant are neither repeated nor skipped.
type EventCursor struct {
	OccurredAt time.Time
	ID         uuid.UUID
}
