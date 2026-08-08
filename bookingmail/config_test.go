package bookingmail

import "testing"

func TestFromEnvDisabledWithoutHost(t *testing.T) {
	t.Setenv("BOOKING_IMAP_HOST", "")
	c := FromEnv()
	if c.Enabled() {
		t.Fatal("should be disabled when BOOKING_IMAP_HOST is unset")
	}
}

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("BOOKING_IMAP_HOST", "mail.decayolympia.org")
	t.Setenv("BOOKING_IMAP_PASS", "hunter2")
	t.Setenv("BOOKING_IMAP_PORT", "")
	t.Setenv("BOOKING_IMAP_USER", "")
	t.Setenv("BOOKING_SMTP_HOST", "")
	t.Setenv("BOOKING_ADDRESS", "")
	t.Setenv("BOOKING_FROM_NAME", "")

	c := FromEnv()
	if !c.Enabled() {
		t.Fatal("should be enabled once host and password are set")
	}
	if c.Address != "booking@decayolympia.org" {
		t.Errorf("Address = %q", c.Address)
	}
	if c.IMAPUser != c.Address {
		t.Errorf("IMAPUser should default to Address, got %q", c.IMAPUser)
	}
	if c.IMAPPort != "993" {
		t.Errorf("IMAPPort = %q, want default 993", c.IMAPPort)
	}
	if !c.imapImplicitTLS() {
		t.Error("port 993 should use implicit TLS")
	}
	// SMTP defaults to the IMAP host/user/pass, since it's the same mailbox.
	if c.SMTPHost != c.IMAPHost {
		t.Errorf("SMTPHost = %q, want to default to IMAPHost", c.SMTPHost)
	}
	if c.SMTPUser != c.IMAPUser || c.SMTPPass != c.IMAPPass {
		t.Error("SMTP user/pass should default to IMAP user/pass")
	}
	if c.SMTPPort != "587" {
		t.Errorf("SMTPPort = %q, want default 587", c.SMTPPort)
	}
	if c.smtpImplicitTLS() {
		t.Error("port 587 should not use implicit TLS")
	}
	if !c.CanSend() {
		t.Error("CanSend should be true once SMTP defaults resolve from IMAP settings")
	}
}

func TestFromEnvIMAPSTARTTLSOn143(t *testing.T) {
	t.Setenv("BOOKING_IMAP_HOST", "mail.decayolympia.org")
	t.Setenv("BOOKING_IMAP_PASS", "x")
	t.Setenv("BOOKING_IMAP_PORT", "143")
	c := FromEnv()
	if c.imapImplicitTLS() {
		t.Error("port 143 should use STARTTLS, not implicit TLS")
	}
}

func TestFromEnvSMTPImplicitTLSOn465(t *testing.T) {
	t.Setenv("BOOKING_IMAP_HOST", "mail.decayolympia.org")
	t.Setenv("BOOKING_IMAP_PASS", "x")
	t.Setenv("BOOKING_SMTP_PORT", "465")
	c := FromEnv()
	if !c.smtpImplicitTLS() {
		t.Error("port 465 should use implicit TLS")
	}
}
