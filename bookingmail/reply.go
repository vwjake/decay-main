package bookingmail

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/smtp"
	"strings"
	"time"
)

// ReplyInput is what an admin composed.
type ReplyInput struct {
	To         string
	ToName     string
	Subject    string
	Body       string
	SenderName string // the admin's name, appended as a signature
	InReplyTo  string // Message-ID being replied to, for threading
}

// ReplyResult reports what happened after Send.
type ReplyResult struct {
	MessageID string
	// Warning is set when the mail genuinely sent but filing a copy in Sent
	// failed — the reply went out, but it may not show up in the thread
	// until the recipient replies.
	Warning string
}

// Send delivers a reply via SMTP as the booking mailbox, then files a copy in
// the Sent folder so the conversation stays visible on the booking page. The
// mail having gone out is treated as success even if filing the copy fails;
// nothing below that point should make an admin think it wasn't sent.
func (h *Handler) Send(in ReplyInput) (ReplyResult, error) {
	to := strings.ToLower(strings.TrimSpace(in.To))
	if !looksLikeEmail(to) {
		return ReplyResult{}, fmt.Errorf("that recipient address is not valid")
	}
	if strings.TrimSpace(in.Subject) == "" {
		return ReplyResult{}, fmt.Errorf("the subject line is empty")
	}
	if strings.TrimSpace(in.Body) == "" {
		return ReplyResult{}, fmt.Errorf("the message body is empty")
	}
	if !h.cfg.CanSend() {
		return ReplyResult{}, fmt.Errorf("outgoing mail is not configured on this server")
	}

	built, err := h.buildOutgoing(in, to)
	if err != nil {
		return ReplyResult{}, err
	}

	if err := h.smtpSend(to, built.raw); err != nil {
		return ReplyResult{}, err
	}

	res := ReplyResult{MessageID: built.messageID}
	if err := h.fileInSent(built.raw); err != nil {
		res.Warning = "The reply was sent, but a copy could not be filed in the Sent folder, " +
			"so it may not appear in the history below until the recipient replies."
	}

	h.Forget(to)
	return res, nil
}

type outgoingMessage struct {
	messageID string
	raw       []byte
}

// buildOutgoing assembles an RFC 5322 message: an admin's name is appended as
// a signature (the mail goes out as the shared address, so this is how the
// recipient knows who they're actually talking to), subject and name are
// RFC 2047-encoded if not plain ASCII, and the body is quoted-printable so
// non-ASCII text and line endings survive.
func (h *Handler) buildOutgoing(in ReplyInput, to string) (outgoingMessage, error) {
	domain := "decayolympia.org"
	if i := strings.IndexByte(h.cfg.Address, '@'); i >= 0 {
		domain = h.cfg.Address[i+1:]
	}
	idBytes := make([]byte, 12)
	if _, err := rand.Read(idBytes); err != nil {
		return outgoingMessage{}, err
	}
	messageID := hex.EncodeToString(idBytes) + "@" + domain

	body := SignBody(in.Body, in.SenderName)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")

	var b strings.Builder
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "From: %s\r\n", formatAddress(h.cfg.FromName, h.cfg.Address))
	fmt.Fprintf(&b, "To: %s\r\n", formatAddress(in.ToName, to))
	fmt.Fprintf(&b, "Subject: %s\r\n", encodeHeader(in.Subject))
	fmt.Fprintf(&b, "Message-ID: <%s>\r\n", messageID)
	if reply := strings.Trim(in.InReplyTo, "<> \t"); reply != "" {
		fmt.Fprintf(&b, "In-Reply-To: <%s>\r\n", reply)
		fmt.Fprintf(&b, "References: <%s>\r\n", reply)
	}
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	b.WriteString("X-Mailer: decay-main\r\n")
	b.WriteString("\r\n")

	var qp strings.Builder
	w := quotedprintable.NewWriter(&qp)
	_, _ = w.Write([]byte(body))
	_ = w.Close()
	b.WriteString(qp.String())

	return outgoingMessage{messageID: messageID, raw: []byte(b.String())}, nil
}

// SignBody appends the sending admin's name to body, so the recipient knows
// who they're talking to even though the mail goes out as the shared
// address. Exported so the admin preview page can show the exact text a send
// will produce.
func SignBody(body, senderName string) string {
	body = strings.TrimRight(strings.ReplaceAll(body, "\r\n", "\n"), "\n \t")
	senderName = strings.TrimSpace(senderName)
	if senderName == "" {
		return body + "\n\n— DECAY Booking"
	}
	return body + "\n\n— " + senderName + "\nDECAY Booking"
}

func encodeHeader(v string) string {
	v = sanitizeHeader(v)
	return mime.QEncoding.Encode("UTF-8", v)
}

func formatAddress(name, addr string) string {
	name = sanitizeHeader(name)
	if name == "" {
		return addr
	}
	return mime.QEncoding.Encode("UTF-8", name) + " <" + addr + ">"
}

// sanitizeHeader strips CR/LF so a hostile value can't smuggle in extra
// headers, matching mail.Mailer's rule for the same reason.
func sanitizeHeader(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return strings.TrimSpace(v)
}

// smtpSend delivers msg over SMTP, following the same implicit-TLS-for-465,
// STARTTLS-otherwise rule as mail.Mailer.send.
func (h *Handler) smtpSend(to string, msg []byte) error {
	addr := h.cfg.smtpAddr()
	auth := smtp.PlainAuth("", h.cfg.SMTPUser, h.cfg.SMTPPass, h.cfg.SMTPHost)

	if !h.cfg.smtpImplicitTLS() {
		if err := smtp.SendMail(addr, auth, h.cfg.Address, []string{to}, msg); err != nil {
			return fmt.Errorf("could not send the reply: %w", err)
		}
		return nil
	}

	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: h.cfg.SMTPHost})
	if err != nil {
		return fmt.Errorf("could not connect to %s: %w", addr, err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, h.cfg.SMTPHost)
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}
	if err := c.Mail(h.cfg.Address); err != nil {
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

// fileInSent puts a copy of an outgoing message in the Sent folder. This is
// the one write this package performs against the mailbox — everything else
// reads with EXAMINE. Without it, a reply sent from the admin would never
// appear in the thread it was sent from, because SMTP submission alone
// doesn't save a copy.
func (h *Handler) fileInSent(raw []byte) error {
	c, err := h.dialIMAP()
	if err != nil {
		return err
	}
	defer c.Logout()

	if err := c.Login(h.cfg.IMAPUser, h.cfg.IMAPPass); err != nil {
		return err
	}

	folders, err := h.pickFolders(c)
	if err != nil {
		return err
	}
	var sent string
	for _, f := range folders {
		if !strings.EqualFold(f, "INBOX") {
			sent = f
			break
		}
	}
	if sent == "" {
		return fmt.Errorf("no Sent folder found")
	}

	return c.Append(sent, []string{`\Seen`}, time.Time{}, newLiteral(raw))
}

// literalBytes adapts a []byte to imap.Literal (io.Reader + Len).
type literalBytes struct {
	data []byte
	pos  int
}

func newLiteral(b []byte) *literalBytes { return &literalBytes{data: b} }

func (l *literalBytes) Read(p []byte) (int, error) {
	if l.pos >= len(l.data) {
		return 0, io.EOF
	}
	n := copy(p, l.data[l.pos:])
	l.pos += n
	return n, nil
}

func (l *literalBytes) Len() int { return len(l.data) }
