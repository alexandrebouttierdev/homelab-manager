package ports

import (
	"context"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

// NotificationRepository stores the alert silences and the notification
// channels (SPEC_ADMINISTRATION.md § 6.2, § 6.3). Every change made by a
// user commits with its audit entry.
type NotificationRepository interface {
	// Silences returns the silences in force or ended after endedAfter,
	// newest first, at most limit.
	Silences(ctx context.Context, endedAfter time.Time, limit int) ([]domain.AlertSilence, error)
	// ActiveSilences returns the silences in force at now.
	ActiveSilences(ctx context.Context, now time.Time) ([]domain.AlertSilence, error)
	// CreateSilence fails with domain.ErrTooManySilences when
	// domain.MaxActiveSilences are already in force.
	CreateSilence(ctx context.Context, silence domain.AlertSilence, audit domain.AuditEntry) error
	// LiftSilence ends a silence in force at at; a silence already ended is
	// returned unchanged, without audit.
	LiftSilence(ctx context.Context, id uuid.UUID, by string, at time.Time, audit domain.AuditEntry) (domain.AlertSilence, error)
	// DeleteSilencesEndedBefore purges ended silences, at most limit.
	DeleteSilencesEndedBefore(ctx context.Context, before time.Time, limit int) (int, error)

	// NotificationChannels returns the channels by name.
	NotificationChannels(ctx context.Context) ([]domain.NotificationChannel, error)
	NotificationChannel(ctx context.Context, id uuid.UUID) (domain.NotificationChannel, error)
	// CreateNotificationChannel fails with domain.ErrTooManyChannels beyond
	// domain.MaxNotificationChannels.
	CreateNotificationChannel(ctx context.Context, channel domain.NotificationChannel, audit domain.AuditEntry) error
	// UpdateNotificationChannel replaces the configuration and the sealed
	// secrets; the delivery state is kept.
	UpdateNotificationChannel(ctx context.Context, channel domain.NotificationChannel, audit domain.AuditEntry) error
	DeleteNotificationChannel(ctx context.Context, id uuid.UUID, audit domain.AuditEntry) error
	// RecordDelivery stores the outcome of a send (failure nil: success) and
	// marks the alerts it carried as notified at at. A channel deleted
	// meanwhile is ignored.
	RecordDelivery(ctx context.Context, id uuid.UUID, at time.Time, failure *domain.DeliveryError, notified []uuid.UUID) error
}

// Notifier sends one message to one channel with its secrets in clear. A
// failure is a *domain.DeliveryError that carries no address nor secret.
type Notifier interface {
	Send(ctx context.Context, channel domain.NotificationChannel, secrets domain.ChannelSecrets, host string, batch domain.NotificationBatch) error
}
