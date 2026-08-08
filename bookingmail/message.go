package bookingmail

import "time"

// Message is one email in a thread. The Go struct itself is the schema —
// every field is always present (zero-valued if absent), so a template can
// rely on the shape without checking for missing keys the way a JSON record
// would need to.
type Message struct {
	Key         string // stable identity for de-duplicating across folders
	UID         uint32
	Folder      string
	MessageID   string
	InReplyTo   string
	Date        time.Time
	Direction   string // "sent" or "received"
	FromName    string
	FromEmail   string
	To          string
	CC          string
	Subject     string
	Body        string // the part of the body before any quoted reply chain
	Quoted      string // the quoted reply chain, if any, shown collapsed
	Truncated   bool
	Attachments []string
}

func (m Message) IsSent() bool { return m.Direction == "sent" }

// When renders the message time in the venue's timezone for display.
func (m Message) When(venue *time.Location) string {
	return m.Date.In(venue).Format("Jan 2, 2006 · 3:04 PM")
}

// Snippet is a short, quote-stripped preview for a collapsed row.
func (m Message) Snippet(length int) string {
	return snippet(m.Body, length)
}

// DisplayBody is Body with the truncation notice appended, if any — the
// exact text a template should show, so it doesn't have to reproduce that
// conditional itself.
func (m Message) DisplayBody() string {
	if !m.Truncated {
		return m.Body
	}
	return m.Body + "\n\n… message truncated."
}

// Thread is the result of fetching a mailbox for messages to/from a set of
// addresses.
type Thread struct {
	Messages   []Message
	Addresses  []string
	FetchedAt  time.Time
	Cached     bool
	Configured bool
	Err        error
}

// OK reports whether the fetch succeeded (or there was nothing to fetch).
func (t Thread) OK() bool { return t.Err == nil }

// Counts returns how many messages were received vs sent, for a summary line.
func (t Thread) Counts() (received, sent int) {
	for _, m := range t.Messages {
		if m.IsSent() {
			sent++
		} else {
			received++
		}
	}
	return
}
