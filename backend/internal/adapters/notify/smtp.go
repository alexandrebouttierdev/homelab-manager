package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

// sendMail sends the message over SMTP with STARTTLS or implicit TLS; a
// plain connection is never used, so the password never travels in clear.
func (n *Notifier) sendMail(ctx context.Context, settings domain.ChannelSettings, password, host string, message domain.NotificationMessage, now time.Time) error {
	address := net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port))
	tlsConfig := &tls.Config{ServerName: settings.Host, MinVersion: tls.VersionTLS12, RootCAs: n.rootCAs}
	dialer := &net.Dialer{}
	var conn net.Conn
	var err error
	if settings.Security == domain.SMTPTLS {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return classify(err)
	}
	// The SMTP client has no context: the deadline and the cancellation
	// close the connection.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, settings.Host)
	if err != nil {
		_ = conn.Close()
		return smtpError(ctx, err)
	}
	defer client.Close()
	if err := client.Hello(helloName(host)); err != nil {
		return smtpError(ctx, err)
	}
	if settings.Security == domain.SMTPStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return &domain.DeliveryError{Code: domain.DeliveryTLS}
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return smtpError(ctx, err)
		}
	}
	if settings.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", settings.Username, password, settings.Host)); err != nil {
			return smtpError(ctx, err)
		}
	}
	if err := client.Mail(settings.From); err != nil {
		return smtpError(ctx, err)
	}
	for _, recipient := range settings.To {
		if err := client.Rcpt(recipient); err != nil {
			return smtpError(ctx, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return smtpError(ctx, err)
	}
	content, err := mailContent(settings, message, now)
	if err != nil {
		return &domain.DeliveryError{Code: domain.DeliveryFailed}
	}
	if _, err := writer.Write(content); err != nil {
		return smtpError(ctx, err)
	}
	if err := writer.Close(); err != nil {
		return smtpError(ctx, err)
	}
	if err := client.Quit(); err != nil {
		return smtpError(ctx, err)
	}
	return nil
}

// helloName is the machine name announced to the server, or localhost
// when it is not a valid host name.
func helloName(host string) string {
	if host == "" || strings.ContainsAny(host, " \r\n") {
		return "localhost"
	}
	return host
}

func smtpError(ctx context.Context, err error) error {
	var reply *textproto.Error
	switch {
	case ctx.Err() != nil:
		return &domain.DeliveryError{Code: domain.DeliveryTimeout}
	case errors.As(err, &reply):
		return &domain.DeliveryError{Code: domain.DeliverySMTP, Status: reply.Code}
	}
	return classify(err)
}

// mailContent writes the headers and the quoted-printable body. The
// addresses are plain (validated by the domain), the subject is encoded.
func mailContent(settings domain.ChannelSettings, message domain.NotificationMessage, now time.Time) ([]byte, error) {
	var content bytes.Buffer
	headers := []string{
		"From: " + settings.From,
		"To: " + strings.Join(settings.To, ", "),
		"Subject: " + mime.QEncoding.Encode("utf-8", message.Title),
		"Date: " + now.Format(time.RFC1123Z),
		fmt.Sprintf("Message-ID: <%s@monoops>", uuid.New()),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: quoted-printable",
		"Auto-Submitted: auto-generated",
	}
	content.WriteString(strings.Join(headers, "\r\n") + "\r\n\r\n")
	body := quotedprintable.NewWriter(&content)
	if _, err := body.Write([]byte(strings.ReplaceAll(message.Body, "\n", "\r\n"))); err != nil {
		return nil, err
	}
	if err := body.Close(); err != nil {
		return nil, err
	}
	return content.Bytes(), nil
}
