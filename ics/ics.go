// Package ics renders DECAY's events as an iCalendar feed (RFC 5545),
// the format Nextcloud, Apple Calendar, and Google Calendar all subscribe
// to. The site's SQLite database is the record; this is a read-only view
// of it that calendar clients poll.
package ics

import (
	"mime"
	"path"
	"strings"
	"time"

	"decay-main/db"
)

// defaultDuration is how long an event without an end time is shown as
// running. iCalendar treats a missing DTEND as a zero-length event, which
// most clients render as an unreadable sliver, and the old PHP site made
// the same two-hour assumption.
const defaultDuration = 2 * time.Hour

// maxLineOctets is the content-line limit from RFC 5545 section 3.1.
// Longer lines are folded onto continuation lines starting with a space.
const maxLineOctets = 75

// Calendar renders events as a complete VCALENDAR document. name is what
// subscribers see the calendar called in their client, and baseURL is the
// site's public origin, used to build absolute links back to each event.
func Calendar(name, baseURL string, events []db.Event) []byte {
	var b strings.Builder
	writeLine(&b, "BEGIN:VCALENDAR")
	writeLine(&b, "VERSION:2.0")
	writeLine(&b, "PRODID:-//DECAY//decay-main//EN")
	writeLine(&b, "CALSCALE:GREGORIAN")
	writeLine(&b, "METHOD:PUBLISH")
	writeLine(&b, "X-WR-CALNAME:"+escape(name))
	writeLine(&b, "X-WR-TIMEZONE:America/Los_Angeles")

	stamp := time.Now().UTC().Format(utcLayout)
	base := strings.TrimSuffix(baseURL, "/")
	for _, ev := range events {
		writeEvent(&b, ev, stamp, base)
	}

	writeLine(&b, "END:VCALENDAR")
	return []byte(b.String())
}

const utcLayout = "20060102T150405Z"

func writeEvent(b *strings.Builder, ev db.Event, stamp, base string) {
	end := ev.StartsAt.Add(defaultDuration)
	if ev.EndsAt != nil {
		end = *ev.EndsAt
	}

	writeLine(b, "BEGIN:VEVENT")
	writeLine(b, "UID:"+ev.UID)
	writeLine(b, "DTSTAMP:"+stamp)
	// Times are stored as absolute instants, so emitting them in UTC is
	// exact and saves shipping a VTIMEZONE block for the venue.
	writeLine(b, "DTSTART:"+ev.StartsAt.UTC().Format(utcLayout))
	writeLine(b, "DTEND:"+end.UTC().Format(utcLayout))
	writeLine(b, "SUMMARY:"+escape(ev.Title))
	if ev.Location != "" {
		writeLine(b, "LOCATION:"+escape(ev.Location))
	}
	if description := ev.Description; description != "" {
		writeLine(b, "DESCRIPTION:"+escape(description))
	}
	if ev.EventType != "" {
		writeLine(b, "CATEGORIES:"+escape(ev.EventType))
	}
	// Point at the event's own page rather than any external link, so a
	// calendar entry leads somewhere that stays under DECAY's control —
	// this URL persists in subscribers' clients.
	if base != "" {
		writeLine(b, "URL:"+escape(base+ev.Path()))
	}
	// The flyer travels as a link, not as data — the image itself lives on
	// the site. Clients that understand ATTACH show it; the rest ignore it.
	if ev.HasFlyer() && base != "" {
		attach := "ATTACH"
		if fmtType := mime.TypeByExtension(strings.ToLower(path.Ext(ev.Flyer))); fmtType != "" {
			attach += ";FMTTYPE=" + strings.SplitN(fmtType, ";", 2)[0]
		}
		writeLine(b, attach+":"+escape(base+ev.FlyerPath()))
	}
	writeLine(b, "END:VEVENT")
}

// escape applies the TEXT escaping rules from RFC 5545 section 3.3.11:
// backslash, semicolon, and comma are escaped, and newlines become \n.
// The order matters — backslashes have to go first.
func escape(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		";", `\;`,
		",", `\,`,
		"\r\n", `\n`,
		"\n", `\n`,
		"\r", `\n`,
	).Replace(s)
}

// writeLine appends one content line with CRLF, folding it if it exceeds
// the octet limit. The limit counts a continuation line's leading space,
// so those carry one octet less. Folding splits on octets rather than
// runes, so the cut is pulled back to a UTF-8 boundary to avoid splitting
// a multi-byte character across lines.
func writeLine(b *strings.Builder, line string) {
	limit := maxLineOctets
	for len(line) > limit {
		cut := limit
		for cut > 0 && !isBoundary(line[cut]) {
			cut--
		}
		if cut == 0 {
			cut = limit
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
		limit = maxLineOctets - 1 // the leading space counts
	}
	b.WriteString(line)
	b.WriteString("\r\n")
}

// isBoundary reports whether a byte starts a UTF-8 sequence, i.e. is not
// a continuation byte.
func isBoundary(c byte) bool { return c&0xc0 != 0x80 }
