package staff

import (
	"testing"
	"time"
)

// sampleICS mirrors what a Nextcloud shared calendar exports: a VTIMEZONE
// block (whose own DTSTART must be ignored), a TZID-tagged local meeting, a
// UTC meeting, and an all-day entry. Lines are CRLF-terminated and one is
// folded, as a real feed's would be.
const sampleICS = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Nextcloud//EN\r\n" +
	"BEGIN:VTIMEZONE\r\n" +
	"TZID:America/Los_Angeles\r\n" +
	"BEGIN:STANDARD\r\n" +
	"DTSTART:19701101T020000\r\n" +
	"TZOFFSETFROM:-0700\r\n" +
	"TZOFFSETTO:-0800\r\n" +
	"END:STANDARD\r\n" +
	"END:VTIMEZONE\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:board-1@decay\r\n" +
	"DTSTART;TZID=America/Los_Angeles:20260115T190000\r\n" +
	"DTEND;TZID=America/Los_Angeles:20260115T203000\r\n" +
	"SUMMARY:Board meeting\\, January\r\n" +
	"LOCATION:DECAY\r\n" +
	"DESCRIPTION:Agenda:\r\n  budget and the grant\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:utc-1@decay\r\n" +
	"DTSTART:20260120T030000Z\r\n" +
	"SUMMARY:Late sync\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:allday-1@decay\r\n" +
	"DTSTART;VALUE=DATE:20260201\r\n" +
	"SUMMARY:Work party\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParse(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	ms := Parse([]byte(sampleICS), la)
	if len(ms) != 3 {
		t.Fatalf("got %d meetings, want 3", len(ms))
	}

	// Sorted soonest first: Jan 15, Jan 20, Feb 1.
	if ms[0].UID != "board-1@decay" || ms[1].UID != "utc-1@decay" || ms[2].UID != "allday-1@decay" {
		t.Fatalf("unexpected order: %s, %s, %s", ms[0].UID, ms[1].UID, ms[2].UID)
	}

	board := ms[0]
	// TZID local time resolves to the named zone.
	if got := board.Start.In(la).Format("2006-01-02 15:04"); got != "2026-01-15 19:00" {
		t.Errorf("board start = %s, want 2026-01-15 19:00", got)
	}
	if got := board.End.In(la).Format("15:04"); got != "20:30" {
		t.Errorf("board end = %s, want 20:30", got)
	}
	// TEXT escaping is undone, and the folded DESCRIPTION is rejoined.
	if board.Summary != "Board meeting, January" {
		t.Errorf("summary = %q", board.Summary)
	}
	if board.Description != "Agenda: budget and the grant" {
		t.Errorf("description = %q", board.Description)
	}

	// The VTIMEZONE's DTSTART (1970) must not have leaked in as a meeting.
	for _, m := range ms {
		if m.Start.Year() == 1970 {
			t.Fatalf("VTIMEZONE DTSTART leaked as a meeting: %+v", m)
		}
	}

	// UTC instant: 03:00Z is 19:00 the previous evening in LA.
	if got := ms[1].Start.In(la).Format("2006-01-02 15:04"); got != "2026-01-19 19:00" {
		t.Errorf("utc start in LA = %s, want 2026-01-19 19:00", got)
	}

	// All-day entry is flagged and its end defaults to the next day.
	allDay := ms[2]
	if !allDay.AllDay {
		t.Error("date-only entry not marked all-day")
	}
	if got := allDay.End.Sub(allDay.Start); got != 24*time.Hour {
		t.Errorf("all-day span = %v, want 24h", got)
	}
}

func TestUpcoming(t *testing.T) {
	la, _ := time.LoadLocation("America/Los_Angeles")
	ms := Parse([]byte(sampleICS), la)

	// A "now" just after the first meeting drops it and keeps the rest.
	now := time.Date(2026, 1, 16, 0, 0, 0, 0, la)
	up := Upcoming(ms, now, 0)
	if len(up) != 2 || up[0].UID != "utc-1@decay" {
		t.Fatalf("upcoming = %d, first %v", len(up), first(up))
	}

	// The limit caps the count.
	if got := Upcoming(ms, time.Date(2026, 1, 1, 0, 0, 0, 0, la), 1); len(got) != 1 {
		t.Errorf("limit 1 returned %d", len(got))
	}
}

func first(ms []Meeting) string {
	if len(ms) == 0 {
		return "<none>"
	}
	return ms[0].UID
}

func TestParseEmpty(t *testing.T) {
	la, _ := time.LoadLocation("America/Los_Angeles")
	if got := Parse(nil, la); len(got) != 0 {
		t.Errorf("empty feed returned %d meetings", len(got))
	}
}
