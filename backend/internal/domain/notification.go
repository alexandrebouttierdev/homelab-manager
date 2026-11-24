package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Bounds of the notifications (SPEC_ADMINISTRATION.md § 6.2, § 6.3).
const (
	MinSilenceDuration = 15 * time.Minute
	MaxSilenceDuration = 7 * 24 * time.Hour
	// MaxActiveSilences bounds the silences in force at once.
	MaxActiveSilences = 100
	// SilenceHistory is how long an ended silence stays listed.
	SilenceHistory = 7 * 24 * time.Hour
	// MaxListedSilences bounds the silence list.
	MaxListedSilences = 200

	MaxNotificationChannels = 20
	DefaultRenotifyMinutes  = 30
	MinRenotifyMinutes      = 5
	MaxRenotifyMinutes      = 1440
	MaxSMTPRecipients       = 10
	// NotificationQueueCapacity bounds the transitions waiting for one
	// channel; older ones are counted, not kept.
	NotificationQueueCapacity = 200
	// NotificationGrouping is the minimum delay between two messages of
	// one channel.
	NotificationGrouping = time.Minute

	maxSilenceReasonRunes = 200
	maxTargetKeyBytes     = 512
	maxChannelNameBytes   = 100
	maxURLBytes           = 2048
	maxSecretBytes        = 1024
	maxSettingBytes       = 255
	minSigningSecretBytes = 16
	fingerprintLength     = 12
)

var (
	ErrInvalidSilence     = errors.New("alert silence is invalid")
	ErrSilenceNotFound    = errors.New("alert silence not found")
	ErrTooManySilences    = errors.New("too many active alert silences")
	ErrInvalidChannel     = errors.New("notification channel is invalid")
	ErrChannelNotFound    = errors.New("notification channel not found")
	ErrTooManyChannels    = errors.New("too many notification channels")
	ErrSecretsUnavailable = errors.New("the secrets key is unavailable")
	telegramChatPattern   = regexp.MustCompile(`^(-?[0-9]{1,20}|@[A-Za-z0-9_]{5,32})$`)
	telegramTokenPattern  = regexp.MustCompile(`^[0-9]{1,20}:[A-Za-z0-9_-]{30,100}$`)
	ntfyTopicPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	hostnamePattern       = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	discordHosts          = []string{"discord.com", "discordapp.com"}
	errInvalidURL         = errors.New("invalid URL")
)

// SilenceScope is what a silence covers: the alerts of one rule, the alert
// of one rule on one target, or every alert.
type SilenceScope string

const (
	SilenceRule   SilenceScope = "rule"
	SilenceTarget SilenceScope = "target"
	SilenceAll    SilenceScope = "all"
)

// SilenceRequest is a silence asked by a user; it starts at once.
type SilenceRequest struct {
	Scope     SilenceScope
	RuleID    *uuid.UUID
	TargetKey string
	Duration  time.Duration
	Reason    string
}

func invalidSilence(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSilence, fmt.Sprintf(format, args...))
}

// Validate checks a silence request received from a client.
func (r SilenceRequest) Validate() error {
	switch r.Scope {
	case SilenceAll:
		if r.RuleID != nil || r.TargetKey != "" {
			return invalidSilence("a global silence takes no ruleId nor targetKey")
		}
	case SilenceRule:
		if r.RuleID == nil || r.TargetKey != "" {
			return invalidSilence("a rule silence takes a ruleId and no targetKey")
		}
	case SilenceTarget:
		if r.RuleID == nil || r.TargetKey == "" {
			return invalidSilence("a target silence takes a ruleId and a targetKey")
		}
		if len(r.TargetKey) > maxTargetKeyBytes || !utf8.ValidString(r.TargetKey) || strings.IndexFunc(r.TargetKey, unicode.IsControl) >= 0 {
			return invalidSilence("targetKey must be 1 to %d bytes without control characters", maxTargetKeyBytes)
		}
	default:
		return invalidSilence("scope must be rule, target or all")
	}
	if r.Duration < MinSilenceDuration || r.Duration > MaxSilenceDuration {
		return invalidSilence("the duration must be 15 minutes to 7 days")
	}
	if strings.TrimSpace(r.Reason) == "" || !utf8.ValidString(r.Reason) || utf8.RuneCountInString(r.Reason) > maxSilenceReasonRunes ||
		strings.IndexFunc(r.Reason, unicode.IsControl) >= 0 {
		return invalidSilence("reason must be 1 to %d characters without control characters", maxSilenceReasonRunes)
	}
	return nil
}

// AlertSilence suspends the notifications of the alerts it covers; the
// alerts stay open and visible (decision Q-7).
type AlertSilence struct {
	ID        uuid.UUID
	Scope     SilenceScope
	RuleID    *uuid.UUID
	TargetKey string
	Reason    string
	CreatedBy string
	StartsAt  time.Time
	EndsAt    time.Time
	LiftedAt  *time.Time
	LiftedBy  string
}

// ActiveAt reports whether the silence is in force at now.
func (s AlertSilence) ActiveAt(now time.Time) bool {
	end := s.EndsAt
	if s.LiftedAt != nil && s.LiftedAt.Before(end) {
		end = *s.LiftedAt
	}
	return !now.Before(s.StartsAt) && now.Before(end)
}

// Covers reports whether the scope of the silence includes the alert.
func (s AlertSilence) Covers(alert Alert) bool {
	switch s.Scope {
	case SilenceAll:
		return true
	case SilenceRule:
		return s.RuleID != nil && *s.RuleID == alert.RuleID
	case SilenceTarget:
		return s.RuleID != nil && *s.RuleID == alert.RuleID && s.TargetKey == alert.TargetKey
	}
	return false
}

// Silenced reports whether an active silence covers the alert.
func Silenced(alert Alert, silences []AlertSilence, now time.Time) bool {
	return slices.ContainsFunc(silences, func(s AlertSilence) bool { return s.ActiveAt(now) && s.Covers(alert) })
}

// ChannelType is one of the channels of SPEC_ADMINISTRATION.md § 6.3.
type ChannelType string

const (
	ChannelTelegram ChannelType = "telegram"
	ChannelDiscord  ChannelType = "discord"
	ChannelGotify   ChannelType = "gotify"
	ChannelNtfy     ChannelType = "ntfy"
	ChannelSMTP     ChannelType = "smtp"
	ChannelWebhook  ChannelType = "webhook"
)

// ChannelTypes lists the channel types.
var ChannelTypes = []ChannelType{ChannelTelegram, ChannelDiscord, ChannelGotify, ChannelNtfy, ChannelSMTP, ChannelWebhook}

// SMTPSecurity is how the SMTP connection is encrypted; plain SMTP is not
// offered.
type SMTPSecurity string

const (
	SMTPStartTLS SMTPSecurity = "starttls"
	SMTPTLS      SMTPSecurity = "tls"
)

// Names of the channel secrets.
const (
	SecretBotToken      = "botToken"
	SecretWebhookURL    = "webhookUrl"
	SecretAppToken      = "appToken"
	SecretAccessToken   = "accessToken"
	SecretPassword      = "password"
	SecretSigningSecret = "signingSecret"
)

// channelSecrets maps each type to its secrets and whether each is
// required. The SMTP password is required with a username.
var channelSecrets = map[ChannelType]map[string]bool{
	ChannelTelegram: {SecretBotToken: true},
	ChannelDiscord:  {SecretWebhookURL: true},
	ChannelGotify:   {SecretAppToken: true},
	ChannelNtfy:     {SecretAccessToken: false},
	ChannelSMTP:     {SecretPassword: false},
	ChannelWebhook:  {SecretSigningSecret: false},
}

// ChannelSecretNames lists the secrets a channel type accepts.
func ChannelSecretNames(kind ChannelType) []string {
	return slices.Sorted(maps.Keys(channelSecrets[kind]))
}

// ChannelSettings holds the settings of every type that are not secret;
// each type uses its own and leaves the others empty.
type ChannelSettings struct {
	// ChatID: Telegram, a chat number or @channel name.
	ChatID string `json:"chatId,omitempty"`
	// ServerURL: Gotify and ntfy.
	ServerURL string `json:"serverUrl,omitempty"`
	// Topic: ntfy.
	Topic string `json:"topic,omitempty"`
	// Host, Port, Security, Username, From and To: SMTP.
	Host     string       `json:"host,omitempty"`
	Port     int          `json:"port,omitempty"`
	Security SMTPSecurity `json:"security,omitempty"`
	Username string       `json:"username,omitempty"`
	From     string       `json:"from,omitempty"`
	To       []string     `json:"to,omitempty"`
	// URL: webhook.
	URL string `json:"url,omitempty"`
}

// ChannelSecrets maps a secret name to its value. It only exists in clear
// while a channel is validated, sealed or used.
type ChannelSecrets map[string]string

// DeliveryErrorCode tells why a message could not be sent, without any
// address, token or server answer.
type DeliveryErrorCode string

const (
	DeliveryTimeout    DeliveryErrorCode = "timeout"
	DeliveryConnection DeliveryErrorCode = "connection_failed"
	DeliveryTLS        DeliveryErrorCode = "tls_failed"
	DeliveryRedirect   DeliveryErrorCode = "redirect_refused"
	DeliveryHTTPStatus DeliveryErrorCode = "http_status"
	DeliverySMTP       DeliveryErrorCode = "smtp_rejected"
	DeliverySecrets    DeliveryErrorCode = "secrets_unavailable"
	DeliveryFailed     DeliveryErrorCode = "failed"
)

// DeliveryError is a failed delivery; Status is the HTTP status or the SMTP
// reply code when there is one.
type DeliveryError struct {
	Code   DeliveryErrorCode
	Status int
}

func (e *DeliveryError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("notification delivery failed: %s %d", e.Code, e.Status)
	}
	return "notification delivery failed: " + string(e.Code)
}

// NotificationChannel is a destination of the notifications. Its secrets
// are sealed by the secrets box: SealedSecrets is opaque, SecretNames lists
// the secrets it holds.
type NotificationChannel struct {
	ID              uuid.UUID
	Name            string
	Type            ChannelType
	Enabled         bool
	MinSeverity     AlertSeverity
	RenotifyMinutes int
	Settings        ChannelSettings
	SealedSecrets   string
	SecretNames     []string
	LastSentAt      *time.Time
	LastErrorAt     *time.Time
	LastError       DeliveryErrorCode
	LastErrorStatus int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func invalidChannel(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidChannel, fmt.Sprintf(format, args...))
}

// Validate checks the settings of a channel received from a client; the
// secrets are checked by ValidateChannelSecrets.
func (c NotificationChannel) Validate() error {
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > maxChannelNameBytes || !utf8.ValidString(c.Name) || strings.IndexFunc(c.Name, unicode.IsControl) >= 0 {
		return invalidChannel("name must be 1 to %d bytes without control characters", maxChannelNameBytes)
	}
	if !slices.Contains(AlertSeverities, c.MinSeverity) {
		return invalidChannel("minSeverity must be info, warning or critical")
	}
	if c.RenotifyMinutes != 0 && (c.RenotifyMinutes < MinRenotifyMinutes || c.RenotifyMinutes > MaxRenotifyMinutes) {
		return invalidChannel("renotifyMinutes must be 0 or %d to %d", MinRenotifyMinutes, MaxRenotifyMinutes)
	}
	s := c.Settings
	only := func(allowed ...string) error {
		used := map[string]bool{
			"chatId": s.ChatID != "", "serverUrl": s.ServerURL != "", "topic": s.Topic != "", "host": s.Host != "",
			"port": s.Port != 0, "security": s.Security != "", "username": s.Username != "", "from": s.From != "",
			"to": len(s.To) > 0, "url": s.URL != "",
		}
		for name, set := range used {
			if set && !slices.Contains(allowed, name) {
				return invalidChannel("a %s channel does not take %s", c.Type, name)
			}
		}
		return nil
	}
	switch c.Type {
	case ChannelTelegram:
		if err := only("chatId"); err != nil {
			return err
		}
		if !telegramChatPattern.MatchString(s.ChatID) {
			return invalidChannel("chatId must be a chat number or an @channel name")
		}
	case ChannelDiscord:
		return only()
	case ChannelGotify:
		if err := only("serverUrl"); err != nil {
			return err
		}
		return validateServerURL("serverUrl", s.ServerURL)
	case ChannelNtfy:
		if err := only("serverUrl", "topic"); err != nil {
			return err
		}
		if !ntfyTopicPattern.MatchString(s.Topic) {
			return invalidChannel("topic must be 1 to 64 letters, digits, - or _")
		}
		return validateServerURL("serverUrl", s.ServerURL)
	case ChannelSMTP:
		if err := only("host", "port", "security", "username", "from", "to"); err != nil {
			return err
		}
		return validateSMTP(s)
	case ChannelWebhook:
		if err := only("url"); err != nil {
			return err
		}
		return validateServerURL("url", s.URL)
	default:
		return invalidChannel("type must be telegram, discord, gotify, ntfy, smtp or webhook")
	}
	return nil
}

// validateServerURL accepts an absolute http or https URL without
// credentials, query nor fragment. Private addresses are allowed: Gotify
// and ntfy often run on the local network.
func validateServerURL(name, value string) error {
	parsed, err := parseHTTPURL(value)
	if err != nil || parsed.RawQuery != "" || parsed.ForceQuery {
		return invalidChannel("%s must be an http or https URL without credentials, query nor fragment", name)
	}
	return nil
}

func parseHTTPURL(value string) (*url.URL, error) {
	if value == "" || len(value) > maxURLBytes || strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.ContainsAny(value, " ") {
		return nil, errInvalidURL
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return nil, errInvalidURL
	}
	return parsed, nil
}

func validateSMTP(s ChannelSettings) error {
	if len(s.Host) > maxSettingBytes || (!hostnamePattern.MatchString(s.Host) && net.ParseIP(s.Host) == nil) {
		return invalidChannel("host must be a host name or an IP address")
	}
	if s.Port < 1 || s.Port > 65535 {
		return invalidChannel("port must be 1 to 65535")
	}
	if s.Security != SMTPStartTLS && s.Security != SMTPTLS {
		return invalidChannel("security must be starttls or tls")
	}
	if len(s.Username) > maxSettingBytes || strings.IndexFunc(s.Username, unicode.IsControl) >= 0 || !utf8.ValidString(s.Username) {
		return invalidChannel("username must be at most %d bytes without control characters", maxSettingBytes)
	}
	if !plainAddress(s.From) {
		return invalidChannel("from must be a plain e-mail address")
	}
	if len(s.To) == 0 || len(s.To) > MaxSMTPRecipients {
		return invalidChannel("to must hold 1 to %d addresses", MaxSMTPRecipients)
	}
	for i, address := range s.To {
		if !plainAddress(address) || slices.Contains(s.To[:i], address) {
			return invalidChannel("to must hold distinct plain e-mail addresses")
		}
	}
	return nil
}

// plainAddress accepts "user@example.org" only: no display name, comment
// nor line break that could inject a header.
func plainAddress(value string) bool {
	if value == "" || len(value) > maxSettingBytes || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Name == "" && parsed.Address == value
}

// ValidateChannelSecretNames checks which secrets a channel holds: no
// unknown one, the required ones present, the SMTP password with a
// username only. It needs no secret value.
func ValidateChannelSecretNames(c NotificationChannel, names []string) error {
	accepted := channelSecrets[c.Type]
	for _, name := range names {
		if _, ok := accepted[name]; !ok {
			return invalidChannel("a %s channel has no secret %q", c.Type, name)
		}
	}
	for name, required := range accepted {
		if required && !slices.Contains(names, name) {
			return invalidChannel("secret %s is required", name)
		}
	}
	if c.Type == ChannelSMTP && slices.Contains(names, SecretPassword) != (c.Settings.Username != "") {
		return invalidChannel("username and password go together")
	}
	return nil
}

// ValidateChannelSecrets checks the complete set of secrets of a channel:
// the names as ValidateChannelSecretNames, and the values well formed.
func ValidateChannelSecrets(c NotificationChannel, secrets ChannelSecrets) error {
	if err := ValidateChannelSecretNames(c, slices.Sorted(maps.Keys(secrets))); err != nil {
		return err
	}
	for name, value := range secrets {
		if value == "" || len(value) > maxSecretBytes || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return invalidChannel("secret %s must be 1 to %d bytes without control characters", name, maxSecretBytes)
		}
	}
	switch c.Type {
	case ChannelTelegram:
		if !telegramTokenPattern.MatchString(secrets[SecretBotToken]) {
			return invalidChannel("botToken must be a Telegram bot token")
		}
	case ChannelDiscord:
		parsed, err := parseHTTPURL(secrets[SecretWebhookURL])
		if err != nil || parsed.Scheme != "https" || !slices.Contains(discordHosts, parsed.Hostname()) ||
			!strings.HasPrefix(parsed.Path, "/api/webhooks/") || parsed.Port() != "" {
			return invalidChannel("webhookUrl must be a Discord webhook URL")
		}
	case ChannelWebhook:
		if value, ok := secrets[SecretSigningSecret]; ok && len(value) < minSigningSecretBytes {
			return invalidChannel("signingSecret must be at least %d bytes", minSigningSecretBytes)
		}
	}
	return nil
}

// MergeChannelSecrets applies changes to the current secrets: an absent
// name keeps its value, nil removes it, a value replaces it. The input is
// not modified.
func MergeChannelSecrets(current ChannelSecrets, changes map[string]*string) ChannelSecrets {
	merged := maps.Clone(current)
	if merged == nil {
		merged = ChannelSecrets{}
	}
	for name, value := range changes {
		if value == nil {
			delete(merged, name)
		} else {
			merged[name] = *value
		}
	}
	return merged
}

// SecretFingerprint is the beginning of the SHA-256 of a secret, enough to
// tell two values apart without revealing them.
func SecretFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:fingerprintLength]
}

// ChannelSecretsAssociatedData binds sealed secrets to their channel.
func ChannelSecretsAssociatedData(id uuid.UUID) string {
	return "notification-channel:" + id.String()
}

// Accepts reports whether the channel receives alerts of this severity.
func (c NotificationChannel) Accepts(severity AlertSeverity) bool {
	return slices.Index(AlertSeverities, severity) >= slices.Index(AlertSeverities, c.MinSeverity)
}

// Routes reports whether a transition goes to the channel: enabled,
// severity accepted, alert not silenced.
func (c NotificationChannel) Routes(n Notification, silences []AlertSilence, now time.Time) bool {
	return c.Enabled && c.Accepts(n.Alert.Severity) && !Silenced(n.Alert, silences, now)
}

// DueReminders lists the open alerts the channel must remind: firing (not
// acknowledged), accepted, not silenced, and last notified at least
// RenotifyMinutes ago. reminded holds the last reminder sent by this
// channel since the start; otherwise the last notification of the alert,
// else its firing, counts.
func DueReminders(c NotificationChannel, open []Alert, silences []AlertSilence, reminded map[uuid.UUID]time.Time, now time.Time) []Alert {
	if !c.Enabled || c.RenotifyMinutes <= 0 {
		return nil
	}
	interval := time.Duration(c.RenotifyMinutes) * time.Minute
	var due []Alert
	for _, alert := range open {
		if alert.State != AlertFiring || !c.Accepts(alert.Severity) || Silenced(alert, silences, now) {
			continue
		}
		last, ok := reminded[alert.ID]
		if !ok {
			last = alert.FiredAt
			if alert.LastNotifiedAt != nil && alert.LastNotifiedAt.After(last) {
				last = *alert.LastNotifiedAt
			}
		}
		if now.Sub(last) >= interval {
			due = append(due, alert)
		}
	}
	return due
}

// NotificationEvent is the transition a notification reports.
type NotificationEvent string

const (
	NotifyFiring   NotificationEvent = "firing"
	NotifyResolved NotificationEvent = "resolved"
	NotifyReminder NotificationEvent = "reminder"
)

// Notification is one transition of one alert.
type Notification struct {
	Event NotificationEvent
	Alert Alert
}

// NotificationQueue holds the transitions waiting for one channel. It
// keeps the newest NotificationQueueCapacity and counts the others.
type NotificationQueue struct {
	items   []Notification
	omitted int
}

func (q *NotificationQueue) Push(n Notification) {
	if len(q.items) == NotificationQueueCapacity {
		q.items = slices.Delete(q.items, 0, 1)
		q.omitted++
	}
	q.items = append(q.items, n)
}

func (q *NotificationQueue) Len() int { return len(q.items) }

// Drain empties the queue into a batch, oldest first.
func (q *NotificationQueue) Drain() NotificationBatch {
	batch := NotificationBatch{Notifications: q.items, Omitted: q.omitted}
	q.items, q.omitted = nil, 0
	return batch
}

// NotificationBatch is the content of one message. Test marks the message
// sent by the « Tester » button.
type NotificationBatch struct {
	Notifications []Notification
	Omitted       int
	Test          bool
}

// HighestSeverity gives the priority of the message; info for a test.
func (b NotificationBatch) HighestSeverity() AlertSeverity {
	highest := AlertInfo
	for _, n := range b.Notifications {
		if slices.Index(AlertSeverities, n.Alert.Severity) > slices.Index(AlertSeverities, highest) {
			highest = n.Alert.Severity
		}
	}
	return highest
}

// NotificationMessage is the text of a message for the chat and e-mail
// channels.
type NotificationMessage struct {
	Title string
	Body  string
}

var (
	severityLabels = map[AlertSeverity]string{AlertInfo: "Info", AlertWarning: "Avertissement", AlertCritical: "Critique"}
	eventLabels    = map[NotificationEvent]string{NotifyFiring: "Déclenchée", NotifyResolved: "Résolue", NotifyReminder: "Toujours active"}
	resolutionText = map[AlertResolution]string{
		ResolutionConditionCleared: "condition levée", ResolutionNoData: "données absentes", ResolutionTargetGone: "cible disparue",
		ResolutionRuleDisabled: "règle désactivée", ResolutionRuleDeleted: "règle supprimée", ResolutionRuleChanged: "règle modifiée",
	}
	collectorLabels = map[string]string{"system": "système", "docker": "Docker", "services": "services systemd"}
)

// RenderNotification writes a batch in French. maxRunes bounds the body
// (0: no bound); notifications that do not fit are counted at the end.
func RenderNotification(host string, batch NotificationBatch, now time.Time, maxRunes int) NotificationMessage {
	if batch.Test {
		return NotificationMessage{
			Title: host + " · Test de notification",
			Body:  "Message de test envoyé par Homelab Manager le " + now.UTC().Format("02/01/2006 à 15:04") + " UTC. Ce canal fonctionne.",
		}
	}
	title := host + " · " + batchSummary(batch)
	if len(batch.Notifications) == 1 && batch.Omitted == 0 {
		n := batch.Notifications[0]
		prefix := severityLabels[n.Alert.Severity]
		if n.Event == NotifyResolved {
			prefix = eventLabels[NotifyResolved]
		}
		title = host + " · " + prefix + " · " + n.Alert.RuleName
	}
	var body strings.Builder
	omitted := batch.Omitted
	for i, n := range batch.Notifications {
		block := renderNotification(n, now)
		if i > 0 {
			block = "\n\n" + block
		}
		remaining := omitted + len(batch.Notifications) - i - 1
		if maxRunes > 0 {
			reserve := 0
			if remaining > 0 {
				reserve = utf8.RuneCountInString(moreLine(remaining + 1))
			}
			if utf8.RuneCountInString(body.String())+utf8.RuneCountInString(block)+reserve > maxRunes {
				omitted += len(batch.Notifications) - i
				break
			}
		}
		body.WriteString(block)
	}
	if omitted > 0 {
		body.WriteString(moreLine(omitted))
	}
	return NotificationMessage{Title: title, Body: body.String()}
}

func moreLine(count int) string {
	if count == 1 {
		return "\n\n… et 1 autre notification"
	}
	return fmt.Sprintf("\n\n… et %d autres notifications", count)
}

func batchSummary(batch NotificationBatch) string {
	counts := map[NotificationEvent]int{}
	for _, n := range batch.Notifications {
		counts[n.Event]++
	}
	var parts []string
	for _, item := range []struct {
		event            NotificationEvent
		singular, plural string
	}{{NotifyFiring, "déclenchée", "déclenchées"}, {NotifyResolved, "résolue", "résolues"}, {NotifyReminder, "toujours active", "toujours actives"}} {
		switch count := counts[item.event]; {
		case count == 1:
			parts = append(parts, "1 "+item.singular)
		case count > 1:
			parts = append(parts, strconv.Itoa(count)+" "+item.plural)
		}
	}
	if batch.Omitted > 0 {
		parts = append(parts, strconv.Itoa(batch.Omitted)+" autres")
	}
	return strings.Join(parts, ", ")
}

func renderNotification(n Notification, now time.Time) string {
	a := n.Alert
	parts := []string{describeTarget(a.Target)}
	switch {
	case a.Target.Kind == AlertTargetVulnerabilities && a.Value != nil:
		parts = append(parts, frenchNumber(*a.Value)+" vulnérabilités")
	case a.Value != nil:
		value := "valeur " + frenchNumber(*a.Value)
		if a.Threshold != nil {
			value += ", seuil " + frenchNumber(*a.Threshold)
		}
		parts = append(parts, value)
	case a.Observed != "":
		parts = append(parts, "état "+a.Observed)
	}
	switch n.Event {
	case NotifyResolved:
		if a.ResolvedAt != nil {
			text := "résolue après " + frenchDuration(a.ResolvedAt.Sub(a.FiredAt))
			if reason, ok := resolutionText[a.Resolution]; ok {
				text += " (" + reason + ")"
			}
			parts = append(parts, text)
		}
	case NotifyReminder:
		parts = append(parts, "active depuis "+frenchDuration(now.Sub(a.FiredAt)))
	}
	return eventLabels[n.Event] + " · " + severityLabels[a.Severity] + " · " + a.RuleName + "\n" + strings.Join(parts, " · ")
}

func describeTarget(t AlertTarget) string {
	var text string
	switch t.Kind {
	case AlertTargetHost:
		text = "Hôte"
	case AlertTargetContainer:
		text = "Conteneur " + t.ContainerName
		if t.ContainerName == "" && t.ContainerID != nil {
			text = "Conteneur " + t.ContainerID.String()
		}
	case AlertTargetUnit:
		return "Unité " + t.Unit
	case AlertTargetVulnerabilities:
		return "Paquets de la machine"
	case AlertTargetCollector:
		label, ok := collectorLabels[t.Collector]
		if !ok {
			label = t.Collector
		}
		return "Collecte " + label
	}
	if t.Metric == "" {
		return text
	}
	text += " · " + t.Metric
	if len(t.Labels) > 0 {
		keys := slices.Sorted(maps.Keys(t.Labels))
		pairs := make([]string, len(keys))
		for i, key := range keys {
			pairs[i] = key + "=" + t.Labels[key]
		}
		text += " (" + strings.Join(pairs, ", ") + ")"
	}
	return text
}

// frenchNumber writes at most two decimals with a decimal comma.
func frenchNumber(value float64) string {
	return strings.Replace(strconv.FormatFloat(math.Round(value*100)/100, 'f', -1, 64), ".", ",", 1)
}

// frenchDuration writes a duration in days, hours and minutes.
func frenchDuration(d time.Duration) string {
	minutes := int(d / time.Minute)
	switch {
	case minutes < 1:
		return "moins d'une minute"
	case minutes < 60:
		return fmt.Sprintf("%d min", minutes)
	case minutes < 24*60:
		if minutes%60 == 0 {
			return fmt.Sprintf("%d h", minutes/60)
		}
		return fmt.Sprintf("%d h %02d min", minutes/60, minutes%60)
	}
	hours := minutes / 60
	if hours%24 == 0 {
		return fmt.Sprintf("%d j", hours/24)
	}
	return fmt.Sprintf("%d j %d h", hours/24, hours%24)
}

// webhookVersion versions the webhook body (API_CONTRACTS.md § 4.8).
const webhookVersion = 1

type webhookPayload struct {
	Version       int                   `json:"version"`
	Host          string                `json:"host"`
	SentAt        time.Time             `json:"sentAt"`
	Test          bool                  `json:"test"`
	Notifications []webhookNotification `json:"notifications"`
	Omitted       int                   `json:"omitted"`
}

type webhookNotification struct {
	Event NotificationEvent `json:"event"`
	Alert webhookAlert      `json:"alert"`
}

type webhookAlert struct {
	ID             string      `json:"id"`
	RuleID         string      `json:"ruleId"`
	RuleName       string      `json:"ruleName"`
	RuleType       string      `json:"ruleType"`
	Severity       string      `json:"severity"`
	State          string      `json:"state"`
	TargetKey      string      `json:"targetKey"`
	Target         AlertTarget `json:"target"`
	Value          *float64    `json:"value"`
	Threshold      *float64    `json:"threshold"`
	Observed       *string     `json:"observed"`
	FiredAt        time.Time   `json:"firedAt"`
	AcknowledgedAt *time.Time  `json:"acknowledgedAt"`
	ResolvedAt     *time.Time  `json:"resolvedAt"`
	Resolution     *string     `json:"resolution"`
}

func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// WebhookPayload is the JSON body of the generic webhook.
func WebhookPayload(host string, batch NotificationBatch, now time.Time) ([]byte, error) {
	payload := webhookPayload{
		Version: webhookVersion, Host: host, SentAt: now.UTC(), Test: batch.Test,
		Notifications: make([]webhookNotification, 0, len(batch.Notifications)), Omitted: batch.Omitted,
	}
	for _, n := range batch.Notifications {
		a := n.Alert
		payload.Notifications = append(payload.Notifications, webhookNotification{Event: n.Event, Alert: webhookAlert{
			ID: a.ID.String(), RuleID: a.RuleID.String(), RuleName: a.RuleName, RuleType: string(a.RuleType),
			Severity: string(a.Severity), State: string(a.State), TargetKey: a.TargetKey, Target: a.Target,
			Value: a.Value, Threshold: a.Threshold, Observed: optionalText(a.Observed), FiredAt: a.FiredAt.UTC(),
			AcknowledgedAt: a.AcknowledgedAt, ResolvedAt: a.ResolvedAt, Resolution: optionalText(string(a.Resolution)),
		}})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode webhook payload: %w", err)
	}
	return body, nil
}

// SignWebhook signs timestamp + "." + body with HMAC-SHA256; the receiver
// checks the signature and rejects old timestamps.
func SignWebhook(secret []byte, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// SortSilences orders silences active first, then newest first.
func SortSilences(silences []AlertSilence, now time.Time) {
	sort.SliceStable(silences, func(i, j int) bool {
		ai, aj := silences[i].ActiveAt(now), silences[j].ActiveAt(now)
		if ai != aj {
			return ai
		}
		return silences[i].StartsAt.After(silences[j].StartsAt)
	})
}
