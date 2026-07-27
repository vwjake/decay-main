// Package meetings reads DECAY's internal calendar. Where the public site
// publishes an .ics feed that Nextcloud subscribes to (see package ics),
// this goes the other way: the admin panel subscribes to a separate,
// internal Nextcloud calendar — board and organising meetings — and shows
// it. It's the same one-way, credential-free arrangement in reverse. We
// only ever read a shared read-only .ics URL; nothing is written back.
package meetings

import (
	"sort"
	"strings"
	"time"
)

// Meeting is one entry from the internal calendar, reduced to what the
// admin view shows. The source calendar is the record; this is a snapshot.
type Meeting struct {
	UID         string
	Summary     string
	Start       time.Time
	End         time.Time
	Location    string
	Description string
	// AllDay marks an entry given as a date with no time (a VALUE=DATE
	// DTSTART), which shows as spanning the day rather than at a clock time.
	AllDay bool
}

// defaultDuration fills in an end for an entry that gives only a start, so
// a meeting reads as a block rather than an instant. Mirrors the ics
// package's assumption for events without an end.
const defaultDuration = time.Hour

// Parse reads an iCalendar document and returns its meetings, soonest
// first. venue is the timezone a floating or date-only time is read in —
// times carrying their own zone (a Z suffix or a TZID naming an IANA zone)
// keep it. A line this parser can't make sense of is skipped rather than
// failing the whole feed, so one odd entry never blanks the calendar.
func Parse(data []byte, venue *time.Location) []Meeting {
	lines := unfold(string(data))

	var meetings []Meeting
	// A VEVENT's properties are only read at the top of the component
	// stack: a VTIMEZONE nested in the calendar carries its own DTSTART
	// lines that must not be mistaken for a meeting's.
	var stack []string
	var cur *Meeting

	for _, line := range lines {
		name, params, value := splitLine(line)
		switch {
		case name == "BEGIN":
			stack = append(stack, value)
			if value == "VEVENT" {
				cur = &Meeting{}
			}
		case name == "END":
			if value == "VEVENT" && cur != nil {
				finish(cur)
				meetings = append(meetings, *cur)
				cur = nil
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case cur != nil && topIs(stack, "VEVENT"):
			apply(cur, name, params, value, venue)
		}
	}

	sort.Slice(meetings, func(i, j int) bool {
		return meetings[i].Start.Before(meetings[j].Start)
	})
	return meetings
}

// Upcoming returns at most limit meetings that haven't ended before now,
// soonest first. A limit of zero means no cap.
func Upcoming(all []Meeting, now time.Time, limit int) []Meeting {
	var out []Meeting
	for _, m := range all {
		if m.End.After(now) {
			out = append(out, m)
			if limit > 0 && len(out) == limit {
				break
			}
		}
	}
	return out
}

func topIs(stack []string, want string) bool {
	return len(stack) > 0 && stack[len(stack)-1] == want
}

func finish(m *Meeting) {
	if m.End.IsZero() {
		if m.AllDay {
			m.End = m.Start.AddDate(0, 0, 1)
		} else {
			m.End = m.Start.Add(defaultDuration)
		}
	}
}

func apply(m *Meeting, name string, params map[string]string, value string, venue *time.Location) {
	switch name {
	case "UID":
		m.UID = value
	case "SUMMARY":
		m.Summary = unescapeText(value)
	case "LOCATION":
		m.Location = unescapeText(value)
	case "DESCRIPTION":
		m.Description = unescapeText(value)
	case "DTSTART":
		if t, allDay, ok := parseTime(params, value, venue); ok {
			m.Start = t
			m.AllDay = allDay
		}
	case "DTEND":
		if t, _, ok := parseTime(params, value, venue); ok {
			m.End = t
		}
	}
}

// parseTime reads an iCalendar date or date-time. It handles the three
// forms Nextcloud emits: a UTC instant (trailing Z), a date-only value
// (VALUE=DATE), and a local time tagged with an IANA TZID; anything else is
// read as floating time in the venue zone.
func parseTime(params map[string]string, value string, venue *time.Location) (time.Time, bool, bool) {
	value = strings.TrimSpace(value)

	if params["VALUE"] == "DATE" || (len(value) == 8 && !strings.Contains(value, "T")) {
		if t, err := time.ParseInLocation("20060102", value, venue); err == nil {
			return t, true, true
		}
		return time.Time{}, false, false
	}

	if strings.HasSuffix(value, "Z") {
		if t, err := time.ParseInLocation("20060102T150405Z", value, time.UTC); err == nil {
			return t, false, true
		}
		return time.Time{}, false, false
	}

	loc := venue
	if tzid := params["TZID"]; tzid != "" {
		// TZIDs from Nextcloud are IANA names, which LoadLocation resolves
		// against the bundled tz database. A name it doesn't know falls back
		// to the venue zone rather than failing the entry.
		if l, err := time.LoadLocation(tzid); err == nil {
			loc = l
		}
	}
	if t, err := time.ParseInLocation("20060102T150405", value, loc); err == nil {
		return t, false, true
	}
	return time.Time{}, false, false
}

// unfold splits a raw iCalendar document into logical lines, rejoining the
// continuation lines RFC 5545 folds long content onto (a line beginning
// with a space or tab continues the one before it).
func unfold(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")

	var lines []string
	for _, physical := range strings.Split(raw, "\n") {
		if physical == "" {
			continue
		}
		if (physical[0] == ' ' || physical[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += physical[1:]
			continue
		}
		lines = append(lines, physical)
	}
	return lines
}

// splitLine breaks a content line into its property name, its parameters,
// and its value. The value starts after the first colon that isn't inside a
// quoted parameter; parameters are the semicolon-separated KEY=VALUE pairs
// between the name and that colon.
func splitLine(line string) (name string, params map[string]string, value string) {
	colon := unquotedIndex(line, ':')
	if colon < 0 {
		return strings.ToUpper(line), nil, ""
	}
	head, value := line[:colon], line[colon+1:]

	parts := splitUnquoted(head, ';')
	name = strings.ToUpper(parts[0])
	if len(parts) > 1 {
		params = map[string]string{}
		for _, p := range parts[1:] {
			if eq := strings.IndexByte(p, '='); eq >= 0 {
				key := strings.ToUpper(strings.TrimSpace(p[:eq]))
				params[key] = strings.Trim(strings.TrimSpace(p[eq+1:]), `"`)
			}
		}
	}
	return name, params, value
}

// unquotedIndex finds the first occurrence of sep that isn't inside a
// double-quoted run, or -1.
func unquotedIndex(s string, sep byte) int {
	quoted := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			quoted = !quoted
		case sep:
			if !quoted {
				return i
			}
		}
	}
	return -1
}

// splitUnquoted splits s on sep, ignoring separators inside double quotes.
func splitUnquoted(s string, sep byte) []string {
	var out []string
	quoted := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			quoted = !quoted
		case sep:
			if !quoted {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// unescapeText reverses the TEXT escaping from RFC 5545 section 3.3.11.
func unescapeText(s string) string {
	return strings.NewReplacer(
		`\n`, "\n",
		`\N`, "\n",
		`\,`, ",",
		`\;`, ";",
		`\\`, `\`,
	).Replace(s)
}
