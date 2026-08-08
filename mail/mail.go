// Package mail sends notification email over SMTP. It is deliberately thin:
// the site's record of a message is the row saved to the database, and mail
// is only a best-effort nudge on top of that. When SMTP isn't configured the
// Mailer is disabled and Notify is a no-op, so the site runs fine without a
// mail server.
package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// Mailer sends plain-text mail: notifications to a fixed inbox (the DECAY
// address) via Notify, and one-off messages to a named recipient via Send.
// The zero value is a disabled mailer whose Notify and Send do nothing.
type Mailer struct {
	addr        string // host:port; empty means disabled
	host        string // host alone, for auth and TLS
	auth        smtp.Auth
	from        string
	to          string
	implicitTLS bool // port 465: wrap the whole connection in TLS
}

// FromEnv builds a Mailer from the environment:
//
//	SMTP_HOST   mail server host (required; unset disables mail)
//	SMTP_PORT   port, default 587
//	SMTP_USER   username for auth (optional; unset sends unauthenticated)
//	SMTP_PASS   password for auth
//	MAIL_FROM   envelope/From address, default noreply@decay.events
//	CONTACT_TO  where contact notifications land, default info@decayolympia.org
//
// With no SMTP_HOST it returns a disabled Mailer, never nil.
func FromEnv() *Mailer {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if host == "" {
		return &Mailer{}
	}
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if port == "" {
		port = "587"
	}
	m := &Mailer{
		host:        host,
		addr:        net.JoinHostPort(host, port),
		from:        envOr("MAIL_FROM", "noreply@decay.events"),
		to:          envOr("CONTACT_TO", "info@decayolympia.org"),
		implicitTLS: port == "465",
	}
	if user := strings.TrimSpace(os.Getenv("SMTP_USER")); user != "" {
		m.auth = smtp.PlainAuth("", user, os.Getenv("SMTP_PASS"), host)
	}
	return m
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// Enabled reports whether mail is configured. When false, Notify is a no-op.
func (m *Mailer) Enabled() bool { return m != nil && m.addr != "" }

// To is the inbox notifications are sent to (for logging).
func (m *Mailer) To() string {
	if m == nil {
		return ""
	}
	return m.to
}

// Notify sends a plain-text message to the configured inbox. replyTo, when a
// valid-looking address, is set as the Reply-To header so a reply reaches the
// original sender. It returns nil when mail is disabled.
func (m *Mailer) Notify(subject, body, replyTo string) error {
	if !m.Enabled() {
		return nil
	}
	msg := buildMessage(m.from, m.to, replyTo, subject, body)
	return m.send(m.to, msg)
}

// Send delivers a plain-text message to one named recipient, rather than to
// DECAY's own inbox: an order confirmation goes to whoever bought something.
// It returns nil when mail is disabled, so an unconfigured site still
// completes a sale — the order is recorded either way, and the confirmation
// page carries the same details.
func (m *Mailer) Send(to, subject, body string) error {
	if !m.Enabled() {
		return nil
	}
	to = sanitizeHeader(to)
	if to == "" {
		return fmt.Errorf("mail: no recipient address")
	}
	return m.send(to, buildMessage(m.from, to, "", subject, body))
}

// send delivers the assembled message. Port 587 (and 25) use STARTTLS, which
// net/smtp's SendMail negotiates for us. Port 465 is implicit TLS — the
// connection is TLS from the first byte — which SendMail can't do, so that
// case is dialed directly. Hover, the mail host, offers both.
func (m *Mailer) send(to string, msg []byte) error {
	if !m.implicitTLS {
		return smtp.SendMail(m.addr, m.auth, m.from, []string{to}, msg)
	}

	conn, err := tls.Dial("tcp", m.addr, &tls.Config{ServerName: m.host})
	if err != nil {
		return err
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer c.Close()

	if m.auth != nil {
		if err := c.Auth(m.auth); err != nil {
			return err
		}
	}
	if err := c.Mail(m.from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// buildMessage assembles an RFC 5322 message. Header values are stripped of
// CR/LF so a hostile subject or reply-to address from the public form can't
// inject extra headers. The body is left as-is; net/smtp's DotWriter handles
// line-ending normalization and dot-stuffing when it's sent.
func buildMessage(from, to, replyTo, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", sanitizeHeader(from))
	fmt.Fprintf(&b, "To: %s\r\n", sanitizeHeader(to))
	if rt := sanitizeHeader(replyTo); rt != "" {
		fmt.Fprintf(&b, "Reply-To: %s\r\n", rt)
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", sanitizeHeader(subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// sanitizeHeader removes CR and LF so a value can't smuggle in new headers,
// and trims surrounding whitespace.
func sanitizeHeader(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return strings.TrimSpace(v)
}
