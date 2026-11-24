// Package notify sends the alert notifications: the only outbound network
// traffic of the Manager (SPEC_ADMINISTRATION.md § 6.3). Every send is
// bounded in time and size, follows no redirect and verifies TLS; a failure
// is reported as a domain.DeliveryError that holds no address nor secret.
package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

const (
	// sendTimeout bounds one send, connection included.
	sendTimeout = 10 * time.Second
	// maxResponseBytes bounds what is read of an answer, then discarded.
	maxResponseBytes = 64 << 10
	userAgent        = "Homelab-Manager"
	telegramAPI      = "https://api.telegram.org"
	// Message bounds of the services, in characters.
	telegramMaxRunes = 4096
	discordMaxRunes  = 2000
	// ntfy turns a message above 4 096 bytes into an attachment.
	ntfyMaxRunes   = 1800
	gotifyMaxRunes = 8000
)

var _ ports.Notifier = (*Notifier)(nil)

// Notifier sends a batch to a channel of any type.
type Notifier struct {
	client      *http.Client
	rootCAs     *x509.CertPool
	telegramAPI string
	now         func() time.Time
}

// Option changes a Notifier; used by the tests to trust a local server.
type Option func(*Notifier)

// WithRootCAs trusts these authorities instead of the system ones.
func WithRootCAs(pool *x509.CertPool) Option { return func(n *Notifier) { n.rootCAs = pool } }

// WithTelegramAPI replaces https://api.telegram.org.
func WithTelegramAPI(base string) Option { return func(n *Notifier) { n.telegramAPI = base } }

// WithClock replaces time.Now.
func WithClock(now func() time.Time) Option { return func(n *Notifier) { n.now = now } }

func New(options ...Option) *Notifier {
	n := &Notifier{telegramAPI: telegramAPI, now: time.Now}
	for _, option := range options {
		option(n)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: n.rootCAs}
	transport.MaxIdleConns, transport.IdleConnTimeout = 4, 30*time.Second
	n.client = &http.Client{
		Transport: transport,
		Timeout:   sendTimeout,
		// A redirect could carry a token to another host: refused.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return n
}

// CloseIdleConnections releases the connections kept for the next send.
func (n *Notifier) CloseIdleConnections() { n.client.CloseIdleConnections() }

func (n *Notifier) Send(ctx context.Context, channel domain.NotificationChannel, secrets domain.ChannelSecrets, host string, batch domain.NotificationBatch) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	now := n.now().UTC()
	switch channel.Type {
	case domain.ChannelTelegram:
		message := render(host, batch, now, telegramMaxRunes)
		return n.postJSON(ctx, n.telegramAPI+"/bot"+secrets[domain.SecretBotToken]+"/sendMessage", nil, map[string]any{
			"chat_id": channel.Settings.ChatID, "text": message.Title + "\n\n" + message.Body, "disable_web_page_preview": true,
		})
	case domain.ChannelDiscord:
		message := render(host, batch, now, discordMaxRunes-4)
		return n.postJSON(ctx, secrets[domain.SecretWebhookURL], nil, map[string]any{
			"content": "**" + message.Title + "**\n" + message.Body,
			// No @everyone nor role mention from an alert text.
			"allowed_mentions": map[string]any{"parse": []string{}},
		})
	case domain.ChannelGotify:
		message := render(host, batch, now, gotifyMaxRunes)
		endpoint, err := joinURL(channel.Settings.ServerURL, "message")
		if err != nil {
			return err
		}
		return n.postJSON(ctx, endpoint, map[string]string{"X-Gotify-Key": secrets[domain.SecretAppToken]}, map[string]any{
			"title": message.Title, "message": message.Body, "priority": gotifyPriority[batch.HighestSeverity()],
		})
	case domain.ChannelNtfy:
		message := render(host, batch, now, ntfyMaxRunes)
		headers := map[string]string{}
		if token := secrets[domain.SecretAccessToken]; token != "" {
			headers["Authorization"] = "Bearer " + token
		}
		// JSON publishing on the server root: UTF-8 titles need no header
		// encoding.
		return n.postJSON(ctx, channel.Settings.ServerURL, headers, map[string]any{
			"topic": channel.Settings.Topic, "title": message.Title, "message": message.Body,
			"priority": ntfyPriority[batch.HighestSeverity()],
		})
	case domain.ChannelWebhook:
		body, err := domain.WebhookPayload(host, batch, now)
		if err != nil {
			return &domain.DeliveryError{Code: domain.DeliveryFailed}
		}
		headers := map[string]string{}
		if secret := secrets[domain.SecretSigningSecret]; secret != "" {
			timestamp := now.Unix()
			headers["X-Homelab-Timestamp"] = strconv.FormatInt(timestamp, 10)
			headers["X-Homelab-Signature"] = domain.SignWebhook([]byte(secret), timestamp, body)
		}
		return n.post(ctx, channel.Settings.URL, headers, body)
	case domain.ChannelSMTP:
		return n.sendMail(ctx, channel.Settings, secrets[domain.SecretPassword], host, render(host, batch, now, 0), now)
	}
	return &domain.DeliveryError{Code: domain.DeliveryFailed}
}

var (
	gotifyPriority = map[domain.AlertSeverity]int{domain.AlertInfo: 2, domain.AlertWarning: 5, domain.AlertCritical: 8}
	ntfyPriority   = map[domain.AlertSeverity]int{domain.AlertInfo: 3, domain.AlertWarning: 4, domain.AlertCritical: 5}
)

// render bounds the whole text, title included, to maxRunes (0: none).
func render(host string, batch domain.NotificationBatch, now time.Time, maxRunes int) domain.NotificationMessage {
	if maxRunes == 0 {
		return domain.RenderNotification(host, batch, now, 0)
	}
	title := domain.RenderNotification(host, batch, now, 0).Title
	budget := max(maxRunes-utf8.RuneCountInString(title)-2, 100)
	return domain.RenderNotification(host, batch, now, budget)
}

func joinURL(base, element string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", &domain.DeliveryError{Code: domain.DeliveryFailed}
	}
	parsed.Path = path.Join("/", parsed.Path, element)
	return parsed.String(), nil
}

func (n *Notifier) postJSON(ctx context.Context, endpoint string, headers map[string]string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &domain.DeliveryError{Code: domain.DeliveryFailed}
	}
	return n.post(ctx, endpoint, headers, body)
}

func (n *Notifier) post(ctx context.Context, endpoint string, headers map[string]string, body []byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return &domain.DeliveryError{Code: domain.DeliveryFailed}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", userAgent)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := n.client.Do(request)
	if err != nil {
		return classify(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
	switch {
	case response.StatusCode >= 300 && response.StatusCode < 400:
		return &domain.DeliveryError{Code: domain.DeliveryRedirect, Status: response.StatusCode}
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return &domain.DeliveryError{Code: domain.DeliveryHTTPStatus, Status: response.StatusCode}
	}
	return nil
}

// classify turns a transport error into a code: the error itself holds the
// URL, which may carry a token (Telegram, Discord), and is never returned.
func classify(err error) *domain.DeliveryError {
	var netErr net.Error
	var certificate *tls.CertificateVerificationError
	var record tls.RecordHeaderError
	var alert tls.AlertError
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return &domain.DeliveryError{Code: domain.DeliveryTimeout}
	case errors.As(err, &certificate) || errors.As(err, &record) || errors.As(err, &alert) ||
		errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || strings.Contains(err.Error(), "tls:"):
		return &domain.DeliveryError{Code: domain.DeliveryTLS}
	}
	return &domain.DeliveryError{Code: domain.DeliveryConnection}
}
