package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// notificationTick is how often the dispatcher routes the transitions
	// and sends the queues whose channel is free (one message a minute).
	notificationTick = 5 * time.Second
	// reminderInterval is how often the reminders are looked for.
	reminderInterval = time.Minute
	// maxPendingTransitions bounds the transitions waiting for the
	// dispatcher; beyond, they are dropped and counted.
	maxPendingTransitions = 1000
	// notificationReadTimeout bounds the reads of one dispatch.
	notificationReadTimeout = 10 * time.Second
	// deliveryRecordTimeout bounds the write of a delivery outcome.
	deliveryRecordTimeout    = 5 * time.Second
	notificationPurgeTimeout = 30 * time.Second
	auditTargetSilence       = "alert_silence"
	auditTargetChannel       = "notification_channel"
	fallbackHostname         = "homelab"
)

// NotificationAlertReader reads the open alerts and the rules.
type NotificationAlertReader interface {
	OpenAlerts(ctx context.Context) ([]domain.Alert, error)
	AlertRule(ctx context.Context, id uuid.UUID) (domain.AlertRule, error)
	AlertRules(ctx context.Context) ([]domain.AlertRule, error)
}

// ChannelInput is a channel received from a client. Secrets lists the
// changes: an absent name keeps its value, nil removes it.
type ChannelInput struct {
	Name            string
	Type            domain.ChannelType
	Enabled         bool
	MinSeverity     domain.AlertSeverity
	RenotifyMinutes int
	Settings        domain.ChannelSettings
	Secrets         map[string]*string
}

// SecretView describes a secret without its value. Fingerprint is empty
// when the secrets key is unavailable.
type SecretView struct {
	Defined     bool
	Fingerprint string
}

// ChannelView is a channel as served: its secrets are never included.
type ChannelView struct {
	Channel domain.NotificationChannel
	Secrets map[string]SecretView
}

// SilenceView is a silence with the name of its rule ("" for a global
// silence or a deleted rule).
type SilenceView struct {
	Silence  domain.AlertSilence
	RuleName string
	Active   bool
}

// NotificationUsecase sends the alert transitions to the notification
// channels (SPEC_ADMINISTRATION.md § 6.3) and manages the channels and
// the silences. Run owns the dispatcher goroutine and its state; the
// sends are the only outbound traffic of the Manager and never slow down
// the evaluation of the alerts.
type NotificationUsecase struct {
	repo      ports.NotificationRepository
	alerts    NotificationAlertReader
	box       ports.SecretBox
	notifier  ports.Notifier
	audit     ports.AuditRecorder
	retention RetentionReader
	logger    *zap.Logger
	now       func() time.Time
	hostname  func() (string, error)
	tick      time.Duration

	incoming chan domain.Notification
	dropped  atomic.Int64

	// Dispatcher state, used by the Run goroutine only.
	backlog       []domain.Notification
	queues        map[uuid.UUID]*domain.NotificationQueue
	lastSent      map[uuid.UUID]time.Time
	reminded      map[uuid.UUID]map[uuid.UUID]time.Time
	lastReminders time.Time
}

func NewNotificationUsecase(repo ports.NotificationRepository, alerts NotificationAlertReader, box ports.SecretBox, notifier ports.Notifier,
	audit ports.AuditRecorder, retention RetentionReader, logger *zap.Logger) *NotificationUsecase {
	return &NotificationUsecase{
		repo: repo, alerts: alerts, box: box, notifier: notifier, audit: audit, retention: retention, logger: logger,
		now: time.Now, hostname: os.Hostname, tick: notificationTick,
		incoming: make(chan domain.Notification, maxPendingTransitions),
		queues:   map[uuid.UUID]*domain.NotificationQueue{}, lastSent: map[uuid.UUID]time.Time{},
		reminded: map[uuid.UUID]map[uuid.UUID]time.Time{},
	}
}

// SecretsAvailable reports whether the secrets key is loaded.
func (u *NotificationUsecase) SecretsAvailable() bool { return u.box != nil }

// AlertTransitions receives the alerts fired and resolved by an
// evaluation. It never blocks: beyond maxPendingTransitions, transitions
// are dropped and counted.
func (u *NotificationUsecase) AlertTransitions(fired, resolved []domain.Alert) {
	push := func(event domain.NotificationEvent, alerts []domain.Alert) {
		for _, alert := range alerts {
			select {
			case u.incoming <- domain.Notification{Event: event, Alert: alert}:
			default:
				u.dropped.Add(1)
			}
		}
	}
	push(domain.NotifyFiring, fired)
	push(domain.NotifyResolved, resolved)
}

// Run dispatches every 5 s and purges the ended silences at start then
// hourly, until ctx is cancelled. Sends in progress are awaited.
func (u *NotificationUsecase) Run(ctx context.Context) {
	ticker := time.NewTicker(u.tick)
	defer ticker.Stop()
	purge := time.NewTicker(alertPurgeInterval)
	defer purge.Stop()
	u.purge(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-purge.C:
			u.purge(ctx)
		case <-ticker.C:
			u.dispatch(ctx)
		}
	}
}

func (u *NotificationUsecase) purge(ctx context.Context) {
	if u.retention == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, notificationPurgeTimeout)
	defer cancel()
	before := u.now().UTC().Add(-max(u.retention.Retention(), domain.SilenceHistory))
	for {
		deleted, err := u.repo.DeleteSilencesEndedBefore(ctx, before, purgeBatchSize)
		if err != nil {
			u.logger.Warn("could not purge the ended alert silences", zap.Error(err))
			return
		}
		if deleted < purgeBatchSize {
			return
		}
	}
}

// dispatch routes the new transitions to the channels, adds the due
// reminders and sends each queue whose channel sent nothing for a minute.
func (u *NotificationUsecase) dispatch(ctx context.Context) {
	now := u.now().UTC()
	if dropped := u.dropped.Swap(0); dropped > 0 {
		u.logger.Warn("alert notifications dropped: too many transitions waiting", zap.Int64("dropped", dropped))
	}
	for drained := false; !drained; {
		select {
		case n := <-u.incoming:
			if len(u.backlog) < maxPendingTransitions {
				u.backlog = append(u.backlog, n)
			}
		default:
			drained = true
		}
	}
	reminders := now.Sub(u.lastReminders) >= reminderInterval
	if len(u.backlog) == 0 && !reminders && !u.queued() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, notificationReadTimeout)
	defer cancel()
	channels, err := u.repo.NotificationChannels(readCtx)
	if err != nil {
		u.logger.Warn("could not read the notification channels", zap.Error(err))
		return
	}
	silences, err := u.repo.ActiveSilences(readCtx, now)
	if err != nil {
		u.logger.Warn("could not read the alert silences", zap.Error(err))
		return
	}
	enabled := u.forgetMissingChannels(channels)
	for _, n := range u.backlog {
		for _, channel := range enabled {
			if !channel.Routes(n, silences, now) {
				continue
			}
			u.queue(channel.ID).Push(n)
			switch n.Event {
			case domain.NotifyFiring:
				u.remember(channel.ID)[n.Alert.ID] = now
			case domain.NotifyResolved:
				delete(u.remember(channel.ID), n.Alert.ID)
			}
		}
	}
	u.backlog = nil
	if reminders {
		if open, err := u.alerts.OpenAlerts(readCtx); err != nil {
			u.logger.Warn("could not read the open alerts for the reminders", zap.Error(err))
		} else {
			u.lastReminders = now
			u.addReminders(enabled, open, silences, now)
		}
	}
	u.flush(ctx, enabled, silences, now)
}

// forgetMissingChannels drops the state of deleted or disabled channels
// and returns the enabled ones.
func (u *NotificationUsecase) forgetMissingChannels(channels []domain.NotificationChannel) []domain.NotificationChannel {
	var enabled []domain.NotificationChannel
	keep := map[uuid.UUID]bool{}
	for _, channel := range channels {
		if channel.Enabled {
			enabled = append(enabled, channel)
			keep[channel.ID] = true
		}
	}
	maps.DeleteFunc(u.queues, func(id uuid.UUID, _ *domain.NotificationQueue) bool { return !keep[id] })
	maps.DeleteFunc(u.reminded, func(id uuid.UUID, _ map[uuid.UUID]time.Time) bool { return !keep[id] })
	maps.DeleteFunc(u.lastSent, func(id uuid.UUID, _ time.Time) bool { return !keep[id] })
	return enabled
}

// queued reports whether a queue holds transitions not sent yet.
func (u *NotificationUsecase) queued() bool {
	for _, queue := range u.queues {
		if queue.Len() > 0 {
			return true
		}
	}
	return false
}

func (u *NotificationUsecase) queue(id uuid.UUID) *domain.NotificationQueue {
	queue, ok := u.queues[id]
	if !ok {
		queue = &domain.NotificationQueue{}
		u.queues[id] = queue
	}
	return queue
}

func (u *NotificationUsecase) remember(id uuid.UUID) map[uuid.UUID]time.Time {
	reminded, ok := u.reminded[id]
	if !ok {
		reminded = map[uuid.UUID]time.Time{}
		u.reminded[id] = reminded
	}
	return reminded
}

func (u *NotificationUsecase) addReminders(channels []domain.NotificationChannel, open []domain.Alert, silences []domain.AlertSilence, now time.Time) {
	openIDs := map[uuid.UUID]bool{}
	for _, alert := range open {
		openIDs[alert.ID] = true
	}
	for _, channel := range channels {
		reminded := u.remember(channel.ID)
		for id := range reminded {
			if !openIDs[id] {
				delete(reminded, id)
			}
		}
		for _, alert := range domain.DueReminders(channel, open, silences, reminded, now) {
			u.queue(channel.ID).Push(domain.Notification{Event: domain.NotifyReminder, Alert: alert})
			reminded[alert.ID] = now
		}
	}
}

// flush sends, in parallel, the queue of every channel free to send, and
// waits for the sends (10 s each at most).
func (u *NotificationUsecase) flush(ctx context.Context, channels []domain.NotificationChannel, silences []domain.AlertSilence, now time.Time) {
	var wg sync.WaitGroup
	for _, channel := range channels {
		queue, ok := u.queues[channel.ID]
		if !ok || queue.Len() == 0 || now.Sub(u.lastSent[channel.ID]) < domain.NotificationGrouping {
			continue
		}
		batch := queue.Drain()
		// A silence set since the transition was queued still applies.
		batch.Notifications = slices.DeleteFunc(batch.Notifications, func(n domain.Notification) bool {
			return domain.Silenced(n.Alert, silences, now)
		})
		if len(batch.Notifications) == 0 && batch.Omitted == 0 {
			continue
		}
		u.lastSent[channel.ID] = now
		wg.Add(1)
		go func() {
			defer wg.Done()
			u.deliver(ctx, channel, batch, now)
		}()
	}
	wg.Wait()
}

// deliver sends one batch and records its outcome; a failure is not
// retried: the reminders cover the alerts that stay open.
func (u *NotificationUsecase) deliver(ctx context.Context, channel domain.NotificationChannel, batch domain.NotificationBatch, now time.Time) error {
	var failure *domain.DeliveryError
	secrets, err := u.openSecrets(channel)
	if err != nil {
		failure = &domain.DeliveryError{Code: domain.DeliverySecrets}
	} else if err := u.notifier.Send(ctx, channel, secrets, u.host(), batch); err != nil {
		if !errors.As(err, &failure) {
			failure = &domain.DeliveryError{Code: domain.DeliveryFailed}
		}
	}
	var notified []uuid.UUID
	if failure == nil {
		for _, n := range batch.Notifications {
			notified = append(notified, n.Alert.ID)
		}
	} else {
		u.logger.Warn("alert notification failed", zap.String("channel_id", channel.ID.String()),
			zap.String("channel_type", string(channel.Type)), zap.String("code", string(failure.Code)), zap.Int("status", failure.Status))
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deliveryRecordTimeout)
	defer cancel()
	if err := u.repo.RecordDelivery(recordCtx, channel.ID, now, failure, notified); err != nil {
		u.logger.Warn("could not record a notification delivery", zap.String("channel_id", channel.ID.String()), zap.Error(err))
	}
	if failure != nil {
		return failure
	}
	return nil
}

func (u *NotificationUsecase) host() string {
	name, err := u.hostname()
	if err != nil || name == "" {
		return fallbackHostname
	}
	return name
}

func (u *NotificationUsecase) openSecrets(channel domain.NotificationChannel) (domain.ChannelSecrets, error) {
	secrets := domain.ChannelSecrets{}
	if channel.SealedSecrets == "" {
		return secrets, nil
	}
	if u.box == nil {
		return nil, domain.ErrSecretsUnavailable
	}
	plaintext, err := u.box.Open(channel.SealedSecrets, domain.ChannelSecretsAssociatedData(channel.ID))
	if err != nil {
		return nil, fmt.Errorf("open channel secrets: %w", err)
	}
	if err := json.Unmarshal(plaintext, &secrets); err != nil {
		return nil, fmt.Errorf("decode channel secrets: %w", err)
	}
	return secrets, nil
}

func (u *NotificationUsecase) sealSecrets(id uuid.UUID, secrets domain.ChannelSecrets) (string, error) {
	if len(secrets) == 0 {
		return "", nil
	}
	if u.box == nil {
		return "", domain.ErrSecretsUnavailable
	}
	plaintext, err := json.Marshal(secrets)
	if err != nil {
		return "", fmt.Errorf("encode channel secrets: %w", err)
	}
	return u.box.Seal(plaintext, domain.ChannelSecretsAssociatedData(id))
}

func (u *NotificationUsecase) view(channel domain.NotificationChannel) ChannelView {
	view := ChannelView{Channel: channel, Secrets: map[string]SecretView{}}
	values, err := u.openSecrets(channel)
	if err != nil {
		values = nil
	}
	for _, name := range domain.ChannelSecretNames(channel.Type) {
		secret := SecretView{Defined: slices.Contains(channel.SecretNames, name)}
		if value, ok := values[name]; ok && secret.Defined {
			secret.Fingerprint = domain.SecretFingerprint(value)
		}
		view.Secrets[name] = secret
	}
	return view
}

func (u *NotificationUsecase) Channels(ctx context.Context) ([]ChannelView, error) {
	channels, err := u.repo.NotificationChannels(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]ChannelView, 0, len(channels))
	for _, channel := range channels {
		views = append(views, u.view(channel))
	}
	return views, nil
}

func (u *NotificationUsecase) Channel(ctx context.Context, id uuid.UUID) (ChannelView, error) {
	channel, err := u.repo.NotificationChannel(ctx, id)
	if err != nil {
		return ChannelView{}, err
	}
	return u.view(channel), nil
}

func channelFromInput(input ChannelInput) domain.NotificationChannel {
	return domain.NotificationChannel{
		Name: input.Name, Type: input.Type, Enabled: input.Enabled, MinSeverity: input.MinSeverity,
		RenotifyMinutes: input.RenotifyMinutes, Settings: input.Settings,
	}
}

// CreateChannel validates and stores a channel with its secrets sealed.
func (u *NotificationUsecase) CreateChannel(ctx context.Context, actor domain.User, input ChannelInput, requestID string) (ChannelView, error) {
	channel := channelFromInput(input)
	if err := channel.Validate(); err != nil {
		return ChannelView{}, err
	}
	secrets := domain.MergeChannelSecrets(nil, input.Secrets)
	if err := domain.ValidateChannelSecrets(channel, secrets); err != nil {
		return ChannelView{}, err
	}
	now := u.now().UTC()
	channel.ID, channel.CreatedAt, channel.UpdatedAt = uuid.New(), now, now
	sealed, err := u.sealSecrets(channel.ID, secrets)
	if err != nil {
		return ChannelView{}, err
	}
	channel.SealedSecrets, channel.SecretNames = sealed, secretNames(secrets)
	if err := u.repo.CreateNotificationChannel(ctx, channel, notificationAudit(actor, auditTargetChannel, channel.ID, "create", requestID)); err != nil {
		return ChannelView{}, err
	}
	return u.view(channel), nil
}

// UpdateChannel replaces the configuration of a channel; its type cannot
// change. Without a change of secret, the sealed secrets are kept as they
// are, so that a channel can be disabled without the secrets key.
func (u *NotificationUsecase) UpdateChannel(ctx context.Context, actor domain.User, id uuid.UUID, input ChannelInput, requestID string) (ChannelView, error) {
	current, err := u.repo.NotificationChannel(ctx, id)
	if err != nil {
		return ChannelView{}, err
	}
	channel := channelFromInput(input)
	if channel.Type != current.Type {
		return ChannelView{}, fmt.Errorf("%w: the type of a channel cannot change", domain.ErrInvalidChannel)
	}
	if err := channel.Validate(); err != nil {
		return ChannelView{}, err
	}
	channel.ID, channel.CreatedAt, channel.UpdatedAt = id, current.CreatedAt, u.now().UTC()
	if len(input.Secrets) == 0 {
		if err := domain.ValidateChannelSecretNames(channel, current.SecretNames); err != nil {
			return ChannelView{}, err
		}
		channel.SealedSecrets, channel.SecretNames = current.SealedSecrets, current.SecretNames
	} else {
		existing, err := u.openSecrets(current)
		if err != nil {
			return ChannelView{}, err
		}
		secrets := domain.MergeChannelSecrets(existing, input.Secrets)
		if err := domain.ValidateChannelSecrets(channel, secrets); err != nil {
			return ChannelView{}, err
		}
		if channel.SealedSecrets, err = u.sealSecrets(id, secrets); err != nil {
			return ChannelView{}, err
		}
		channel.SecretNames = secretNames(secrets)
	}
	channel.LastSentAt, channel.LastErrorAt, channel.LastError, channel.LastErrorStatus = current.LastSentAt, current.LastErrorAt, current.LastError, current.LastErrorStatus
	if err := u.repo.UpdateNotificationChannel(ctx, channel, notificationAudit(actor, auditTargetChannel, id, "update", requestID)); err != nil {
		return ChannelView{}, err
	}
	return u.view(channel), nil
}

func (u *NotificationUsecase) DeleteChannel(ctx context.Context, actor domain.User, id uuid.UUID, requestID string) error {
	return u.repo.DeleteNotificationChannel(ctx, id, notificationAudit(actor, auditTargetChannel, id, "delete", requestID))
}

// TestChannel sends a test message at once, records its outcome and
// audits it. A failed send returns its *domain.DeliveryError.
func (u *NotificationUsecase) TestChannel(ctx context.Context, actor domain.User, id uuid.UUID, requestID string) (ChannelView, error) {
	channel, err := u.repo.NotificationChannel(ctx, id)
	if err != nil {
		return ChannelView{}, err
	}
	if _, err := u.openSecrets(channel); err != nil {
		return ChannelView{}, err
	}
	now := u.now().UTC()
	sendErr := u.deliver(ctx, channel, domain.NotificationBatch{Test: true}, now)
	entry := notificationAudit(actor, auditTargetChannel, id, "test", requestID)
	if sendErr != nil {
		entry.Outcome = domain.AuditFailure
	}
	if err := u.audit.RecordAudit(ctx, entry); err != nil {
		return ChannelView{}, err
	}
	if channel, err = u.repo.NotificationChannel(ctx, id); err != nil {
		return ChannelView{}, err
	}
	return u.view(channel), sendErr
}

// Silences lists the silences in force and those ended for less than
// domain.SilenceHistory, in force first.
func (u *NotificationUsecase) Silences(ctx context.Context) ([]SilenceView, error) {
	now := u.now().UTC()
	silences, err := u.repo.Silences(ctx, now.Add(-domain.SilenceHistory), domain.MaxListedSilences)
	if err != nil {
		return nil, err
	}
	rules, err := u.alerts.AlertRules(ctx)
	if err != nil {
		return nil, err
	}
	names := map[uuid.UUID]string{}
	for _, rule := range rules {
		names[rule.ID] = rule.Name
	}
	domain.SortSilences(silences, now)
	views := make([]SilenceView, 0, len(silences))
	for _, silence := range silences {
		view := SilenceView{Silence: silence, Active: silence.ActiveAt(now)}
		if silence.RuleID != nil {
			view.RuleName = names[*silence.RuleID]
		}
		views = append(views, view)
	}
	return views, nil
}

// CreateSilence starts a silence at once; a rule or target silence needs
// an existing rule.
func (u *NotificationUsecase) CreateSilence(ctx context.Context, actor domain.User, request domain.SilenceRequest, requestID string) (SilenceView, error) {
	if err := request.Validate(); err != nil {
		return SilenceView{}, err
	}
	var ruleName string
	if request.RuleID != nil {
		rule, err := u.alerts.AlertRule(ctx, *request.RuleID)
		if err != nil {
			return SilenceView{}, err
		}
		ruleName = rule.Name
	}
	now := u.now().UTC()
	silence := domain.AlertSilence{
		ID: uuid.New(), Scope: request.Scope, RuleID: request.RuleID, TargetKey: request.TargetKey,
		Reason: request.Reason, CreatedBy: actor.Username, StartsAt: now, EndsAt: now.Add(request.Duration),
	}
	if err := u.repo.CreateSilence(ctx, silence, notificationAudit(actor, auditTargetSilence, silence.ID, "create", requestID)); err != nil {
		return SilenceView{}, err
	}
	return SilenceView{Silence: silence, RuleName: ruleName, Active: true}, nil
}

// LiftSilence ends a silence early; an ended silence is returned as is.
func (u *NotificationUsecase) LiftSilence(ctx context.Context, actor domain.User, id uuid.UUID, requestID string) (SilenceView, error) {
	now := u.now().UTC()
	silence, err := u.repo.LiftSilence(ctx, id, actor.Username, now, notificationAudit(actor, auditTargetSilence, id, "lift", requestID))
	if err != nil {
		return SilenceView{}, err
	}
	view := SilenceView{Silence: silence, Active: silence.ActiveAt(now)}
	if silence.RuleID != nil {
		if rule, err := u.alerts.AlertRule(ctx, *silence.RuleID); err == nil {
			view.RuleName = rule.Name
		}
	}
	return view, nil
}

func secretNames(secrets domain.ChannelSecrets) []string {
	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func notificationAudit(actor domain.User, targetType string, id uuid.UUID, action, requestID string) domain.AuditEntry {
	return domain.AuditEntry{
		Actor: actor.Username, TargetType: targetType, TargetID: id.String(), Action: action,
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
}
