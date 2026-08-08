// Package bookingmail reads and replies to email for a booking request,
// against a real mailbox (booking@decayolympia.org) rather than the site's
// own database. It's deliberately separate from the mail package: that one
// sends fire-and-forget notifications to a fixed inbox, this one reads a real
// IMAP mailbox and sends as it, to whoever a booking's Email field names.
//
// Like mail.Mailer, an unconfigured Config disables the feature rather than
// erroring: BookingHandler.Enabled reports false and admin pages hide the
// panel instead of showing a broken one.
package bookingmail

import (
	"net"
	"os"
	"strings"
	"time"
)

// Config is everything needed to read and send as the booking mailbox.
type Config struct {
	IMAPHost string
	IMAPPort string
	IMAPUser string
	IMAPPass string

	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string

	// Address is the mailbox's own address, used to tell a sent message from
	// a received one and to skip searching for it as a correspondent.
	Address string
	// FromName is the display name on outgoing replies.
	FromName string

	CacheTTL time.Duration
}

// FromEnv builds a Config from the environment:
//
//	BOOKING_IMAP_HOST   IMAP host (required to enable reading; unset disables everything)
//	BOOKING_IMAP_PORT   port, default 993 (implicit TLS; 143 uses STARTTLS)
//	BOOKING_IMAP_USER   mailbox username, default BOOKING_ADDRESS
//	BOOKING_IMAP_PASS   mailbox password
//	BOOKING_SMTP_HOST   SMTP host, default BOOKING_IMAP_HOST
//	BOOKING_SMTP_PORT   port, default 587 (STARTTLS; 465 uses implicit TLS)
//	BOOKING_SMTP_USER   default BOOKING_IMAP_USER
//	BOOKING_SMTP_PASS   default BOOKING_IMAP_PASS
//	BOOKING_ADDRESS     the mailbox's own address, default booking@decayolympia.org
//	BOOKING_FROM_NAME   display name on replies, default "DECAY Booking"
//
// A zero-value Config (IMAPHost == "") is valid and simply disabled.
func FromEnv() Config {
	c := Config{
		IMAPHost: strings.TrimSpace(os.Getenv("BOOKING_IMAP_HOST")),
		IMAPPort: envOr("BOOKING_IMAP_PORT", "993"),
		Address:  envOr("BOOKING_ADDRESS", "booking@decayolympia.org"),
		FromName: envOr("BOOKING_FROM_NAME", "DECAY Booking"),
		CacheTTL: 15 * time.Minute,
	}
	c.IMAPUser = envOr("BOOKING_IMAP_USER", c.Address)
	c.IMAPPass = os.Getenv("BOOKING_IMAP_PASS")

	c.SMTPHost = envOr("BOOKING_SMTP_HOST", c.IMAPHost)
	c.SMTPPort = envOr("BOOKING_SMTP_PORT", "587")
	c.SMTPUser = envOr("BOOKING_SMTP_USER", c.IMAPUser)
	c.SMTPPass = envOr("BOOKING_SMTP_PASS", c.IMAPPass)
	return c
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// Enabled reports whether reading the mailbox is configured.
func (c Config) Enabled() bool {
	return c.IMAPHost != "" && c.IMAPUser != "" && c.IMAPPass != ""
}

// CanSend reports whether sending is configured. It can be true even when
// Enabled is false — SMTP-only setups don't make much sense, but Config
// doesn't assume the two are always paired.
func (c Config) CanSend() bool {
	return c.SMTPHost != "" && c.SMTPUser != "" && c.SMTPPass != ""
}

func (c Config) imapAddr() string { return net.JoinHostPort(c.IMAPHost, c.IMAPPort) }
func (c Config) smtpAddr() string { return net.JoinHostPort(c.SMTPHost, c.SMTPPort) }

// imapImplicitTLS reports whether the IMAP port wants TLS from the first
// byte (993) rather than STARTTLS (143) — the same port-implies-mode rule
// mail.Mailer already uses for SMTP 465 vs 587.
func (c Config) imapImplicitTLS() bool { return c.IMAPPort != "143" }

func (c Config) smtpImplicitTLS() bool { return c.SMTPPort == "465" }
