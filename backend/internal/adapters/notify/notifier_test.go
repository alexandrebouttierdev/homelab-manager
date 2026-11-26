package notify

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

var sendAt = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

const telegramToken = "123456789:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type received struct {
	method, path string
	header       http.Header
	body         []byte
}

// recorder is an httptest handler that keeps the requests it receives.
type recorder struct {
	mu       sync.Mutex
	requests []received
	status   int
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	r.mu.Lock()
	r.requests = append(r.requests, received{request.Method, request.URL.Path, request.Header.Clone(), body})
	status := r.status
	r.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (r *recorder) only(t *testing.T) received {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) != 1 {
		t.Fatalf("received %d requests, want 1", len(r.requests))
	}
	return r.requests[0]
}

func tlsServer(t *testing.T, handler http.Handler) (*httptest.Server, *Notifier) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return server, New(WithRootCAs(pool), WithTelegramAPI(server.URL), WithClock(func() time.Time { return sendAt }))
}

func batch() domain.NotificationBatch {
	alert := domain.Alert{
		ID: uuid.New(), RuleID: uuid.New(), RuleName: "Unité systemd en échec", RuleType: domain.AlertRuleUnitFailed,
		Severity: domain.AlertCritical, TargetKey: "unit:nginx.service",
		Target: domain.AlertTarget{Kind: domain.AlertTargetUnit, Unit: "nginx.service"},
		State:  domain.AlertFiring, Observed: "failed (exit-code)", FiredAt: sendAt,
	}
	return domain.NotificationBatch{Notifications: []domain.Notification{{Event: domain.NotifyFiring, Alert: alert}}}
}

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return value
}

func channel(kind domain.ChannelType, settings domain.ChannelSettings) domain.NotificationChannel {
	return domain.NotificationChannel{ID: uuid.New(), Name: "c", Type: kind, Enabled: true, MinSeverity: domain.AlertInfo, Settings: settings}
}

func TestTelegramSendsTheMessageToTheChat(t *testing.T) {
	handler := &recorder{}
	_, notifier := tlsServer(t, handler)
	err := notifier.Send(context.Background(), channel(domain.ChannelTelegram, domain.ChannelSettings{ChatID: "-100123"}),
		domain.ChannelSecrets{domain.SecretBotToken: telegramToken}, "nas", batch())
	if err != nil {
		t.Fatalf("Send() = %v", err)
	}
	request := handler.only(t)
	body := decode(t, request.body)
	if request.path != "/bot"+telegramToken+"/sendMessage" || body["chat_id"] != "-100123" ||
		!strings.HasPrefix(body["text"].(string), "nas · Critique · Unité systemd en échec\n\nDéclenchée") {
		t.Fatalf("request = %s %s", request.path, request.body)
	}
	if request.header.Get("User-Agent") != userAgent {
		t.Fatalf("User-Agent = %q", request.header.Get("User-Agent"))
	}
}

func TestDiscordDisablesMentions(t *testing.T) {
	handler := &recorder{status: http.StatusNoContent}
	server, notifier := tlsServer(t, handler)
	err := notifier.Send(context.Background(), channel(domain.ChannelDiscord, domain.ChannelSettings{}),
		domain.ChannelSecrets{domain.SecretWebhookURL: server.URL + "/api/webhooks/1/token"}, "nas", batch())
	if err != nil {
		t.Fatalf("Send() = %v", err)
	}
	body := decode(t, handler.only(t).body)
	mentions, _ := body["allowed_mentions"].(map[string]any)
	if parse, ok := mentions["parse"].([]any); !ok || len(parse) != 0 || !strings.HasPrefix(body["content"].(string), "**nas · Critique") {
		t.Fatalf("body = %v", body)
	}
}

func TestGotifySendsThePriorityOfTheMostSevereAlert(t *testing.T) {
	handler := &recorder{}
	server, notifier := tlsServer(t, handler)
	err := notifier.Send(context.Background(), channel(domain.ChannelGotify, domain.ChannelSettings{ServerURL: server.URL + "/gotify/"}),
		domain.ChannelSecrets{domain.SecretAppToken: "app-token"}, "nas", batch())
	if err != nil {
		t.Fatalf("Send() = %v", err)
	}
	request := handler.only(t)
	body := decode(t, request.body)
	if request.path != "/gotify/message" || request.header.Get("X-Gotify-Key") != "app-token" || body["priority"] != float64(8) || body["title"] == "" {
		t.Fatalf("request = %s %v %s", request.path, request.header, request.body)
	}
}

func TestNtfyPublishesJSONWithAnOptionalToken(t *testing.T) {
	for _, token := range []string{"", "tk_secret"} {
		handler := &recorder{}
		server, notifier := tlsServer(t, handler)
		secrets := domain.ChannelSecrets{}
		if token != "" {
			secrets[domain.SecretAccessToken] = token
		}
		if err := notifier.Send(context.Background(), channel(domain.ChannelNtfy, domain.ChannelSettings{ServerURL: server.URL, Topic: "monoops"}), secrets, "nas", batch()); err != nil {
			t.Fatalf("Send() = %v", err)
		}
		request := handler.only(t)
		body := decode(t, request.body)
		wantAuth := ""
		if token != "" {
			wantAuth = "Bearer " + token
		}
		if body["topic"] != "monoops" || body["priority"] != float64(5) || request.header.Get("Authorization") != wantAuth {
			t.Fatalf("request = %v %s", request.header, request.body)
		}
	}
}

func TestWebhookIsSignedWhenItHasASecret(t *testing.T) {
	handler := &recorder{}
	server, notifier := tlsServer(t, handler)
	secret := strings.Repeat("s", 32)
	err := notifier.Send(context.Background(), channel(domain.ChannelWebhook, domain.ChannelSettings{URL: server.URL + "/hook"}),
		domain.ChannelSecrets{domain.SecretSigningSecret: secret}, "nas", batch())
	if err != nil {
		t.Fatalf("Send() = %v", err)
	}
	request := handler.only(t)
	timestamp := request.header.Get("X-MonoOps-Timestamp")
	if timestamp != strconv.FormatInt(sendAt.Unix(), 10) || request.header.Get("X-MonoOps-Signature") != domain.SignWebhook([]byte(secret), sendAt.Unix(), request.body) {
		t.Fatalf("headers = %v", request.header)
	}
	if body := decode(t, request.body); body["version"] != float64(1) || body["host"] != "nas" {
		t.Fatalf("body = %s", request.body)
	}
	unsigned := &recorder{}
	server, notifier = tlsServer(t, unsigned)
	if err := notifier.Send(context.Background(), channel(domain.ChannelWebhook, domain.ChannelSettings{URL: server.URL}), nil, "nas", batch()); err != nil {
		t.Fatal(err)
	}
	if unsigned.only(t).header.Get("X-MonoOps-Signature") != "" {
		t.Fatal("a webhook without secret is signed")
	}
}

func deliveryError(t *testing.T, err error) *domain.DeliveryError {
	t.Helper()
	var delivery *domain.DeliveryError
	if !errors.As(err, &delivery) {
		t.Fatalf("error = %v, want a DeliveryError", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), telegramToken) {
		t.Fatalf("error %q leaks an address or a secret", err)
	}
	return delivery
}

func TestFailuresAreReportedWithoutAddressNorSecret(t *testing.T) {
	telegram := channel(domain.ChannelTelegram, domain.ChannelSettings{ChatID: "1"})
	secrets := domain.ChannelSecrets{domain.SecretBotToken: telegramToken}

	_, notifier := tlsServer(t, &recorder{status: http.StatusUnauthorized})
	if got := deliveryError(t, notifier.Send(context.Background(), telegram, secrets, "nas", batch())); got.Code != domain.DeliveryHTTPStatus || got.Status != 401 {
		t.Errorf("401 = %+v", got)
	}

	_, notifier = tlsServer(t, http.RedirectHandler("https://elsewhere.example/", http.StatusFound))
	if got := deliveryError(t, notifier.Send(context.Background(), telegram, secrets, "nas", batch())); got.Code != domain.DeliveryRedirect || got.Status != 302 {
		t.Errorf("redirect = %+v", got)
	}

	untrusted := httptest.NewTLSServer(&recorder{})
	t.Cleanup(untrusted.Close)
	if got := deliveryError(t, New(WithTelegramAPI(untrusted.URL)).Send(context.Background(), telegram, secrets, "nas", batch())); got.Code != domain.DeliveryTLS {
		t.Errorf("untrusted certificate = %+v", got)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "https://" + listener.Addr().String()
	_ = listener.Close()
	if got := deliveryError(t, New(WithTelegramAPI(closed)).Send(context.Background(), telegram, secrets, "nas", batch())); got.Code != domain.DeliveryConnection {
		t.Errorf("connection refused = %+v", got)
	}

	release := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	_, notifier = tlsServer(t, slow)
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if got := deliveryError(t, notifier.Send(ctx, telegram, secrets, "nas", batch())); got.Code != domain.DeliveryTimeout {
		t.Errorf("timeout = %+v", got)
	}
}

func TestRenderBoundsTheWholeText(t *testing.T) {
	var long domain.NotificationBatch
	for range 100 {
		long.Notifications = append(long.Notifications, batch().Notifications...)
	}
	message := render("nas", long, sendAt, discordMaxRunes)
	if total := len([]rune(message.Title)) + 2 + len([]rune(message.Body)); total > discordMaxRunes {
		t.Fatalf("text has %d characters, want at most %d", total, discordMaxRunes)
	}
}
