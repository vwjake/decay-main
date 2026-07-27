package mail

import (
	"strings"
	"testing"
)

func TestFromEnvDisabledWithoutHost(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	m := FromEnv()
	if m.Enabled() {
		t.Fatal("mailer should be disabled when SMTP_HOST is unset")
	}
	// Notify on a disabled mailer is a no-op, not an error.
	if err := m.Notify("subject", "body", "someone@x.com"); err != nil {
		t.Fatalf("disabled Notify returned %v, want nil", err)
	}
}

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("MAIL_FROM", "")
	t.Setenv("CONTACT_TO", "")
	m := FromEnv()
	if !m.Enabled() {
		t.Fatal("mailer should be enabled when SMTP_HOST is set")
	}
	if m.addr != "smtp.example.com:587" {
		t.Errorf("addr = %q, want default port 587", m.addr)
	}
	if m.from != "noreply@decay.events" {
		t.Errorf("from = %q, want default", m.from)
	}
	if m.To() != "info@decayolympia.org" {
		t.Errorf("to = %q, want default", m.To())
	}
}

func TestBuildMessageHeaders(t *testing.T) {
	msg := string(buildMessage("noreply@decay.events", "info@decay.events", "ada@x.com", "Contact form: Hi", "hello there"))

	for _, want := range []string{
		"From: noreply@decay.events\r\n",
		"To: info@decay.events\r\n",
		"Reply-To: ada@x.com\r\n",
		"Subject: Contact form: Hi\r\n",
		"Content-Type: text/plain; charset=\"utf-8\"\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing header %q\n---\n%s", want, msg)
		}
	}
	// Headers end, then the body.
	if !strings.HasSuffix(msg, "\r\n\r\nhello there") {
		t.Errorf("body not separated from headers by a blank line:\n%s", msg)
	}
}

func TestBuildMessageOmitsEmptyReplyTo(t *testing.T) {
	msg := string(buildMessage("from@x.com", "to@x.com", "", "Subj", "body"))
	if strings.Contains(msg, "Reply-To:") {
		t.Errorf("empty reply-to should be omitted:\n%s", msg)
	}
}

func TestBuildMessageStripsHeaderInjection(t *testing.T) {
	// A subject and reply-to carrying CRLF must not smuggle in new headers.
	evilSubject := "Hi\r\nBcc: attacker@evil.com"
	evilReplyTo := "ada@x.com\r\nBcc: attacker@evil.com"
	msg := string(buildMessage("from@x.com", "to@x.com", evilReplyTo, evilSubject, "body"))
	// The injected text must not become its own header line — stripping CR/LF
	// collapses it onto the Subject/Reply-To value, where it's inert.
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Errorf("header injection produced a Bcc line:\n%s", msg)
		}
	}
	if strings.Contains(msg, "\r\nBcc:") {
		t.Errorf("injected header survived:\n%s", msg)
	}
}
