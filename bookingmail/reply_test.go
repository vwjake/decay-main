package bookingmail

import (
	"io"
	"mime"
	"mime/quotedprintable"
	"strings"
	"testing"
)

// decodeQuotedPrintableBody splits headers from body on a raw RFC822 message
// and decodes the quoted-printable body, for asserting on what a recipient
// would actually see.
func decodeQuotedPrintableBody(t *testing.T, msg string) string {
	t.Helper()
	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("message has no header/body separator:\n%s", msg)
	}
	b, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(parts[1])))
	if err != nil {
		t.Fatalf("decoding quoted-printable body: %v", err)
	}
	return string(b)
}

func testHandler() *Handler {
	return New(Config{
		IMAPHost: "mail.decayolympia.org",
		IMAPUser: "booking@decayolympia.org",
		IMAPPass: "hunter2",
		SMTPHost: "mail.decayolympia.org",
		SMTPUser: "booking@decayolympia.org",
		SMTPPass: "hunter2",
		Address:  "booking@decayolympia.org",
		FromName: "DECAY Booking",
	})
}

func TestBuildOutgoingHeaders(t *testing.T) {
	h := testHandler()
	out, err := h.buildOutgoing(ReplyInput{
		To:         "hayesnoblemusic@gmail.com",
		ToName:     "Hayes Noble",
		Subject:    "Re: April 3rd — tour stop",
		Body:       "Yes, April 3rd works.\nLoad-in at 6pm.",
		SenderName: "Jake Fery",
		InReplyTo:  "msg1@example.com",
	}, "hayesnoblemusic@gmail.com")
	if err != nil {
		t.Fatal(err)
	}
	msg := string(out.raw)

	for _, want := range []string{
		"From: DECAY Booking <booking@decayolympia.org>\r\n",
		"To: Hayes Noble <hayesnoblemusic@gmail.com>\r\n",
		"In-Reply-To: <msg1@example.com>\r\n",
		"References: <msg1@example.com>\r\n",
		"Content-Transfer-Encoding: quoted-printable\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n---\n%s", want, msg)
		}
	}

	if !strings.Contains(msg, "Message-ID: <"+out.messageID+">\r\n") {
		t.Errorf("Message-ID header doesn't match returned messageID %q:\n%s", out.messageID, msg)
	}

	// The subject carries an em dash, so it must be RFC 2047 encoded, and
	// decoding it back must round-trip to the original text.
	subjectLine := headerLine(msg, "Subject")
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(subjectLine)
	if err != nil {
		t.Fatalf("decoding subject: %v", err)
	}
	if decoded != "Re: April 3rd — tour stop" {
		t.Errorf("decoded subject = %q", decoded)
	}
}

func TestBuildOutgoingSignsBody(t *testing.T) {
	h := testHandler()
	out, err := h.buildOutgoing(ReplyInput{
		To: "a@b.com", Subject: "s", Body: "Hello there.", SenderName: "Jake Fery",
	}, "a@b.com")
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeQuotedPrintableBody(t, string(out.raw))
	if !strings.Contains(decoded, "Hello there.") {
		t.Errorf("body missing original text:\n%s", decoded)
	}
	if !strings.Contains(decoded, "— Jake Fery\r\nDECAY Booking") {
		t.Errorf("body missing signature:\n%s", decoded)
	}
}

func TestBuildOutgoingSignsWithHouseNameWhenSenderUnknown(t *testing.T) {
	h := testHandler()
	out, err := h.buildOutgoing(ReplyInput{To: "a@b.com", Subject: "s", Body: "Hi."}, "a@b.com")
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeQuotedPrintableBody(t, string(out.raw))
	if !strings.Contains(decoded, "— DECAY Booking") {
		t.Errorf("body missing house signature:\n%s", decoded)
	}
}

func TestBuildOutgoingRejectsHeaderInjection(t *testing.T) {
	h := testHandler()
	out, err := h.buildOutgoing(ReplyInput{
		To: "a@b.com", Subject: "hi\r\nBcc: sneak@evil.com", Body: "x",
	}, "a@b.com")
	if err != nil {
		t.Fatal(err)
	}
	msg := string(out.raw)
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Errorf("CRLF in subject produced a Bcc header:\n%s", msg)
		}
	}
}

func TestSendRefusesInvalidInputWithoutTouchingNetwork(t *testing.T) {
	h := testHandler()
	cases := []struct {
		name string
		in   ReplyInput
	}{
		{"bad recipient", ReplyInput{To: "not-an-email", Subject: "s", Body: "b"}},
		{"empty subject", ReplyInput{To: "a@b.com", Subject: "  ", Body: "b"}},
		{"empty body", ReplyInput{To: "a@b.com", Subject: "s", Body: "   "}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := h.Send(c.in)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestSendRefusesWhenNotConfigured(t *testing.T) {
	h := New(Config{})
	_, err := h.Send(ReplyInput{To: "a@b.com", Subject: "s", Body: "b"})
	if err == nil {
		t.Fatal("expected an error when SMTP isn't configured")
	}
}

// headerLine extracts one unfolded header line's value from a raw message.
func headerLine(msg, name string) string {
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(line, name+": ") {
			return strings.TrimPrefix(line, name+": ")
		}
	}
	return ""
}
