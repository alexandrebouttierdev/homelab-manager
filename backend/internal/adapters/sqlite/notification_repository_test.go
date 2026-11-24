package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

func notificationAudit(target, action string) domain.AuditEntry {
	return domain.AuditEntry{Actor: "admin", TargetType: target, TargetID: "x", Action: action, Outcome: domain.AuditSuccess}
}

func silence(scope domain.SilenceScope, ruleID *uuid.UUID, targetKey string, starts time.Time, duration time.Duration) domain.AlertSilence {
	return domain.AlertSilence{
		ID: uuid.New(), Scope: scope, RuleID: ruleID, TargetKey: targetKey, Reason: "maintenance", CreatedBy: "admin",
		StartsAt: starts, EndsAt: starts.Add(duration),
	}
}

func countAudit(t *testing.T, ctx context.Context, audits *AuditRepository, target string) int {
	t.Helper()
	records, err := audits.ListAudit(ctx, domain.AuditFilter{TargetType: target, Limit: 100})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	return len(records)
}

func TestSilencesAreStoredListedLiftedAndPurged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	repository := NewNotificationRepository(database.Client())
	audits := NewAuditRepository(database.Client())
	rule := uuid.New()
	byTarget := silence(domain.SilenceTarget, &rule, "unit:nginx.service", alertAt, time.Hour)
	global := silence(domain.SilenceAll, nil, "", alertAt.Add(time.Minute), 15*time.Minute)
	old := silence(domain.SilenceRule, &rule, "", alertAt.Add(-10*24*time.Hour), time.Hour)
	for _, s := range []domain.AlertSilence{byTarget, global, old} {
		if err := repository.CreateSilence(ctx, s, notificationAudit("alert_silence", "create")); err != nil {
			t.Fatalf("CreateSilence: %v", err)
		}
	}
	if got := countAudit(t, ctx, audits, "alert_silence"); got != 3 {
		t.Fatalf("audited %d silences, want 3", got)
	}
	now := alertAt.Add(2 * time.Minute)
	active, err := repository.ActiveSilences(ctx, now)
	if err != nil || len(active) != 2 || active[0].ID != global.ID || active[1].TargetKey != "unit:nginx.service" || *active[1].RuleID != rule {
		t.Fatalf("ActiveSilences() = (%+v, %v), want the global then the target silence", active, err)
	}
	lifted, err := repository.LiftSilence(ctx, global.ID, "operator", now, notificationAudit("alert_silence", "lift"))
	if err != nil || lifted.LiftedAt == nil || !lifted.LiftedAt.Equal(now) || lifted.LiftedBy != "operator" || lifted.ActiveAt(now) {
		t.Fatalf("LiftSilence() = (%+v, %v)", lifted, err)
	}
	again, err := repository.LiftSilence(ctx, global.ID, "admin", now.Add(time.Minute), notificationAudit("alert_silence", "lift"))
	if err != nil || again.LiftedBy != "operator" {
		t.Fatalf("lifting an ended silence = (%+v, %v), want it unchanged", again, err)
	}
	if got := countAudit(t, ctx, audits, "alert_silence"); got != 4 {
		t.Fatalf("audited %d silence changes, want 4 (an ended silence is not lifted again)", got)
	}
	if _, err := repository.LiftSilence(ctx, uuid.New(), "admin", now, notificationAudit("alert_silence", "lift")); !errors.Is(err, domain.ErrSilenceNotFound) {
		t.Fatalf("lifting an unknown silence = %v, want ErrSilenceNotFound", err)
	}
	listed, err := repository.Silences(ctx, now.Add(-domain.SilenceHistory), domain.MaxListedSilences)
	if err != nil || len(listed) != 2 {
		t.Fatalf("Silences() = (%+v, %v), want the lifted and the active one, not the old one", listed, err)
	}
	deleted, err := repository.DeleteSilencesEndedBefore(ctx, alertAt.Add(-24*time.Hour), 100)
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteSilencesEndedBefore() = (%d, %v), want 1", deleted, err)
	}
}

func TestSilencesAreBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repository := NewNotificationRepository(openTestDatabase(t, ctx).Client())
	for range domain.MaxActiveSilences {
		if err := repository.CreateSilence(ctx, silence(domain.SilenceAll, nil, "", alertAt, time.Hour), notificationAudit("alert_silence", "create")); err != nil {
			t.Fatalf("CreateSilence: %v", err)
		}
	}
	err := repository.CreateSilence(ctx, silence(domain.SilenceAll, nil, "", alertAt, time.Hour), notificationAudit("alert_silence", "create"))
	if !errors.Is(err, domain.ErrTooManySilences) {
		t.Fatalf("CreateSilence beyond the bound = %v, want ErrTooManySilences", err)
	}
	if err := repository.CreateSilence(ctx, silence(domain.SilenceAll, nil, "", alertAt.Add(2*time.Hour), time.Hour), notificationAudit("alert_silence", "create")); err != nil {
		t.Fatalf("a silence starting after the others ended was refused: %v", err)
	}
}

func notificationChannel(name string) domain.NotificationChannel {
	return domain.NotificationChannel{
		ID: uuid.New(), Name: name, Type: domain.ChannelNtfy, Enabled: true, MinSeverity: domain.AlertWarning, RenotifyMinutes: 30,
		Settings:      domain.ChannelSettings{ServerURL: "https://ntfy.example.org", Topic: "homelab"},
		SealedSecrets: "v1.sealed", SecretNames: []string{domain.SecretAccessToken}, CreatedAt: alertAt, UpdatedAt: alertAt,
	}
}

func TestNotificationChannelsAreStoredWithTheirAudit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	repository := NewNotificationRepository(database.Client())
	audits := NewAuditRepository(database.Client())
	channel := notificationChannel("téléphone")
	if err := repository.CreateNotificationChannel(ctx, channel, notificationAudit("notification_channel", "create")); err != nil {
		t.Fatalf("CreateNotificationChannel: %v", err)
	}
	stored, err := repository.NotificationChannel(ctx, channel.ID)
	if err != nil || stored.Settings.Topic != "homelab" || stored.SealedSecrets != "v1.sealed" || len(stored.SecretNames) != 1 ||
		stored.MinSeverity != domain.AlertWarning || stored.RenotifyMinutes != 30 || stored.LastSentAt != nil {
		t.Fatalf("NotificationChannel() = (%+v, %v)", stored, err)
	}
	channel.Name, channel.Enabled, channel.SealedSecrets, channel.SecretNames = "ntfy", false, "", nil
	channel.UpdatedAt = alertAt.Add(time.Minute)
	if err := repository.UpdateNotificationChannel(ctx, channel, notificationAudit("notification_channel", "update")); err != nil {
		t.Fatalf("UpdateNotificationChannel: %v", err)
	}
	listed, err := repository.NotificationChannels(ctx)
	if err != nil || len(listed) != 1 || listed[0].Name != "ntfy" || listed[0].Enabled || listed[0].SealedSecrets != "" || len(listed[0].SecretNames) != 0 {
		t.Fatalf("NotificationChannels() = (%+v, %v)", listed, err)
	}
	missing := notificationChannel("absent")
	if err := repository.UpdateNotificationChannel(ctx, missing, notificationAudit("notification_channel", "update")); !errors.Is(err, domain.ErrChannelNotFound) {
		t.Fatalf("updating an unknown channel = %v, want ErrChannelNotFound", err)
	}
	if err := repository.DeleteNotificationChannel(ctx, channel.ID, notificationAudit("notification_channel", "delete")); err != nil {
		t.Fatalf("DeleteNotificationChannel: %v", err)
	}
	if _, err := repository.NotificationChannel(ctx, channel.ID); !errors.Is(err, domain.ErrChannelNotFound) {
		t.Fatalf("reading a deleted channel = %v, want ErrChannelNotFound", err)
	}
	if err := repository.DeleteNotificationChannel(ctx, channel.ID, notificationAudit("notification_channel", "delete")); !errors.Is(err, domain.ErrChannelNotFound) {
		t.Fatalf("deleting twice = %v, want ErrChannelNotFound", err)
	}
	if got := countAudit(t, ctx, audits, "notification_channel"); got != 3 {
		t.Fatalf("audited %d channel changes, want 3", got)
	}
}

func TestNotificationChannelsAreBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := NewNotificationRepository(openTestDatabase(t, ctx).Client())
	for range domain.MaxNotificationChannels {
		if err := repository.CreateNotificationChannel(ctx, notificationChannel("c"), notificationAudit("notification_channel", "create")); err != nil {
			t.Fatalf("CreateNotificationChannel: %v", err)
		}
	}
	if err := repository.CreateNotificationChannel(ctx, notificationChannel("c"), notificationAudit("notification_channel", "create")); !errors.Is(err, domain.ErrTooManyChannels) {
		t.Fatalf("CreateNotificationChannel beyond the bound = %v, want ErrTooManyChannels", err)
	}
}

func TestRecordDeliveryKeepsTheLastSuccessAndErrorAndMarksAlerts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	repository := NewNotificationRepository(database.Client())
	alerts := NewAlertRepository(database.Client())
	channel := notificationChannel("ntfy")
	if err := repository.CreateNotificationChannel(ctx, channel, notificationAudit("notification_channel", "create")); err != nil {
		t.Fatal(err)
	}
	rule := customRule("disk")
	alert := fired(rule, "host:filesystem.percent{mountpoint=/data}", alertAt)
	if err := alerts.ApplyAlertEvaluation(ctx, []domain.Alert{alert}, nil); err != nil {
		t.Fatal(err)
	}
	sent := alertAt.Add(time.Minute)
	if err := repository.RecordDelivery(ctx, channel.ID, sent, nil, []uuid.UUID{alert.ID}); err != nil {
		t.Fatalf("RecordDelivery(success): %v", err)
	}
	failedAt := alertAt.Add(2 * time.Minute)
	if err := repository.RecordDelivery(ctx, channel.ID, failedAt, &domain.DeliveryError{Code: domain.DeliveryHTTPStatus, Status: 401}, nil); err != nil {
		t.Fatalf("RecordDelivery(failure): %v", err)
	}
	stored, err := repository.NotificationChannel(ctx, channel.ID)
	if err != nil || stored.LastSentAt == nil || !stored.LastSentAt.Equal(sent) || stored.LastErrorAt == nil || !stored.LastErrorAt.Equal(failedAt) ||
		stored.LastError != domain.DeliveryHTTPStatus || stored.LastErrorStatus != 401 {
		t.Fatalf("channel after deliveries = (%+v, %v)", stored, err)
	}
	if err := repository.RecordDelivery(ctx, channel.ID, failedAt, &domain.DeliveryError{Code: domain.DeliveryTimeout}, nil); err != nil {
		t.Fatal(err)
	}
	if stored, _ := repository.NotificationChannel(ctx, channel.ID); stored.LastErrorStatus != 0 || stored.LastError != domain.DeliveryTimeout {
		t.Fatalf("a failure without status kept the previous status: %+v", stored)
	}
	open, err := alerts.OpenAlerts(ctx)
	if err != nil || len(open) != 1 || open[0].LastNotifiedAt == nil || !open[0].LastNotifiedAt.Equal(sent) {
		t.Fatalf("OpenAlerts() = (%+v, %v), want the alert notified at %s", open, err, sent)
	}
	if err := repository.RecordDelivery(ctx, uuid.New(), sent, nil, nil); err != nil {
		t.Fatalf("RecordDelivery for a deleted channel = %v, want nil", err)
	}
}
