package bookingmail

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
)

// stubSMTP is a minimal SMTP server for exercising Handler.Send without ever
// touching a real mailbox. It records the whole conversation so a test can
// assert on protocol, auth, and the exact bytes of the message.
type stubSMTP struct {
	mu   sync.Mutex
	auth struct{ user, pass string }
	from string
	rcpt []string
	data string
}

func startStubSMTP(t *testing.T) (addr string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	s := &stubSMTP{}
	stubSMTPState = s

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		s.serve(conn)
	}()
	return ln.Addr().String()
}

// stubSMTPState lets the test reach into the one connection the stub server
// accepted, without threading a channel through every call site.
var stubSMTPState *stubSMTP

func (s *stubSMTP) serve(conn net.Conn) {
	r := bufio.NewReader(conn)
	fmt.Fprint(conn, "220 stub.test ESMTP\r\n")

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO"):
			fmt.Fprint(conn, "250-stub.test\r\n250 AUTH PLAIN LOGIN\r\n")
		case strings.HasPrefix(upper, "AUTH PLAIN "):
			raw, _ := base64.StdEncoding.DecodeString(line[len("AUTH PLAIN "):])
			parts := strings.SplitN(string(raw), "\x00", 3)
			if len(parts) == 3 {
				s.auth.user, s.auth.pass = parts[1], parts[2]
			}
			fmt.Fprint(conn, "235 2.7.0 OK\r\n")
		case upper == "AUTH LOGIN":
			fmt.Fprint(conn, "334 VXNlcm5hbWU6\r\n")
			userLine, _ := r.ReadString('\n')
			u, _ := base64.StdEncoding.DecodeString(strings.TrimRight(userLine, "\r\n"))
			s.auth.user = string(u)
			fmt.Fprint(conn, "334 UGFzc3dvcmQ6\r\n")
			passLine, _ := r.ReadString('\n')
			p, _ := base64.StdEncoding.DecodeString(strings.TrimRight(passLine, "\r\n"))
			s.auth.pass = string(p)
			fmt.Fprint(conn, "235 2.7.0 OK\r\n")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.from = extractAddr(line)
			fmt.Fprint(conn, "250 2.1.0 OK\r\n")
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.rcpt = append(s.rcpt, extractAddr(line))
			fmt.Fprint(conn, "250 2.1.5 OK\r\n")
		case upper == "DATA":
			fmt.Fprint(conn, "354 End data with <CR><LF>.<CR><LF>\r\n")
			var body strings.Builder
			for {
				dl, err := r.ReadString('\n')
				if err != nil || dl == ".\r\n" {
					break
				}
				body.WriteString(dl)
			}
			s.data = body.String()
			fmt.Fprint(conn, "250 2.0.0 OK: queued\r\n")
		case upper == "QUIT":
			fmt.Fprint(conn, "221 2.0.0 Bye\r\n")
			return
		default:
			fmt.Fprint(conn, "500 5.5.2 unrecognized\r\n")
		}
	}
}

func extractAddr(line string) string {
	i, j := strings.Index(line, "<"), strings.LastIndex(line, ">")
	if i < 0 || j < 0 || j < i {
		return ""
	}
	return line[i+1 : j]
}

func TestSendDeliversAuthenticatedMessage(t *testing.T) {
	addr := startStubSMTP(t)
	host, port, _ := net.SplitHostPort(addr)

	h := New(Config{
		IMAPHost: "127.0.0.1", // unused by Send() itself; fileInSent is tested separately
		SMTPHost: host,
		SMTPPort: port,
		SMTPUser: "booking@decayolympia.org",
		SMTPPass: "hunter2",
		Address:  "booking@decayolympia.org",
		FromName: "DECAY Booking",
	})

	res, err := h.Send(ReplyInput{
		To:         "hayesnoblemusic@gmail.com",
		ToName:     "Hayes Noble",
		Subject:    "Re: April 3rd — tour stop",
		Body:       "Yes, April 3rd works.\n.dot line at start\nLoad-in at 6pm.",
		SenderName: "Jake Fery",
		InReplyTo:  "msg1@example.com",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.MessageID == "" {
		t.Error("expected a Message-ID to be returned")
	}
	// fileInSent will fail (IMAPHost 127.0.0.1 has nothing listening on the
	// real IMAP port), and that must surface as a warning, not a failure —
	// the mail genuinely went out.
	if res.Warning == "" {
		t.Error("expected a warning about filing the Sent copy, since no IMAP stub is running")
	}

	s := stubSMTPState
	if s.auth.user != "booking@decayolympia.org" || s.auth.pass != "hunter2" {
		t.Errorf("auth = %+v", s.auth)
	}
	if s.from != "booking@decayolympia.org" {
		t.Errorf("MAIL FROM = %q", s.from)
	}
	if len(s.rcpt) != 1 || s.rcpt[0] != "hayesnoblemusic@gmail.com" {
		t.Errorf("RCPT TO = %v", s.rcpt)
	}
	if !strings.Contains(s.data, "\r\n..dot line at start") {
		t.Errorf("leading dot should be stuffed on the wire:\n%s", s.data)
	}
}
