package notify

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"mime/quotedprintable"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"monoops/backend/internal/domain"
)

// fakeSMTP is an in-memory SMTP server for one connection at a time. It
// offers STARTTLS (starttls mode) or speaks TLS at once (tls mode).
type fakeSMTP struct {
	listener  net.Listener
	tlsConfig *tls.Config
	implicit  bool
	offerTLS  bool
	rejectTo  string

	mu         sync.Mutex
	auth       string
	from       string
	recipients []string
	data       string
	tlsUsed    bool
}

func certificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

func startSMTP(t *testing.T, implicit, offerTLS bool) (*fakeSMTP, *x509.CertPool) {
	t.Helper()
	cert, pool := certificate(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &fakeSMTP{listener: listener, tlsConfig: &tls.Config{Certificates: []tls.Certificate{cert}}, implicit: implicit, offerTLS: offerTLS}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.serve(conn)
		}
	}()
	return server, pool
}

func (s *fakeSMTP) port() int { return s.listener.Addr().(*net.TCPAddr).Port }

func (s *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	secure := false
	if s.implicit {
		conn = tls.Server(conn, s.tlsConfig)
		secure = true
	}
	text := textproto.NewConn(conn)
	_ = text.PrintfLine("220 localhost ESMTP")
	for {
		line, err := text.ReadLine()
		if err != nil {
			return
		}
		command, argument, _ := strings.Cut(line, " ")
		switch strings.ToUpper(command) {
		case "EHLO":
			if s.offerTLS && !secure {
				_ = text.PrintfLine("250-localhost")
				_ = text.PrintfLine("250-STARTTLS")
			} else {
				_ = text.PrintfLine("250-localhost")
			}
			_ = text.PrintfLine("250 AUTH PLAIN")
		case "STARTTLS":
			_ = text.PrintfLine("220 ready")
			conn = tls.Server(conn, s.tlsConfig)
			text = textproto.NewConn(conn)
			secure = true
		case "AUTH":
			s.mu.Lock()
			s.auth = argument
			s.tlsUsed = secure
			s.mu.Unlock()
			_ = text.PrintfLine("235 ok")
		case "MAIL":
			s.mu.Lock()
			s.from = argument
			s.tlsUsed = secure
			s.mu.Unlock()
			_ = text.PrintfLine("250 ok")
		case "RCPT":
			if s.rejectTo != "" && strings.Contains(argument, s.rejectTo) {
				_ = text.PrintfLine("550 no such user")
				continue
			}
			s.mu.Lock()
			s.recipients = append(s.recipients, argument)
			s.mu.Unlock()
			_ = text.PrintfLine("250 ok")
		case "DATA":
			_ = text.PrintfLine("354 go ahead")
			data, err := text.ReadDotBytes()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.data = string(data)
			s.mu.Unlock()
			_ = text.PrintfLine("250 queued")
		case "QUIT":
			_ = text.PrintfLine("221 bye")
			return
		default:
			_ = text.PrintfLine("502 unknown")
		}
	}
}

func smtpChannel(port int, security domain.SMTPSecurity, username string) domain.NotificationChannel {
	return channel(domain.ChannelSMTP, domain.ChannelSettings{
		Host: "127.0.0.1", Port: port, Security: security, Username: username,
		From: "bot@example.org", To: []string{"me@example.org", "you@example.org"},
	})
}

func TestSMTPSendsOverSTARTTLSWithAuthentication(t *testing.T) {
	server, pool := startSMTP(t, false, true)
	notifier := New(WithRootCAs(pool), WithClock(func() time.Time { return sendAt }))
	err := notifier.Send(context.Background(), smtpChannel(server.port(), domain.SMTPStartTLS, "bot"),
		domain.ChannelSecrets{domain.SecretPassword: "correct horse"}, "nas", batch())
	if err != nil {
		t.Fatalf("Send() = %v", err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	credentials, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(server.auth, "PLAIN "))
	if !server.tlsUsed || string(credentials) != "\x00bot\x00correct horse" {
		t.Fatalf("authentication %q over TLS %v", credentials, server.tlsUsed)
	}
	if server.from != "FROM:<bot@example.org>" || len(server.recipients) != 2 {
		t.Fatalf("envelope = %q %q", server.from, server.recipients)
	}
	// ReadDotBytes turns CRLF into LF.
	headers, body, _ := strings.Cut(server.data, "\n\n")
	for _, want := range []string{"From: bot@example.org", "To: me@example.org, you@example.org", "Subject: =?utf-8?q?nas_=C2=B7_Critique", "Content-Type: text/plain; charset=utf-8"} {
		if !strings.Contains(headers, want) {
			t.Errorf("headers miss %q:\n%s", want, headers)
		}
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if err != nil || !strings.Contains(string(decoded), "Déclenchée · Critique · Unité systemd en échec") {
		t.Fatalf("body = %q (%v)", decoded, err)
	}
}

func TestSMTPSendsOverImplicitTLSWithoutAuthentication(t *testing.T) {
	server, pool := startSMTP(t, true, false)
	if err := New(WithRootCAs(pool)).Send(context.Background(), smtpChannel(server.port(), domain.SMTPTLS, ""), nil, "nas", batch()); err != nil {
		t.Fatalf("Send() = %v", err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if !server.tlsUsed || server.auth != "" || server.data == "" {
		t.Fatalf("TLS %v, auth %q, data %d bytes", server.tlsUsed, server.auth, len(server.data))
	}
}

func TestSMTPRefusesAServerWithoutSTARTTLS(t *testing.T) {
	server, pool := startSMTP(t, false, false)
	err := New(WithRootCAs(pool)).Send(context.Background(), smtpChannel(server.port(), domain.SMTPStartTLS, "bot"),
		domain.ChannelSecrets{domain.SecretPassword: "correct horse"}, "nas", batch())
	if got := deliveryError(t, err); got.Code != domain.DeliveryTLS {
		t.Fatalf("Send() = %+v, want tls_failed", got)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.auth != "" {
		t.Fatal("the password was sent without TLS")
	}
}

func TestSMTPReportsTheReplyCodeOfARefusal(t *testing.T) {
	server, pool := startSMTP(t, true, false)
	server.rejectTo = "you@"
	err := New(WithRootCAs(pool)).Send(context.Background(), smtpChannel(server.port(), domain.SMTPTLS, ""), nil, "nas", batch())
	if got := deliveryError(t, err); got.Code != domain.DeliverySMTP || got.Status != 550 {
		t.Fatalf("Send() = %+v, want smtp_rejected 550", got)
	}
}

func TestSMTPWithAnUntrustedCertificateFails(t *testing.T) {
	server, _ := startSMTP(t, true, false)
	err := New().Send(context.Background(), smtpChannel(server.port(), domain.SMTPTLS, ""), nil, "nas", batch())
	if got := deliveryError(t, err); got.Code != domain.DeliveryTLS {
		t.Fatalf("Send() = %+v, want tls_failed", got)
	}
}
