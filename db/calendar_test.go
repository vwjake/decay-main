package db

import (
	"testing"
	"time"
	_ "time/tzdata"
)

func pacificZone(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func at(t *testing.T, loc *time.Location, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// TestBuildCalendarGrid covers the shape of the grid: whole weeks,
// starting on a Sunday, with the month's days marked in and the padding
// marked out.
func TestBuildCalendarGrid(t *testing.T) {
	loc := pacificZone(t)

	// July 2026 starts on a Wednesday and has 31 days, so it needs five
	// weeks with three leading and two trailing padding cells.
	month := at(t, loc, "2026-07-01 00:00")
	cal := BuildCalendar(nil, month, loc)

	if len(cal.Weeks) != 5 {
		t.Fatalf("got %d weeks, want 5", len(cal.Weeks))
	}
	for i, week := range cal.Weeks {
		if len(week) != 7 {
			t.Errorf("week %d has %d days, want 7", i, len(week))
		}
	}
	if got := cal.Weeks[0][0].Date.Weekday(); got != time.Sunday {
		t.Errorf("grid starts on %v, want Sunday", got)
	}

	inMonth := 0
	for _, week := range cal.Weeks {
		for _, day := range week {
			if day.InMonth {
				inMonth++
			}
		}
	}
	if inMonth != 31 {
		t.Errorf("got %d days in July, want 31", inMonth)
	}
	for i := 0; i < 3; i++ {
		if cal.Weeks[0][i].InMonth {
			t.Errorf("leading cell %d should be outside the month", i)
		}
	}
	if !cal.Weeks[0][3].InMonth || cal.Weeks[0][3].Date.Day() != 1 {
		t.Error("July 1 should be the fourth cell of the first week")
	}

	if cal.Label() != "July 2026" {
		t.Errorf("label %q, want %q", cal.Label(), "July 2026")
	}
	if cal.Prev != "2026-06" || cal.Next != "2026-08" {
		t.Errorf("nav is %s/%s, want 2026-06/2026-08", cal.Prev, cal.Next)
	}
}

// TestBuildCalendarPlacesEvents checks events land on their own day, in
// start order, and that a late show stays on the day it began rather than
// sliding into the next one via UTC.
func TestBuildCalendarPlacesEvents(t *testing.T) {
	loc := pacificZone(t)
	month := at(t, loc, "2026-07-01 00:00")

	events := []Event{
		{Title: "Morning", StartsAt: at(t, loc, "2026-07-15 10:00")},
		{Title: "Evening", StartsAt: at(t, loc, "2026-07-15 19:30")},
		{Title: "Late show", StartsAt: at(t, loc, "2026-07-18 21:00")},
		{Title: "Next month", StartsAt: at(t, loc, "2026-08-02 12:00")},
	}
	cal := BuildCalendar(events, month, loc)

	find := func(day int) CalendarDay {
		t.Helper()
		for _, week := range cal.Weeks {
			for _, d := range week {
				if d.InMonth && d.Date.Day() == day {
					return d
				}
			}
		}
		t.Fatalf("day %d not found in the grid", day)
		return CalendarDay{}
	}

	fifteenth := find(15)
	if len(fifteenth.Events) != 2 {
		t.Fatalf("July 15 has %d events, want 2", len(fifteenth.Events))
	}
	if fifteenth.Events[0].Title != "Morning" || fifteenth.Events[1].Title != "Evening" {
		t.Errorf("July 15 events out of order: %s then %s",
			fifteenth.Events[0].Title, fifteenth.Events[1].Title)
	}

	// 9pm Pacific is the next day in UTC; it must still show on the 18th.
	if got := len(find(18).Events); got != 1 {
		t.Errorf("July 18 has %d events, want 1 — a late show slid to another day", got)
	}

	for _, week := range cal.Weeks {
		for _, d := range week {
			for _, ev := range d.Events {
				if ev.Title == "Next month" {
					t.Error("an August event was placed on the July grid")
				}
			}
		}
	}
}

func TestParseMonth(t *testing.T) {
	loc := pacificZone(t)

	got := ParseMonth("2026-03", loc)
	if got.Year() != 2026 || got.Month() != time.March || got.Day() != 1 {
		t.Errorf("ParseMonth(2026-03) = %v", got)
	}

	// Anything unusable falls back to the current month rather than
	// erroring, so a hand-edited URL still renders.
	now := time.Now().In(loc)
	for _, raw := range []string{"", "nonsense", "2026-13", "2026"} {
		got := ParseMonth(raw, loc)
		if got.Year() != now.Year() || got.Month() != now.Month() {
			t.Errorf("ParseMonth(%q) = %v, want the current month", raw, got)
		}
	}
}

func TestShortTitle(t *testing.T) {
	if got := ShortTitle("Open Draw"); got != "Open Draw" {
		t.Errorf("short titles should pass through, got %q", got)
	}
	long := "Eating, Luna Galassini, Steve Jansen, Get Hat"
	got := ShortTitle(long)
	if len([]rune(got)) > 20 {
		t.Errorf("ShortTitle produced %d runes: %q", len([]rune(got)), got)
	}
	if got[len(got)-3:] != "…" {
		t.Errorf("a trimmed title should end in an ellipsis, got %q", got)
	}
}

func TestStartClock(t *testing.T) {
	loc := pacificZone(t)
	cases := map[string]string{
		"2026-07-15 19:30": "7:30p",
		"2026-07-15 19:00": "7p",
		"2026-07-15 09:15": "9:15a",
		"2026-07-15 00:00": "12a",
		"2026-07-15 12:00": "12p",
	}
	for value, want := range cases {
		ev := Event{StartsAt: at(t, loc, value)}
		if got := ev.StartClock(); got != want {
			t.Errorf("%s -> %q, want %q", value, got, want)
		}
	}
}
