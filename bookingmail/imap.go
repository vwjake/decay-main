package bookingmail

import (
	"crypto/tls"
	"fmt"
	"net/textproto"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"

	// Registers charset decoding for message.CharsetReader, so a message
	// declaring ISO-8859-1 or similar decodes correctly instead of coming
	// back garbled. Side-effect import only.
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
)

const (
	maxMessagesPerThread = 40
	maxBodyChars         = 20000
	errorCacheTTL        = 2 * time.Minute
)

// Handler is the shared entry point for the booking mailbox: reading threads
// and sending replies. It's safe for concurrent use.
type Handler struct {
	cfg Config

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	thread Thread
	until  time.Time
}

// New builds a Handler from cfg. A disabled Config still yields a usable
// Handler whose Thread calls report Configured: false rather than erroring.
func New(cfg Config) *Handler {
	return &Handler{cfg: cfg, cache: make(map[string]cacheEntry)}
}

func (h *Handler) Enabled() bool  { return h.cfg.Enabled() }
func (h *Handler) CanSend() bool  { return h.cfg.CanSend() }
func (h *Handler) Address() string { return h.cfg.Address }

// Thread returns the email history with addresses, reading from cache when
// fresh. addresses is normalized (lowercased, deduplicated, the mailbox's own
// address removed) before use.
func (h *Handler) Thread(addresses []string, forceRefresh bool) Thread {
	addrs := normalizeAddresses(addresses, h.cfg.Address)

	if len(addrs) == 0 {
		return Thread{Configured: h.cfg.Enabled(), FetchedAt: time.Now()}
	}
	if !h.cfg.Enabled() {
		return Thread{Addresses: addrs, Configured: false}
	}

	key := strings.Join(addrs, "|")
	if !forceRefresh {
		if t, ok := h.cached(key); ok {
			return t
		}
	}

	t := h.fetch(addrs)
	t.Configured = true
	h.store(key, t)
	return t
}

func (h *Handler) cached(key string) (Thread, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.cache[key]
	if !ok || time.Now().After(entry.until) {
		return Thread{}, false
	}
	entry.thread.Cached = true
	return entry.thread, true
}

func (h *Handler) store(key string, t Thread) {
	ttl := h.cfg.CacheTTL
	if t.Err != nil {
		// A failed lookup is cached briefly so a misconfiguration doesn't
		// slow every page load, but it shouldn't stick around for the full TTL.
		ttl = errorCacheTTL
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cache[key] = cacheEntry{thread: t, until: time.Now().Add(ttl)}
}

// Forget drops every cached thread that covers address, so a just-sent reply
// shows up immediately instead of waiting out the cache.
func (h *Handler) Forget(address string) {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, entry := range h.cache {
		for _, a := range entry.thread.Addresses {
			if a == address {
				delete(h.cache, key)
				break
			}
		}
	}
}

func normalizeAddresses(addresses []string, mailboxAddr string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range addresses {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" || !looksLikeEmail(a) || a == strings.ToLower(mailboxAddr) || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func looksLikeEmail(s string) bool { return emailRe.MatchString(s) }

// fetch does the actual IMAP round trip: connect, find INBOX + the Sent
// folder, search both for addrs, fetch and parse matches, de-duplicate by
// Message-ID, and sort by date.
func (h *Handler) fetch(addrs []string) Thread {
	t := Thread{Addresses: addrs, FetchedAt: time.Now()}

	c, err := h.dialIMAP()
	if err != nil {
		t.Err = err
		return t
	}
	defer c.Logout()

	if err := c.Login(h.cfg.IMAPUser, h.cfg.IMAPPass); err != nil {
		t.Err = fmt.Errorf("IMAP login failed: %w", err)
		return t
	}

	folders, err := h.pickFolders(c)
	if err != nil {
		t.Err = err
		return t
	}

	byKey := make(map[string]Message)
	for _, folder := range folders {
		msgs, err := h.searchFolder(c, folder, addrs)
		if err != nil {
			// One unreadable folder shouldn't sink threads found elsewhere.
			continue
		}
		for _, m := range msgs {
			byKey[m.Key] = m
		}
	}

	messages := make([]Message, 0, len(byKey))
	for _, m := range byKey {
		messages = append(messages, m)
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].Date.Before(messages[j].Date) })
	if len(messages) > maxMessagesPerThread {
		messages = messages[len(messages)-maxMessagesPerThread:]
	}
	t.Messages = messages
	return t
}

func (h *Handler) dialIMAP() (*imapclient.Client, error) {
	addr := h.cfg.imapAddr()
	var c *imapclient.Client
	var err error
	if h.cfg.imapImplicitTLS() {
		c, err = imapclient.DialTLS(addr, &tls.Config{ServerName: h.cfg.IMAPHost})
	} else {
		c, err = imapclient.Dial(addr)
		if err == nil {
			err = c.StartTLS(&tls.Config{ServerName: h.cfg.IMAPHost})
		}
	}
	if err != nil {
		return nil, fmt.Errorf("could not connect to %s: %w", addr, err)
	}
	return c, nil
}

// pickFolders returns INBOX plus the best Sent-folder candidate, handling
// Dovecot/cPanel ("INBOX.Sent"), Gmail ("[Gmail]/Sent Mail") and plain
// "Sent"/"Sent Items"/"Sent Messages" layouts.
func (h *Handler) pickFolders(c *imapclient.Client) ([]string, error) {
	mailboxes := make(chan *imap.MailboxInfo, 20)
	done := make(chan error, 1)
	go func() { done <- c.List("", "*", mailboxes) }()

	var all []*imap.MailboxInfo
	for mb := range mailboxes {
		all = append(all, mb)
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("could not list mailboxes: %w", err)
	}

	folders := []string{"INBOX"}
	var sentFolder string

	for _, mb := range all {
		for _, attr := range mb.Attributes {
			if strings.EqualFold(attr, imap.SentAttr) {
				sentFolder = mb.Name
			}
		}
	}

	if sentFolder == "" {
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`(?i)^(?:INBOX[./])?Sent$`),
			regexp.MustCompile(`(?i)^\[Gmail\][./]Sent Mail$`),
			regexp.MustCompile(`(?i)(?:^|[./])Sent (?:Items|Messages|Mail)$`),
			regexp.MustCompile(`(?i)(?:^|[./])Sent$`),
		}
		for _, pat := range patterns {
			for _, mb := range all {
				if pat.MatchString(mb.Name) {
					sentFolder = mb.Name
					break
				}
			}
			if sentFolder != "" {
				break
			}
		}
	}

	if sentFolder != "" && !strings.EqualFold(sentFolder, "INBOX") {
		folders = append(folders, sentFolder)
	}
	return folders, nil
}

// searchFolder finds and parses messages to/from/cc'd to any of addrs within
// one folder, opened read-only (EXAMINE, not SELECT) so nothing is ever
// marked as read.
func (h *Handler) searchFolder(c *imapclient.Client, folder string, addrs []string) ([]Message, error) {
	if _, err := c.Select(folder, true); err != nil {
		return nil, err
	}

	criteria := addressCriteria(addrs)
	uids, err := c.UidSearch(criteria)
	if err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return nil, nil
	}
	if len(uids) > maxMessagesPerThread {
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		uids = uids[len(uids)-maxMessagesPerThread:]
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)

	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchInternalDate, section.FetchItem()}

	fetched := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(seqset, items, fetched) }()

	var out []Message
	for raw := range fetched {
		lit := raw.GetBody(section)
		if lit == nil {
			continue
		}
		msg, err := parseMessage(raw, folder, lit, h.cfg.Address)
		if err == nil {
			out = append(out, msg)
		}
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return out, nil
}

// addressCriteria builds "match any of: From/To/Cc contains addr" for each
// address in addrs, ORed together.
func addressCriteria(addrs []string) *imap.SearchCriteria {
	var all []*imap.SearchCriteria
	for _, addr := range addrs {
		for _, field := range []string{"From", "To", "Cc"} {
			all = append(all, &imap.SearchCriteria{
				Header: textproto.MIMEHeader{field: []string{addr}},
			})
		}
	}
	if len(all) == 0 {
		return imap.NewSearchCriteria()
	}
	result := all[0]
	for _, c := range all[1:] {
		result = &imap.SearchCriteria{Or: [][2]*imap.SearchCriteria{{result, c}}}
	}
	return result
}

// parseMessage turns a fetched IMAP message into our Message shape, using
// go-message to walk MIME parts, decode transfer encodings and charsets, and
// pull out the readable text plus any attachment names.
func parseMessage(raw *imap.Message, folder string, body imap.Literal, mailboxAddr string) (Message, error) {
	mr, err := mail.CreateReader(body)
	if err != nil {
		return Message{}, err
	}

	h := mr.Header
	date, _ := h.Date()
	if date.IsZero() && raw.InternalDate.IsZero() == false {
		date = raw.InternalDate
	}
	subject, _ := h.Subject()
	messageID, _ := h.MessageID()
	inReplyTo := ""
	if ids, _ := h.MsgIDList("In-Reply-To"); len(ids) > 0 {
		inReplyTo = ids[0]
	}

	fromName, fromEmail := firstAddress(h, "From")
	to := addressList(h, "To")
	cc := addressList(h, "Cc")

	var plainBody, htmlBody string
	var attachments []string
	for {
		p, err := mr.NextPart()
		if err != nil {
			break // io.EOF or a malformed part; use whatever was already read
		}
		switch ph := p.Header.(type) {
		case *mail.AttachmentHeader:
			name, _ := ph.Filename()
			if name == "" {
				if ct, _, err := ph.ContentType(); err == nil {
					name = ct
				}
			}
			attachments = append(attachments, name)
		case *mail.InlineHeader:
			ct, params, _ := ph.ContentType()
			text := readAllString(p.Body)
			switch ct {
			case "text/plain":
				if plainBody == "" {
					plainBody = text
				}
			case "text/html":
				if htmlBody == "" {
					htmlBody = text
				}
			default:
				_ = params
			}
		}
	}

	full := plainBody
	if strings.TrimSpace(full) == "" {
		full = htmlToText(htmlBody)
	}
	full = strings.ReplaceAll(full, "\r\n", "\n")

	truncated := false
	if len([]rune(full)) > maxBodyChars {
		r := []rune(full)
		full = string(r[:maxBodyChars])
		truncated = true
	}

	newText, quoted := splitQuoted(full)

	key := "raw:" + folder + ":" + fmt.Sprint(raw.Uid)
	if messageID != "" {
		key = "mid:" + messageID
	}

	direction := "received"
	if strings.EqualFold(fromEmail, mailboxAddr) {
		direction = "sent"
	}

	return Message{
		Key:         key,
		UID:         raw.Uid,
		Folder:      folder,
		MessageID:   messageID,
		InReplyTo:   inReplyTo,
		Date:        date,
		Direction:   direction,
		FromName:    fromName,
		FromEmail:   fromEmail,
		To:          to,
		CC:          cc,
		Subject:     subject,
		Body:        newText,
		Quoted:      quoted,
		Truncated:   truncated,
		Attachments: attachments,
	}, nil
}

func firstAddress(h mail.Header, field string) (name, email string) {
	list, err := h.AddressList(field)
	if err != nil || len(list) == 0 {
		return "", ""
	}
	return list[0].Name, list[0].Address
}

func addressList(h mail.Header, field string) string {
	list, err := h.AddressList(field)
	if err != nil || len(list) == 0 {
		return ""
	}
	parts := make([]string, len(list))
	for i, a := range list {
		if a.Name != "" {
			parts[i] = a.Name + " <" + a.Address + ">"
		} else {
			parts[i] = a.Address
		}
	}
	return strings.Join(parts, ", ")
}
