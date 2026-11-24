package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func firingAlert(severity AlertSeverity) Alert {
	return Alert{
		ID: uuid.New(), RuleID: uuid.New(), RuleName: "Unité systemd en échec", RuleType: AlertRuleUnitFailed,
		Severity: severity, TargetKey: "unit:nginx.service", Target: AlertTarget{Kind: AlertTargetUnit, Unit: "nginx.service"},
		State: AlertFiring, Observed: "failed (exit-code)", FiredAt: alertNow.Add(-10 * time.Minute),
	}
}

func TestSilenceRequestValidation(t *testing.T) {
	rule := uuid.New()
	valid := []SilenceRequest{
		{Scope: SilenceAll, Duration: 15 * time.Minute, Reason: "maintenance"},
		{Scope: SilenceRule, RuleID: &rule, Duration: 7 * 24 * time.Hour, Reason: strings.Repeat("é", 200)},
		{Scope: SilenceTarget, RuleID: &rule, TargetKey: "unit:nginx.service", Duration: time.Hour, Reason: "migration"},
	}
	for _, request := range valid {
		if err := request.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v, want nil", request, err)
		}
	}
	invalid := map[string]SilenceRequest{
		"unknown scope":        {Scope: "host", Duration: time.Hour, Reason: "x"},
		"too short":            {Scope: SilenceAll, Duration: 14 * time.Minute, Reason: "x"},
		"too long":             {Scope: SilenceAll, Duration: 7*24*time.Hour + time.Minute, Reason: "x"},
		"empty reason":         {Scope: SilenceAll, Duration: time.Hour, Reason: "  "},
		"long reason":          {Scope: SilenceAll, Duration: time.Hour, Reason: strings.Repeat("a", 201)},
		"control in reason":    {Scope: SilenceAll, Duration: time.Hour, Reason: "a\nb"},
		"all with rule":        {Scope: SilenceAll, RuleID: &rule, Duration: time.Hour, Reason: "x"},
		"rule without rule":    {Scope: SilenceRule, Duration: time.Hour, Reason: "x"},
		"rule with target":     {Scope: SilenceRule, RuleID: &rule, TargetKey: "unit:a", Duration: time.Hour, Reason: "x"},
		"target without key":   {Scope: SilenceTarget, RuleID: &rule, Duration: time.Hour, Reason: "x"},
		"target without rule":  {Scope: SilenceTarget, TargetKey: "unit:a", Duration: time.Hour, Reason: "x"},
		"target key too long":  {Scope: SilenceTarget, RuleID: &rule, TargetKey: strings.Repeat("k", 513), Duration: time.Hour, Reason: "x"},
		"control in target":    {Scope: SilenceTarget, RuleID: &rule, TargetKey: "unit:\x00", Duration: time.Hour, Reason: "x"},
		"invalid UTF-8 reason": {Scope: SilenceAll, Duration: time.Hour, Reason: "\xff"},
	}
	for name, request := range invalid {
		if err := request.Validate(); !errors.Is(err, ErrInvalidSilence) {
			t.Errorf("%s: Validate() = %v, want ErrInvalidSilence", name, err)
		}
	}
}

func TestSilenceCoversItsScopeWhileActive(t *testing.T) {
	alert := firingAlert(AlertCritical)
	other := firingAlert(AlertCritical)
	ruleID := alert.RuleID
	active := func(s AlertSilence) AlertSilence {
		s.StartsAt, s.EndsAt = alertNow.Add(-time.Minute), alertNow.Add(time.Hour)
		return s
	}
	all := active(AlertSilence{Scope: SilenceAll})
	byRule := active(AlertSilence{Scope: SilenceRule, RuleID: &ruleID})
	byTarget := active(AlertSilence{Scope: SilenceTarget, RuleID: &ruleID, TargetKey: alert.TargetKey})
	otherTarget := active(AlertSilence{Scope: SilenceTarget, RuleID: &ruleID, TargetKey: "unit:other.service"})
	for name, test := range map[string]struct {
		silence AlertSilence
		alert   Alert
		want    bool
	}{
		"all covers anything":          {all, other, true},
		"rule covers its alerts":       {byRule, alert, true},
		"rule leaves other rules":      {byRule, other, false},
		"target covers its target":     {byTarget, alert, true},
		"target leaves other targets":  {otherTarget, alert, false},
		"same key under another rule":  {byTarget, Alert{RuleID: uuid.New(), TargetKey: alert.TargetKey}, false},
		"not started":                  {AlertSilence{Scope: SilenceAll, StartsAt: alertNow.Add(time.Minute), EndsAt: alertNow.Add(time.Hour)}, alert, false},
		"ended":                        {AlertSilence{Scope: SilenceAll, StartsAt: alertNow.Add(-time.Hour), EndsAt: alertNow}, alert, false},
		"lifted":                       {func() AlertSilence { s := all; lifted := alertNow; s.LiftedAt = &lifted; return s }(), alert, false},
		"lifted in the future is open": {func() AlertSilence { s := all; lifted := alertNow.Add(time.Minute); s.LiftedAt = &lifted; return s }(), alert, true},
	} {
		if got := Silenced(test.alert, []AlertSilence{test.silence}, alertNow); got != test.want {
			t.Errorf("%s: Silenced() = %v, want %v", name, got, test.want)
		}
	}
}

func validChannel(kind ChannelType) (NotificationChannel, ChannelSecrets) {
	channel := NotificationChannel{Name: "canal", Type: kind, Enabled: true, MinSeverity: AlertWarning, RenotifyMinutes: 30}
	secrets := ChannelSecrets{}
	switch kind {
	case ChannelTelegram:
		channel.Settings.ChatID = "-1001234567890"
		secrets[SecretBotToken] = "123456789:" + strings.Repeat("A", 35)
	case ChannelDiscord:
		secrets[SecretWebhookURL] = "https://discord.com/api/webhooks/123/abc_DEF-ghi"
	case ChannelGotify:
		channel.Settings.ServerURL = "http://192.168.1.10:8080"
		secrets[SecretAppToken] = "AbCdEf123456"
	case ChannelNtfy:
		channel.Settings.ServerURL, channel.Settings.Topic = "https://ntfy.sh", "homelab_alerts"
	case ChannelSMTP:
		channel.Settings = ChannelSettings{Host: "smtp.example.org", Port: 587, Security: SMTPStartTLS, Username: "bot", From: "bot@example.org", To: []string{"me@example.org"}}
		secrets[SecretPassword] = "correct horse"
	case ChannelWebhook:
		channel.Settings.URL = "https://hooks.example.org/homelab"
	}
	return channel, secrets
}

func TestChannelValidation(t *testing.T) {
	for _, kind := range ChannelTypes {
		channel, secrets := validChannel(kind)
		if err := channel.Validate(); err != nil {
			t.Errorf("%s: Validate() = %v, want nil", kind, err)
		}
		if err := ValidateChannelSecrets(channel, secrets); err != nil {
			t.Errorf("%s: ValidateChannelSecrets() = %v, want nil", kind, err)
		}
	}
	change := func(kind ChannelType, edit func(*NotificationChannel)) NotificationChannel {
		channel, _ := validChannel(kind)
		edit(&channel)
		return channel
	}
	invalid := map[string]NotificationChannel{
		"empty name":           change(ChannelNtfy, func(c *NotificationChannel) { c.Name = "" }),
		"unknown type":         change(ChannelNtfy, func(c *NotificationChannel) { c.Type = "slack" }),
		"unknown severity":     change(ChannelNtfy, func(c *NotificationChannel) { c.MinSeverity = "error" }),
		"renotify too short":   change(ChannelNtfy, func(c *NotificationChannel) { c.RenotifyMinutes = 4 }),
		"renotify too long":    change(ChannelNtfy, func(c *NotificationChannel) { c.RenotifyMinutes = 1441 }),
		"renotify negative":    change(ChannelNtfy, func(c *NotificationChannel) { c.RenotifyMinutes = -1 }),
		"telegram chat":        change(ChannelTelegram, func(c *NotificationChannel) { c.Settings.ChatID = "chat" }),
		"telegram extra":       change(ChannelTelegram, func(c *NotificationChannel) { c.Settings.URL = "https://x.org" }),
		"discord extra":        change(ChannelDiscord, func(c *NotificationChannel) { c.Settings.Topic = "x" }),
		"gotify ftp":           change(ChannelGotify, func(c *NotificationChannel) { c.Settings.ServerURL = "ftp://x.org" }),
		"gotify credentials":   change(ChannelGotify, func(c *NotificationChannel) { c.Settings.ServerURL = "https://u:p@x.org" }),
		"gotify query":         change(ChannelGotify, func(c *NotificationChannel) { c.Settings.ServerURL = "https://x.org/?token=a" }),
		"ntfy without topic":   change(ChannelNtfy, func(c *NotificationChannel) { c.Settings.Topic = "" }),
		"ntfy bad topic":       change(ChannelNtfy, func(c *NotificationChannel) { c.Settings.Topic = "a/b" }),
		"smtp port":            change(ChannelSMTP, func(c *NotificationChannel) { c.Settings.Port = 0 }),
		"smtp security":        change(ChannelSMTP, func(c *NotificationChannel) { c.Settings.Security = "none" }),
		"smtp host":            change(ChannelSMTP, func(c *NotificationChannel) { c.Settings.Host = "smtp example" }),
		"smtp from name":       change(ChannelSMTP, func(c *NotificationChannel) { c.Settings.From = "Bot <bot@example.org>" }),
		"smtp header newline":  change(ChannelSMTP, func(c *NotificationChannel) { c.Settings.To = []string{"a@example.org\r\nBcc: x@y.z"} }),
		"smtp no recipient":    change(ChannelSMTP, func(c *NotificationChannel) { c.Settings.To = nil }),
		"smtp 11 recipients":   change(ChannelSMTP, func(c *NotificationChannel) { c.Settings.To = strings.Split(strings.Repeat("a@b.org,", 11)[:88], ",") }),
		"smtp same recipient":  change(ChannelSMTP, func(c *NotificationChannel) { c.Settings.To = []string{"a@b.org", "a@b.org"} }),
		"webhook without url":  change(ChannelWebhook, func(c *NotificationChannel) { c.Settings.URL = "" }),
		"webhook relative url": change(ChannelWebhook, func(c *NotificationChannel) { c.Settings.URL = "/hook" }),
	}
	for name, channel := range invalid {
		if err := channel.Validate(); !errors.Is(err, ErrInvalidChannel) {
			t.Errorf("%s: Validate() = %v, want ErrInvalidChannel", name, err)
		}
	}
}

func TestChannelSecretsValidation(t *testing.T) {
	check := func(name string, kind ChannelType, edit func(NotificationChannel, ChannelSecrets) (NotificationChannel, ChannelSecrets), wantErr bool) {
		t.Helper()
		channel, secrets := validChannel(kind)
		channel, secrets = edit(channel, secrets)
		err := ValidateChannelSecrets(channel, secrets)
		if wantErr && !errors.Is(err, ErrInvalidChannel) {
			t.Errorf("%s: ValidateChannelSecrets() = %v, want ErrInvalidChannel", name, err)
		}
		if !wantErr && err != nil {
			t.Errorf("%s: ValidateChannelSecrets() = %v, want nil", name, err)
		}
	}
	set := func(key, value string) func(NotificationChannel, ChannelSecrets) (NotificationChannel, ChannelSecrets) {
		return func(c NotificationChannel, s ChannelSecrets) (NotificationChannel, ChannelSecrets) {
			if value == "" {
				delete(s, key)
			} else {
				s[key] = value
			}
			return c, s
		}
	}
	check("telegram without token", ChannelTelegram, set(SecretBotToken, ""), true)
	check("telegram malformed token", ChannelTelegram, set(SecretBotToken, "not-a-token"), true)
	check("discord other host", ChannelDiscord, set(SecretWebhookURL, "https://evil.example/api/webhooks/1/a"), true)
	check("discord not a webhook", ChannelDiscord, set(SecretWebhookURL, "https://discord.com/channels/1"), true)
	check("discord http", ChannelDiscord, set(SecretWebhookURL, "http://discord.com/api/webhooks/1/a"), true)
	check("gotify without token", ChannelGotify, set(SecretAppToken, ""), true)
	check("ntfy with token", ChannelNtfy, set(SecretAccessToken, "tk_abcdef"), false)
	check("unknown secret", ChannelNtfy, set("password", "x"), true)
	check("secret with newline", ChannelGotify, set(SecretAppToken, "a\nb"), true)
	check("secret too long", ChannelGotify, set(SecretAppToken, strings.Repeat("a", 1025)), true)
	check("smtp user without password", ChannelSMTP, set(SecretPassword, ""), true)
	check("smtp password without user", ChannelSMTP, func(c NotificationChannel, s ChannelSecrets) (NotificationChannel, ChannelSecrets) {
		c.Settings.Username = ""
		return c, s
	}, true)
	check("smtp anonymous", ChannelSMTP, func(c NotificationChannel, s ChannelSecrets) (NotificationChannel, ChannelSecrets) {
		c.Settings.Username = ""
		return c, ChannelSecrets{}
	}, false)
	check("webhook short signing secret", ChannelWebhook, set(SecretSigningSecret, "short"), true)
	check("webhook signing secret", ChannelWebhook, set(SecretSigningSecret, strings.Repeat("s", 32)), false)
}

func TestMergeChannelSecrets(t *testing.T) {
	current := ChannelSecrets{"a": "1", "b": "2"}
	replacement, empty := "3", ""
	merged := MergeChannelSecrets(current, map[string]*string{"a": &replacement, "b": nil, "c": &empty})
	if len(merged) != 2 || merged["a"] != "3" || merged["c"] != "" {
		t.Fatalf("MergeChannelSecrets() = %v, want a replaced, b removed, c kept for validation", merged)
	}
	if current["a"] != "1" || current["b"] != "2" {
		t.Fatalf("MergeChannelSecrets changed its input: %v", current)
	}
}

func TestSecretFingerprintIsTwelveHexCharacters(t *testing.T) {
	sum := sha256.Sum256([]byte("token"))
	if got, want := SecretFingerprint("token"), hex.EncodeToString(sum[:])[:12]; got != want {
		t.Fatalf("SecretFingerprint() = %q, want %q", got, want)
	}
}

func TestChannelRoutesBySeverityAndSilence(t *testing.T) {
	channel, _ := validChannel(ChannelNtfy)
	warning, info := firingAlert(AlertWarning), firingAlert(AlertInfo)
	if !channel.Routes(Notification{Event: NotifyFiring, Alert: warning}, nil, alertNow) {
		t.Fatal("a warning alert is not routed to a warning channel")
	}
	if channel.Routes(Notification{Event: NotifyFiring, Alert: info}, nil, alertNow) {
		t.Fatal("an info alert is routed to a warning channel")
	}
	silence := AlertSilence{Scope: SilenceAll, StartsAt: alertNow.Add(-time.Minute), EndsAt: alertNow.Add(time.Minute)}
	if channel.Routes(Notification{Event: NotifyResolved, Alert: warning}, []AlertSilence{silence}, alertNow) {
		t.Fatal("a silenced alert is routed")
	}
	channel.Enabled = false
	if channel.Routes(Notification{Event: NotifyFiring, Alert: warning}, nil, alertNow) {
		t.Fatal("a disabled channel receives notifications")
	}
}

func TestDueReminders(t *testing.T) {
	channel, _ := validChannel(ChannelNtfy)
	old := firingAlert(AlertCritical)
	recent := firingAlert(AlertCritical)
	recent.FiredAt = alertNow.Add(-5 * time.Minute)
	acknowledged := firingAlert(AlertCritical)
	acknowledged.FiredAt, acknowledged.State = alertNow.Add(-time.Hour), AlertAcknowledged
	notified := firingAlert(AlertCritical)
	notified.FiredAt = alertNow.Add(-time.Hour)
	at := alertNow
	notified.LastNotifiedAt = &at
	low := firingAlert(AlertInfo)
	low.FiredAt = alertNow.Add(-time.Hour)
	remembered := firingAlert(AlertCritical)
	remembered.FiredAt = alertNow.Add(-2 * time.Hour)
	channel.RenotifyMinutes = 30
	due := DueReminders(channel, []Alert{old, recent, acknowledged, notified, low, remembered}, nil,
		map[uuid.UUID]time.Time{remembered.ID: alertNow.Add(5 * time.Minute)}, alertNow.Add(20*time.Minute))
	if len(due) != 1 || due[0].ID != old.ID {
		t.Fatalf("DueReminders() = %v, want only the alert fired 30 min ago", due)
	}
	silence := AlertSilence{Scope: SilenceAll, StartsAt: alertNow, EndsAt: alertNow.Add(time.Hour)}
	if due := DueReminders(channel, []Alert{old}, []AlertSilence{silence}, nil, alertNow.Add(20*time.Minute)); len(due) != 0 {
		t.Fatalf("DueReminders() = %v for a silenced alert, want none", due)
	}
	channel.RenotifyMinutes = 0
	if due := DueReminders(channel, []Alert{old}, nil, nil, alertNow.Add(time.Hour)); len(due) != 0 {
		t.Fatalf("DueReminders() = %v without renotification, want none", due)
	}
}

func TestNotificationQueueKeepsTheNewestAndCountsTheOthers(t *testing.T) {
	var queue NotificationQueue
	alerts := make([]Alert, NotificationQueueCapacity+5)
	for i := range alerts {
		alerts[i] = firingAlert(AlertWarning)
		queue.Push(Notification{Event: NotifyFiring, Alert: alerts[i]})
	}
	if queue.Len() != NotificationQueueCapacity {
		t.Fatalf("Len() = %d, want %d", queue.Len(), NotificationQueueCapacity)
	}
	batch := queue.Drain()
	if batch.Omitted != 5 || len(batch.Notifications) != NotificationQueueCapacity || batch.Notifications[0].Alert.ID != alerts[5].ID {
		t.Fatalf("Drain() kept %d (first %s), omitted %d; want the %d newest and 5 omitted",
			len(batch.Notifications), batch.Notifications[0].Alert.ID, batch.Omitted, NotificationQueueCapacity)
	}
	if queue.Len() != 0 || queue.Drain().Omitted != 0 {
		t.Fatal("Drain() did not empty the queue")
	}
}

func TestRenderNotificationInFrench(t *testing.T) {
	unit := firingAlert(AlertCritical)
	disk := Alert{
		ID: uuid.New(), RuleName: "Disque presque plein", RuleType: AlertRuleHostMetric, Severity: AlertWarning,
		Target: AlertTarget{Kind: AlertTargetHost, Metric: "filesystem.percent", Labels: map[string]string{"mountpoint": "/", "device": "sda1"}},
		Value:  float(91.456), Threshold: float(85), State: AlertResolved, FiredAt: alertNow.Add(-75 * time.Minute),
		ResolvedAt: &alertNow, Resolution: ResolutionConditionCleared,
	}
	reminder := Alert{
		ID: uuid.New(), RuleName: "Conteneur arrêté", RuleType: AlertRuleContainerState, Severity: AlertInfo,
		Target: AlertTarget{Kind: AlertTargetContainer, ContainerName: "nginx"}, Observed: "exited", State: AlertFiring,
		FiredAt: alertNow.Add(-26 * time.Hour),
	}
	message := RenderNotification("nas", NotificationBatch{Notifications: []Notification{
		{Event: NotifyFiring, Alert: unit}, {Event: NotifyResolved, Alert: disk}, {Event: NotifyReminder, Alert: reminder},
	}, Omitted: 2}, alertNow, 0)
	if message.Title != "nas · 1 déclenchée, 1 résolue, 1 toujours active, 2 autres" {
		t.Errorf("Title = %q", message.Title)
	}
	for _, want := range []string{
		"Déclenchée · Critique · Unité systemd en échec\nUnité nginx.service · état failed (exit-code)",
		"Résolue · Avertissement · Disque presque plein\nHôte · filesystem.percent (device=sda1, mountpoint=/) · valeur 91,46, seuil 85 · résolue après 1 h 15 min (condition levée)",
		"Toujours active · Info · Conteneur arrêté\nConteneur nginx · état exited · active depuis 1 j 2 h",
		"… et 2 autres notifications",
	} {
		if !strings.Contains(message.Body, want) {
			t.Errorf("Body misses %q:\n%s", want, message.Body)
		}
	}
	single := RenderNotification("nas", NotificationBatch{Notifications: []Notification{{Event: NotifyFiring, Alert: unit}}}, alertNow, 0)
	if single.Title != "nas · Critique · Unité systemd en échec" {
		t.Errorf("single Title = %q", single.Title)
	}
	test := RenderNotification("nas", NotificationBatch{Test: true}, alertNow, 0)
	if test.Title != "nas · Test de notification" || !strings.Contains(test.Body, "Homelab Manager") {
		t.Errorf("test message = %+v", test)
	}
}

func TestRenderNotificationFitsTheLimit(t *testing.T) {
	var batch NotificationBatch
	for range 50 {
		alert := firingAlert(AlertCritical)
		alert.RuleName = strings.Repeat("règle très longue ", 5)
		batch.Notifications = append(batch.Notifications, Notification{Event: NotifyFiring, Alert: alert})
	}
	message := RenderNotification("nas", batch, alertNow, 2000)
	if count := utf8.RuneCountInString(message.Body); count > 2000 {
		t.Fatalf("Body has %d characters, want at most 2000", count)
	}
	if !strings.Contains(message.Body, "autres notifications") {
		t.Fatalf("a truncated body does not tell how many notifications are left out:\n%s", message.Body)
	}
}

func TestWebhookPayloadAndSignature(t *testing.T) {
	alert := firingAlert(AlertCritical)
	body, err := WebhookPayload("nas", NotificationBatch{Notifications: []Notification{{Event: NotifyFiring, Alert: alert}}, Omitted: 1}, alertNow)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Version       int       `json:"version"`
		Host          string    `json:"host"`
		SentAt        time.Time `json:"sentAt"`
		Test          bool      `json:"test"`
		Omitted       int       `json:"omitted"`
		Notifications []struct {
			Event string `json:"event"`
			Alert struct {
				ID        string `json:"id"`
				Severity  string `json:"severity"`
				TargetKey string `json:"targetKey"`
				Observed  string `json:"observed"`
				Target    struct {
					Kind string `json:"kind"`
					Unit string `json:"unit"`
				} `json:"target"`
			} `json:"alert"`
		} `json:"notifications"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	n := decoded.Notifications
	if decoded.Version != 1 || decoded.Host != "nas" || !decoded.SentAt.Equal(alertNow) || decoded.Omitted != 1 || len(n) != 1 ||
		n[0].Event != "firing" || n[0].Alert.ID != alert.ID.String() || n[0].Alert.Severity != "critical" ||
		n[0].Alert.TargetKey != "unit:nginx.service" || n[0].Alert.Target.Unit != "nginx.service" || n[0].Alert.Observed != "failed (exit-code)" {
		t.Fatalf("payload = %s", body)
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte("1764000000." + string(body)))
	if got, want := SignWebhook([]byte("secret"), 1764000000, body), "sha256="+hex.EncodeToString(mac.Sum(nil)); got != want {
		t.Fatalf("SignWebhook() = %q, want %q", got, want)
	}
}

func TestNotificationBatchHighestSeverity(t *testing.T) {
	batch := NotificationBatch{Notifications: []Notification{{Alert: firingAlert(AlertInfo)}, {Alert: firingAlert(AlertCritical)}, {Alert: firingAlert(AlertWarning)}}}
	if got := batch.HighestSeverity(); got != AlertCritical {
		t.Fatalf("HighestSeverity() = %s, want critical", got)
	}
	if got := (NotificationBatch{Test: true}).HighestSeverity(); got != AlertInfo {
		t.Fatalf("HighestSeverity() of a test = %s, want info", got)
	}
}

func TestDeliveryErrorHidesDetails(t *testing.T) {
	err := error(&DeliveryError{Code: DeliveryHTTPStatus, Status: 401})
	if err.Error() != "notification delivery failed: http_status 401" {
		t.Fatalf("Error() = %q", err.Error())
	}
	var delivery *DeliveryError
	if !errors.As(fmtWrap(err), &delivery) || delivery.Status != 401 {
		t.Fatal("a wrapped DeliveryError is not found by errors.As")
	}
}

func fmtWrap(err error) error { return errors.Join(errors.New("send"), err) }
