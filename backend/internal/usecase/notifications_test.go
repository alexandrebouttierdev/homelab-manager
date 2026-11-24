package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// memoryNotifications stands for the silence and channel tables.
type memoryNotifications struct {
	mu         sync.Mutex
	silences   []domain.AlertSilence
	channels   []domain.NotificationChannel
	audits     []domain.AuditEntry
	deliveries []delivery
	purged     []time.Time
}

type delivery struct {
	channel  uuid.UUID
	failure  *domain.DeliveryError
	notified []uuid.UUID
}

func (m *memoryNotifications) Silences(_ context.Context, endedAfter time.Time, _ int) ([]domain.AlertSilence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.silences), nil
}

func (m *memoryNotifications) ActiveSilences(_ context.Context, now time.Time) ([]domain.AlertSilence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var active []domain.AlertSilence
	for _, s := range m.silences {
		if s.ActiveAt(now) {
			active = append(active, s)
		}
	}
	return active, nil
}

func (m *memoryNotifications) CreateSilence(_ context.Context, silence domain.AlertSilence, audit domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.silences = append(m.silences, silence)
	m.audits = append(m.audits, audit)
	return nil
}

func (m *memoryNotifications) LiftSilence(_ context.Context, id uuid.UUID, by string, at time.Time, audit domain.AuditEntry) (domain.AlertSilence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.silences {
		if m.silences[i].ID == id {
			m.silences[i].LiftedAt, m.silences[i].LiftedBy = &at, by
			m.audits = append(m.audits, audit)
			return m.silences[i], nil
		}
	}
	return domain.AlertSilence{}, domain.ErrSilenceNotFound
}

func (m *memoryNotifications) DeleteSilencesEndedBefore(_ context.Context, before time.Time, _ int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purged = append(m.purged, before)
	return 0, nil
}

func (m *memoryNotifications) NotificationChannels(context.Context) ([]domain.NotificationChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.channels), nil
}

func (m *memoryNotifications) NotificationChannel(_ context.Context, id uuid.UUID) (domain.NotificationChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, channel := range m.channels {
		if channel.ID == id {
			return channel, nil
		}
	}
	return domain.NotificationChannel{}, domain.ErrChannelNotFound
}

func (m *memoryNotifications) CreateNotificationChannel(_ context.Context, channel domain.NotificationChannel, audit domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = append(m.channels, channel)
	m.audits = append(m.audits, audit)
	return nil
}

func (m *memoryNotifications) UpdateNotificationChannel(_ context.Context, channel domain.NotificationChannel, audit domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.channels {
		if m.channels[i].ID == channel.ID {
			m.channels[i] = channel
			m.audits = append(m.audits, audit)
			return nil
		}
	}
	return domain.ErrChannelNotFound
}

func (m *memoryNotifications) DeleteNotificationChannel(_ context.Context, id uuid.UUID, audit domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.channels {
		if m.channels[i].ID == id {
			m.channels = slices.Delete(m.channels, i, i+1)
			m.audits = append(m.audits, audit)
			return nil
		}
	}
	return domain.ErrChannelNotFound
}

func (m *memoryNotifications) RecordDelivery(_ context.Context, id uuid.UUID, at time.Time, failure *domain.DeliveryError, notified []uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deliveries = append(m.deliveries, delivery{id, failure, notified})
	for i := range m.channels {
		if m.channels[i].ID == id {
			if failure == nil {
				m.channels[i].LastSentAt = &at
			} else {
				m.channels[i].LastErrorAt, m.channels[i].LastError = &at, failure.Code
			}
		}
	}
	return nil
}

// fakeNotifier records the batches it sends.
type fakeNotifier struct {
	mu      sync.Mutex
	sent    map[uuid.UUID][]domain.NotificationBatch
	secrets []domain.ChannelSecrets
	err     error
}

func (f *fakeNotifier) Send(_ context.Context, channel domain.NotificationChannel, secrets domain.ChannelSecrets, host string, batch domain.NotificationBatch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sent == nil {
		f.sent = map[uuid.UUID][]domain.NotificationBatch{}
	}
	f.sent[channel.ID] = append(f.sent[channel.ID], batch)
	f.secrets = append(f.secrets, secrets)
	return f.err
}

func (f *fakeNotifier) batches(id uuid.UUID) []domain.NotificationBatch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent[id])
}

// reversibleBox seals by prefixing the associated data: enough to check
// that the usecase seals, binds and opens.
type reversibleBox struct{}

func (reversibleBox) Seal(plaintext []byte, associated string) (string, error) {
	return "sealed|" + associated + "|" + string(plaintext), nil
}

func (reversibleBox) Open(sealed, associated string) ([]byte, error) {
	rest, ok := strings.CutPrefix(sealed, "sealed|"+associated+"|")
	if !ok {
		return nil, errors.New("cannot open")
	}
	return []byte(rest), nil
}

type recordingAudit struct {
	mu      sync.Mutex
	entries []domain.AuditEntry
}

func (r *recordingAudit) RecordAudit(_ context.Context, entry domain.AuditEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
	return nil
}

type notificationFixture struct {
	usecase  *NotificationUsecase
	repo     *memoryNotifications
	alerts   *memoryAlerts
	notifier *fakeNotifier
	audit    *recordingAudit
	now      *time.Time
}

func newNotificationFixture(t *testing.T, withBox bool) notificationFixture {
	t.Helper()
	f := notificationFixture{repo: &memoryNotifications{}, alerts: &memoryAlerts{}, notifier: &fakeNotifier{}, audit: &recordingAudit{}}
	now := alertBase
	f.now = &now
	var box reversibleBox
	f.usecase = NewNotificationUsecase(f.repo, f.alerts, nil, f.notifier, f.audit, &fakeAlertSources{}, zap.NewNop())
	if withBox {
		f.usecase.box = box
	}
	f.usecase.now = func() time.Time { return *f.now }
	f.usecase.hostname = func() (string, error) { return "nas", nil }
	return f
}

func (f notificationFixture) channel(minSeverity domain.AlertSeverity, renotify int) domain.NotificationChannel {
	channel := domain.NotificationChannel{
		ID: uuid.New(), Name: "ntfy", Type: domain.ChannelNtfy, Enabled: true, MinSeverity: minSeverity, RenotifyMinutes: renotify,
		Settings: domain.ChannelSettings{ServerURL: "https://ntfy.example.org", Topic: "homelab"},
	}
	f.repo.channels = append(f.repo.channels, channel)
	return channel
}

func (f notificationFixture) advance(d time.Duration) { *f.now = f.now.Add(d) }

func openAlert(severity domain.AlertSeverity, firedAt time.Time) domain.Alert {
	return domain.Alert{
		ID: uuid.New(), RuleID: uuid.New(), RuleName: "disk", RuleType: domain.AlertRuleHostMetric, Severity: severity,
		TargetKey: "host:filesystem.percent{mountpoint=/}", State: domain.AlertFiring, FiredAt: firedAt,
	}
}

func events(batch domain.NotificationBatch) []string {
	var out []string
	for _, n := range batch.Notifications {
		out = append(out, string(n.Event)+":"+n.Alert.ID.String()[:4])
	}
	return out
}

func TestNotificationsAreGroupedOnceAMinutePerChannel(t *testing.T) {
	f := newNotificationFixture(t, true)
	channel := f.channel(domain.AlertWarning, 0)
	first, second := openAlert(domain.AlertWarning, alertBase), openAlert(domain.AlertCritical, alertBase)
	f.usecase.AlertTransitions([]domain.Alert{first}, nil)
	f.usecase.dispatch(context.Background())
	if batches := f.notifier.batches(channel.ID); len(batches) != 1 || len(batches[0].Notifications) != 1 {
		t.Fatalf("first dispatch sent %v, want the first alert at once", batches)
	}
	f.advance(5 * time.Second)
	resolved := first
	resolved.State = domain.AlertResolved
	f.usecase.AlertTransitions([]domain.Alert{second}, []domain.Alert{resolved})
	f.usecase.dispatch(context.Background())
	if batches := f.notifier.batches(channel.ID); len(batches) != 1 {
		t.Fatalf("sent %d messages within a minute, want 1", len(batches))
	}
	f.advance(55 * time.Second)
	f.usecase.dispatch(context.Background())
	batches := f.notifier.batches(channel.ID)
	if len(batches) != 2 || len(batches[1].Notifications) != 2 || batches[1].Notifications[0].Event != domain.NotifyFiring ||
		batches[1].Notifications[1].Event != domain.NotifyResolved {
		t.Fatalf("second message = %v, want the firing and the resolution grouped", events(batches[len(batches)-1]))
	}
	if len(f.repo.deliveries) != 2 || f.repo.deliveries[1].failure != nil || len(f.repo.deliveries[1].notified) != 2 {
		t.Fatalf("deliveries = %+v", f.repo.deliveries)
	}
}

func TestNotificationsFollowSeverityAndSilences(t *testing.T) {
	f := newNotificationFixture(t, true)
	critical := f.channel(domain.AlertCritical, 0)
	everything := f.channel(domain.AlertInfo, 0)
	disabled := f.channel(domain.AlertInfo, 0)
	f.repo.channels[2].Enabled = false
	warning, silenced := openAlert(domain.AlertWarning, alertBase), openAlert(domain.AlertCritical, alertBase)
	f.repo.silences = []domain.AlertSilence{{ID: uuid.New(), Scope: domain.SilenceRule, RuleID: &silenced.RuleID, StartsAt: alertBase.Add(-time.Minute), EndsAt: alertBase.Add(time.Hour)}}
	f.usecase.AlertTransitions([]domain.Alert{warning, silenced}, nil)
	f.usecase.dispatch(context.Background())
	if got := f.notifier.batches(critical.ID); len(got) != 0 {
		t.Fatalf("critical channel received %v", got)
	}
	if got := f.notifier.batches(everything.ID); len(got) != 1 || len(got[0].Notifications) != 1 || got[0].Notifications[0].Alert.ID != warning.ID {
		t.Fatalf("info channel received %v, want the warning alert only", got)
	}
	if got := f.notifier.batches(disabled.ID); len(got) != 0 {
		t.Fatalf("disabled channel received %v", got)
	}
}

func TestASilenceSetBeforeTheSendStillApplies(t *testing.T) {
	f := newNotificationFixture(t, true)
	channel := f.channel(domain.AlertInfo, 0)
	first, later := openAlert(domain.AlertWarning, alertBase), openAlert(domain.AlertWarning, alertBase)
	f.usecase.AlertTransitions([]domain.Alert{first}, nil)
	f.usecase.dispatch(context.Background())
	f.advance(10 * time.Second)
	f.usecase.AlertTransitions([]domain.Alert{later}, nil)
	f.usecase.dispatch(context.Background())
	f.repo.silences = []domain.AlertSilence{{ID: uuid.New(), Scope: domain.SilenceAll, StartsAt: f.now.Add(time.Second), EndsAt: f.now.Add(time.Hour)}}
	f.advance(time.Minute)
	f.usecase.dispatch(context.Background())
	if got := f.notifier.batches(channel.ID); len(got) != 1 {
		t.Fatalf("sent %d messages, want the queued alert dropped by the new silence", len(got))
	}
}

func TestRemindersRepeatUntilAcknowledged(t *testing.T) {
	f := newNotificationFixture(t, true)
	channel := f.channel(domain.AlertWarning, 30)
	alert := openAlert(domain.AlertWarning, alertBase)
	f.alerts.alerts = []domain.Alert{alert}
	f.usecase.AlertTransitions([]domain.Alert{alert}, nil)
	f.usecase.dispatch(context.Background())
	f.advance(29 * time.Minute)
	f.usecase.dispatch(context.Background())
	if got := f.notifier.batches(channel.ID); len(got) != 1 {
		t.Fatalf("sent %d messages before 30 min, want 1", len(got))
	}
	f.advance(time.Minute)
	f.usecase.dispatch(context.Background())
	got := f.notifier.batches(channel.ID)
	if len(got) != 2 || got[1].Notifications[0].Event != domain.NotifyReminder {
		t.Fatalf("messages = %d, want a reminder after 30 min", len(got))
	}
	f.alerts.alerts[0].State = domain.AlertAcknowledged
	f.advance(time.Hour)
	f.usecase.dispatch(context.Background())
	if got := f.notifier.batches(channel.ID); len(got) != 2 {
		t.Fatalf("an acknowledged alert was reminded: %d messages", len(got))
	}
}

func TestAFailedSendIsRecordedAndNotRetried(t *testing.T) {
	f := newNotificationFixture(t, true)
	channel := f.channel(domain.AlertInfo, 0)
	f.notifier.err = &domain.DeliveryError{Code: domain.DeliveryHTTPStatus, Status: 500}
	f.usecase.AlertTransitions([]domain.Alert{openAlert(domain.AlertWarning, alertBase)}, nil)
	f.usecase.dispatch(context.Background())
	f.advance(2 * time.Minute)
	f.usecase.dispatch(context.Background())
	if got := f.notifier.batches(channel.ID); len(got) != 1 {
		t.Fatalf("sent %d times, want 1 (no retry)", len(got))
	}
	if len(f.repo.deliveries) != 1 || f.repo.deliveries[0].failure.Status != 500 || len(f.repo.deliveries[0].notified) != 0 {
		t.Fatalf("deliveries = %+v", f.repo.deliveries)
	}
}

func TestWithoutTheSecretsKeyChannelsWithSecretsFail(t *testing.T) {
	f := newNotificationFixture(t, false)
	channel := f.channel(domain.AlertInfo, 0)
	f.repo.channels[0].SealedSecrets, f.repo.channels[0].SecretNames = "sealed", []string{domain.SecretAccessToken}
	f.usecase.AlertTransitions([]domain.Alert{openAlert(domain.AlertWarning, alertBase)}, nil)
	f.usecase.dispatch(context.Background())
	if got := f.notifier.batches(channel.ID); len(got) != 0 {
		t.Fatal("a channel with secrets was used without the secrets key")
	}
	if len(f.repo.deliveries) != 1 || f.repo.deliveries[0].failure.Code != domain.DeliverySecrets {
		t.Fatalf("deliveries = %+v, want secrets_unavailable", f.repo.deliveries)
	}
}

func TestAlertTransitionsNeverBlock(t *testing.T) {
	f := newNotificationFixture(t, true)
	alerts := make([]domain.Alert, maxPendingTransitions+5)
	f.usecase.AlertTransitions(alerts, nil)
	if got := f.usecase.dropped.Load(); got != 5 {
		t.Fatalf("dropped %d transitions, want 5", got)
	}
}

func ntfyInput(token *string) ChannelInput {
	input := ChannelInput{
		Name: "téléphone", Type: domain.ChannelNtfy, Enabled: true, MinSeverity: domain.AlertWarning, RenotifyMinutes: 30,
		Settings: domain.ChannelSettings{ServerURL: "https://ntfy.example.org", Topic: "homelab"},
	}
	if token != nil {
		input.Secrets = map[string]*string{domain.SecretAccessToken: token}
	}
	return input
}

func TestChannelSecretsAreSealedAndNeverServed(t *testing.T) {
	f := newNotificationFixture(t, true)
	admin := domain.User{Username: "admin"}
	token := "tk_first"
	view, err := f.usecase.CreateChannel(context.Background(), admin, ntfyInput(&token), "req")
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	stored := f.repo.channels[0]
	if stored.SealedSecrets != "sealed|notification-channel:"+stored.ID.String()+`|{"accessToken":"tk_first"}` || !slices.Equal(stored.SecretNames, []string{"accessToken"}) {
		t.Fatalf("stored = %+v", stored)
	}
	if secret := view.Secrets[domain.SecretAccessToken]; !secret.Defined || secret.Fingerprint != domain.SecretFingerprint("tk_first") {
		t.Fatalf("view = %+v", view.Secrets)
	}
	// An update without secrets keeps them; with null removes them.
	input := ntfyInput(nil)
	input.Name = "ntfy"
	if _, err := f.usecase.UpdateChannel(context.Background(), admin, stored.ID, input, "req"); err != nil || f.repo.channels[0].SealedSecrets != stored.SealedSecrets {
		t.Fatalf("UpdateChannel without secrets = %v, sealed %q", err, f.repo.channels[0].SealedSecrets)
	}
	input.Secrets = map[string]*string{domain.SecretAccessToken: nil}
	view, err = f.usecase.UpdateChannel(context.Background(), admin, stored.ID, input, "req")
	if err != nil || f.repo.channels[0].SealedSecrets != "" || view.Secrets[domain.SecretAccessToken].Defined {
		t.Fatalf("UpdateChannel removing the token = (%+v, %v)", view, err)
	}
	input.Type = domain.ChannelGotify
	if _, err := f.usecase.UpdateChannel(context.Background(), admin, stored.ID, input, "req"); !errors.Is(err, domain.ErrInvalidChannel) {
		t.Fatalf("changing the type = %v, want ErrInvalidChannel", err)
	}
	if len(f.repo.audits) != 3 || f.repo.audits[0].TargetType != "notification_channel" || f.repo.audits[0].Action != "create" {
		t.Fatalf("audits = %+v", f.repo.audits)
	}
}

func TestChannelsWithoutTheSecretsKey(t *testing.T) {
	f := newNotificationFixture(t, false)
	admin := domain.User{Username: "admin"}
	token := "tk"
	if _, err := f.usecase.CreateChannel(context.Background(), admin, ntfyInput(&token), "req"); !errors.Is(err, domain.ErrSecretsUnavailable) {
		t.Fatalf("CreateChannel with a secret = %v, want ErrSecretsUnavailable", err)
	}
	view, err := f.usecase.CreateChannel(context.Background(), admin, ntfyInput(nil), "req")
	if err != nil {
		t.Fatalf("CreateChannel without secret = %v", err)
	}
	f.repo.channels[0].SealedSecrets, f.repo.channels[0].SecretNames = "sealed", []string{domain.SecretAccessToken}
	input := ntfyInput(nil)
	input.Enabled = false
	if _, err := f.usecase.UpdateChannel(context.Background(), admin, view.Channel.ID, input, "req"); err != nil {
		t.Fatalf("disabling a channel without the key = %v", err)
	}
	listed, err := f.usecase.Channels(context.Background())
	if err != nil || !listed[0].Secrets[domain.SecretAccessToken].Defined || listed[0].Secrets[domain.SecretAccessToken].Fingerprint != "" {
		t.Fatalf("Channels() = (%+v, %v), want the secret defined without fingerprint", listed, err)
	}
	if _, err := f.usecase.TestChannel(context.Background(), admin, view.Channel.ID, "req"); !errors.Is(err, domain.ErrSecretsUnavailable) {
		t.Fatalf("TestChannel without the key = %v, want ErrSecretsUnavailable", err)
	}
}

func TestTestChannelSendsRecordsAndAudits(t *testing.T) {
	f := newNotificationFixture(t, true)
	admin := domain.User{Username: "admin"}
	token := "tk_value"
	view, err := f.usecase.CreateChannel(context.Background(), admin, ntfyInput(&token), "req")
	if err != nil {
		t.Fatal(err)
	}
	tested, err := f.usecase.TestChannel(context.Background(), admin, view.Channel.ID, "req")
	if err != nil || tested.Channel.LastSentAt == nil {
		t.Fatalf("TestChannel = (%+v, %v)", tested, err)
	}
	batches := f.notifier.batches(view.Channel.ID)
	if len(batches) != 1 || !batches[0].Test || f.notifier.secrets[0][domain.SecretAccessToken] != "tk_value" {
		t.Fatalf("sent %+v with %v", batches, f.notifier.secrets)
	}
	f.notifier.err = &domain.DeliveryError{Code: domain.DeliveryTimeout}
	_, err = f.usecase.TestChannel(context.Background(), admin, view.Channel.ID, "req")
	var failure *domain.DeliveryError
	if !errors.As(err, &failure) || failure.Code != domain.DeliveryTimeout {
		t.Fatalf("failed TestChannel = %v, want the DeliveryError", err)
	}
	if len(f.audit.entries) != 2 || f.audit.entries[0].Outcome != domain.AuditSuccess || f.audit.entries[1].Outcome != domain.AuditFailure ||
		f.audit.entries[1].Action != "test" {
		t.Fatalf("audits = %+v", f.audit.entries)
	}
}

func TestSilencesNeedAnExistingRuleAndAreAudited(t *testing.T) {
	f := newNotificationFixture(t, true)
	operator := domain.User{Username: "operator"}
	rule := diskRule(0)
	f.alerts.rules = []domain.AlertRule{rule}
	unknown := uuid.New()
	if _, err := f.usecase.CreateSilence(context.Background(), operator, domain.SilenceRequest{Scope: domain.SilenceRule, RuleID: &unknown, Duration: time.Hour, Reason: "x"}, "req"); !errors.Is(err, domain.ErrAlertRuleNotFound) {
		t.Fatalf("silence of an unknown rule = %v", err)
	}
	if _, err := f.usecase.CreateSilence(context.Background(), operator, domain.SilenceRequest{Scope: domain.SilenceAll, Duration: time.Minute, Reason: "x"}, "req"); !errors.Is(err, domain.ErrInvalidSilence) {
		t.Fatalf("one minute silence = %v, want ErrInvalidSilence", err)
	}
	view, err := f.usecase.CreateSilence(context.Background(), operator, domain.SilenceRequest{Scope: domain.SilenceRule, RuleID: &rule.ID, Duration: time.Hour, Reason: "mise à jour"}, "req")
	if err != nil || !view.Active || view.RuleName != "disk" || view.Silence.CreatedBy != "operator" || !view.Silence.EndsAt.Equal(alertBase.Add(time.Hour)) {
		t.Fatalf("CreateSilence = (%+v, %v)", view, err)
	}
	f.advance(time.Minute)
	lifted, err := f.usecase.LiftSilence(context.Background(), operator, view.Silence.ID, "req")
	if err != nil || lifted.Active || lifted.Silence.LiftedBy != "operator" {
		t.Fatalf("LiftSilence = (%+v, %v)", lifted, err)
	}
	listed, err := f.usecase.Silences(context.Background())
	if err != nil || len(listed) != 1 || listed[0].RuleName != "disk" || listed[0].Active {
		t.Fatalf("Silences = (%+v, %v)", listed, err)
	}
	if len(f.repo.audits) != 2 || f.repo.audits[0].Action != "create" || f.repo.audits[1].Action != "lift" || f.repo.audits[1].TargetType != "alert_silence" {
		t.Fatalf("audits = %+v", f.repo.audits)
	}
}

func TestNotificationRunPurgesAndStops(t *testing.T) {
	f := newNotificationFixture(t, true)
	f.usecase.tick = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.usecase.Run(ctx)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
	f.repo.mu.Lock()
	defer f.repo.mu.Unlock()
	if len(f.repo.purged) != 1 || !f.repo.purged[0].Equal(alertBase.Add(-7*24*time.Hour)) {
		t.Fatalf("purged = %v, want once with the 7 day retention", f.repo.purged)
	}
}

// sinkRecorder records the transitions of the alert engine.
type sinkRecorder struct {
	fired, resolved []domain.Alert
}

func (s *sinkRecorder) AlertTransitions(fired, resolved []domain.Alert) {
	s.fired, s.resolved = append(s.fired, fired...), append(s.resolved, resolved...)
}

func TestAlertEngineSendsItsTransitionsAndMarksSilencedAlerts(t *testing.T) {
	usecase, repo, sources, now := newAlertFixture(t)
	sink := &sinkRecorder{}
	silences := &memoryNotifications{}
	usecase.WithNotifications(sink, silences)
	rule := diskRule(0)
	repo.rules = []domain.AlertRule{rule}
	sources.host = diskSample(92)
	usecase.evaluate(context.Background())
	if len(sink.fired) != 1 || len(sink.resolved) != 0 {
		t.Fatalf("sink = %+v", sink)
	}
	silences.silences = []domain.AlertSilence{{Scope: domain.SilenceRule, RuleID: &rule.ID, StartsAt: alertBase, EndsAt: alertBase.Add(time.Hour)}}
	listed, err := usecase.Alerts(context.Background(), domain.AlertFilter{})
	if err != nil || len(listed) != 1 || !listed[0].Silenced {
		t.Fatalf("Alerts() = (%+v, %v), want the alert silenced", listed, err)
	}
	*now = now.Add(15 * time.Second)
	sources.host = diskSample(50)
	usecase.evaluate(context.Background())
	if len(sink.resolved) != 1 || sink.resolved[0].ID != sink.fired[0].ID || sink.resolved[0].Resolution != domain.ResolutionConditionCleared ||
		sink.resolved[0].ResolvedAt == nil || sink.resolved[0].RuleName != "disk" {
		t.Fatalf("resolved = %+v", sink.resolved)
	}
	alert, err := usecase.Alert(context.Background(), sink.fired[0].ID)
	if err != nil || alert.Silenced {
		t.Fatalf("a resolved alert is marked silenced: (%+v, %v)", alert, err)
	}
}
