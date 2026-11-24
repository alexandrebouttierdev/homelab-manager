package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"homelab-manager/backend/ent"
	entalert "homelab-manager/backend/ent/alert"
	entsilence "homelab-manager/backend/ent/alertsilence"
	entchannel "homelab-manager/backend/ent/notificationchannel"
	"homelab-manager/backend/ent/predicate"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

var _ ports.NotificationRepository = (*NotificationRepository)(nil)

// NotificationRepository stores the alert silences and the notification
// channels.
type NotificationRepository struct {
	client *ent.Client
}

func NewNotificationRepository(client *ent.Client) *NotificationRepository {
	return &NotificationRepository{client: client}
}

// activeAt selects the silences in force at now; lifted_at is always the
// moment a user ended the silence, never a future time.
func activeAt(now time.Time) predicate.AlertSilence {
	now = now.UTC()
	return entsilence.And(entsilence.LiftedAtIsNil(), entsilence.StartsAtLTE(now), entsilence.EndsAtGT(now))
}

func (r *NotificationRepository) Silences(ctx context.Context, endedAfter time.Time, limit int) ([]domain.AlertSilence, error) {
	records, err := r.client.AlertSilence.Query().
		Where(entsilence.Or(
			entsilence.And(entsilence.LiftedAtIsNil(), entsilence.EndsAtGT(endedAfter.UTC())),
			entsilence.LiftedAtGT(endedAfter.UTC()),
		)).
		Order(entsilence.ByStartsAt(entsql.OrderDesc()), entsilence.ByID()).Limit(limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read alert silences: %w", err)
	}
	return silencesDomain(records), nil
}

func (r *NotificationRepository) ActiveSilences(ctx context.Context, now time.Time) ([]domain.AlertSilence, error) {
	records, err := r.client.AlertSilence.Query().Where(activeAt(now)).
		Order(entsilence.ByStartsAt(entsql.OrderDesc()), entsilence.ByID()).Limit(domain.MaxActiveSilences).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read active alert silences: %w", err)
	}
	return silencesDomain(records), nil
}

func (r *NotificationRepository) CreateSilence(ctx context.Context, silence domain.AlertSilence, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		count, err := tx.AlertSilence.Query().Where(activeAt(silence.StartsAt)).Count(ctx)
		if err != nil {
			return fmt.Errorf("count active alert silences: %w", err)
		}
		if count >= domain.MaxActiveSilences {
			return domain.ErrTooManySilences
		}
		create := tx.AlertSilence.Create().
			SetID(silence.ID).SetScope(string(silence.Scope)).SetNillableRuleID(silence.RuleID).
			SetReason(silence.Reason).SetCreatedBy(silence.CreatedBy).
			SetStartsAt(silence.StartsAt.UTC()).SetEndsAt(silence.EndsAt.UTC())
		if silence.TargetKey != "" {
			create.SetTargetKey(silence.TargetKey)
		}
		if err := create.Exec(ctx); err != nil {
			return fmt.Errorf("create alert silence: %w", err)
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *NotificationRepository) LiftSilence(ctx context.Context, id uuid.UUID, by string, at time.Time, audit domain.AuditEntry) (domain.AlertSilence, error) {
	var result domain.AlertSilence
	err := withTx(ctx, r.client, func(tx *ent.Tx) error {
		record, err := tx.AlertSilence.Get(ctx, id)
		if ent.IsNotFound(err) {
			return domain.ErrSilenceNotFound
		}
		if err != nil {
			return fmt.Errorf("read alert silence: %w", err)
		}
		if !silenceDomain(record).ActiveAt(at) {
			result = silenceDomain(record)
			return nil
		}
		record, err = tx.AlertSilence.UpdateOne(record).SetLiftedAt(at.UTC()).SetLiftedBy(by).Save(ctx)
		if err != nil {
			return fmt.Errorf("lift alert silence: %w", err)
		}
		result = silenceDomain(record)
		return writeAudit(ctx, tx, audit)
	})
	return result, err
}

func (r *NotificationRepository) DeleteSilencesEndedBefore(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit < 1 || limit > 10000 {
		limit = 1000
	}
	ids, err := r.client.AlertSilence.Query().Where(entsilence.EndsAtLT(before.UTC())).
		Order(entsilence.ByEndsAt()).Limit(limit).IDs(ctx)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return r.client.AlertSilence.Delete().Where(entsilence.IDIn(ids...)).Exec(ctx)
}

func (r *NotificationRepository) NotificationChannels(ctx context.Context) ([]domain.NotificationChannel, error) {
	records, err := r.client.NotificationChannel.Query().Order(entchannel.ByName(), entchannel.ByID()).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read notification channels: %w", err)
	}
	channels := make([]domain.NotificationChannel, 0, len(records))
	for _, record := range records {
		channel, err := channelDomain(record)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func (r *NotificationRepository) NotificationChannel(ctx context.Context, id uuid.UUID) (domain.NotificationChannel, error) {
	record, err := r.client.NotificationChannel.Get(ctx, id)
	if ent.IsNotFound(err) {
		return domain.NotificationChannel{}, domain.ErrChannelNotFound
	}
	if err != nil {
		return domain.NotificationChannel{}, fmt.Errorf("read notification channel: %w", err)
	}
	return channelDomain(record)
}

func encodeChannel(channel domain.NotificationChannel) (settings, names string, err error) {
	encodedSettings, err := json.Marshal(channel.Settings)
	if err != nil {
		return "", "", fmt.Errorf("encode channel settings: %w", err)
	}
	secretNames := channel.SecretNames
	if secretNames == nil {
		secretNames = []string{}
	}
	encodedNames, err := json.Marshal(secretNames)
	if err != nil {
		return "", "", fmt.Errorf("encode channel secret names: %w", err)
	}
	return string(encodedSettings), string(encodedNames), nil
}

func (r *NotificationRepository) CreateNotificationChannel(ctx context.Context, channel domain.NotificationChannel, audit domain.AuditEntry) error {
	settings, names, err := encodeChannel(channel)
	if err != nil {
		return err
	}
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		count, err := tx.NotificationChannel.Query().Count(ctx)
		if err != nil {
			return fmt.Errorf("count notification channels: %w", err)
		}
		if count >= domain.MaxNotificationChannels {
			return domain.ErrTooManyChannels
		}
		if err := tx.NotificationChannel.Create().
			SetID(channel.ID).SetName(channel.Name).SetType(string(channel.Type)).SetEnabled(channel.Enabled).
			SetMinSeverity(string(channel.MinSeverity)).SetRenotifyMinutes(channel.RenotifyMinutes).
			SetSettings(settings).SetSecrets(channel.SealedSecrets).SetSecretNames(names).
			SetCreatedAt(channel.CreatedAt.UTC()).SetUpdatedAt(channel.UpdatedAt.UTC()).
			Exec(ctx); err != nil {
			return fmt.Errorf("create notification channel: %w", err)
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *NotificationRepository) UpdateNotificationChannel(ctx context.Context, channel domain.NotificationChannel, audit domain.AuditEntry) error {
	settings, names, err := encodeChannel(channel)
	if err != nil {
		return err
	}
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		updated, err := tx.NotificationChannel.Update().Where(entchannel.ID(channel.ID)).
			SetName(channel.Name).SetEnabled(channel.Enabled).SetMinSeverity(string(channel.MinSeverity)).
			SetRenotifyMinutes(channel.RenotifyMinutes).SetSettings(settings).
			SetSecrets(channel.SealedSecrets).SetSecretNames(names).SetUpdatedAt(channel.UpdatedAt.UTC()).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("update notification channel: %w", err)
		}
		if updated == 0 {
			return domain.ErrChannelNotFound
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *NotificationRepository) DeleteNotificationChannel(ctx context.Context, id uuid.UUID, audit domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		deleted, err := tx.NotificationChannel.Delete().Where(entchannel.ID(id)).Exec(ctx)
		if err != nil {
			return fmt.Errorf("delete notification channel: %w", err)
		}
		if deleted == 0 {
			return domain.ErrChannelNotFound
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *NotificationRepository) RecordDelivery(ctx context.Context, id uuid.UUID, at time.Time, failure *domain.DeliveryError, notified []uuid.UUID) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		update := tx.NotificationChannel.Update().Where(entchannel.ID(id))
		if failure == nil {
			update.SetLastSentAt(at.UTC())
		} else {
			update.SetLastErrorAt(at.UTC()).SetLastError(string(failure.Code))
			if failure.Status != 0 {
				update.SetLastErrorStatus(failure.Status)
			} else {
				update.ClearLastErrorStatus()
			}
		}
		if _, err := update.Save(ctx); err != nil {
			return fmt.Errorf("record notification delivery: %w", err)
		}
		if len(notified) == 0 {
			return nil
		}
		if _, err := tx.Alert.Update().Where(entalert.IDIn(notified...)).SetLastNotifiedAt(at.UTC()).Save(ctx); err != nil {
			return fmt.Errorf("mark alerts notified: %w", err)
		}
		return nil
	})
}

func silencesDomain(records []*ent.AlertSilence) []domain.AlertSilence {
	silences := make([]domain.AlertSilence, 0, len(records))
	for _, record := range records {
		silences = append(silences, silenceDomain(record))
	}
	return silences
}

func silenceDomain(record *ent.AlertSilence) domain.AlertSilence {
	silence := domain.AlertSilence{
		ID: record.ID, Scope: domain.SilenceScope(record.Scope), RuleID: record.RuleID, Reason: record.Reason,
		CreatedBy: record.CreatedBy, StartsAt: record.StartsAt, EndsAt: record.EndsAt, LiftedAt: record.LiftedAt,
	}
	if record.TargetKey != nil {
		silence.TargetKey = *record.TargetKey
	}
	if record.LiftedBy != nil {
		silence.LiftedBy = *record.LiftedBy
	}
	return silence
}

func channelDomain(record *ent.NotificationChannel) (domain.NotificationChannel, error) {
	channel := domain.NotificationChannel{
		ID: record.ID, Name: record.Name, Type: domain.ChannelType(record.Type), Enabled: record.Enabled,
		MinSeverity: domain.AlertSeverity(record.MinSeverity), RenotifyMinutes: record.RenotifyMinutes,
		SealedSecrets: record.Secrets, LastSentAt: record.LastSentAt, LastErrorAt: record.LastErrorAt,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if record.LastError != nil {
		channel.LastError = domain.DeliveryErrorCode(*record.LastError)
	}
	if record.LastErrorStatus != nil {
		channel.LastErrorStatus = *record.LastErrorStatus
	}
	if err := json.Unmarshal([]byte(record.Settings), &channel.Settings); err != nil {
		return domain.NotificationChannel{}, fmt.Errorf("decode channel settings %s: %w", record.ID, err)
	}
	if err := json.Unmarshal([]byte(record.SecretNames), &channel.SecretNames); err != nil {
		return domain.NotificationChannel{}, fmt.Errorf("decode channel secret names %s: %w", record.ID, err)
	}
	return channel, nil
}
