package views

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"decay-main/db"
)

// TestStaffViewRenders exercises the admin calendar screen in each of its
// states — not configured, configured-and-populated, and a fetch error —
// catching runtime template failures a type-check wouldn't.
func TestStaffViewRenders(t *testing.T) {
	me := db.User{Username: "smoke", Role: db.RoleMaster}
	venue, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 7, 20, 19, 0, 0, 0, venue)

	calendar := AdminCalMonth{
		Month: time.Date(2026, 7, 1, 0, 0, 0, 0, venue),
		Prev:  "2026-06",
		Next:  "2026-08",
		Count: 1,
		Weeks: [][]AdminCalDay{{{
			Date:    start.Add(24 * time.Hour),
			InMonth: true,
			Entries: []CalEntry{{
				Category: CalEvent,
				Title:    "No Tape",
				Href:     "/events/no-tape",
				Linkable: true,
			}},
		}}},
	}

	// Not configured: the how-to-connect note stands in, but the calendar
	// grid still renders since it comes off the database, not Nextcloud.
	var buf strings.Builder
	if err := AdminStaff(StaffPage{Configured: false, Calendar: calendar, Venue: venue}, me).
		Render(context.Background(), &buf); err != nil {
		t.Fatalf("unconfigured render: %v", err)
	}
	if !strings.Contains(buf.String(), "STAFF_ICS_URL") {
		t.Error("unconfigured page doesn't explain how to connect the calendar")
	}
	if !strings.Contains(buf.String(), "No Tape") {
		t.Error("the calendar grid should render without a staff calendar connected")
	}
	if !strings.Contains(buf.String(), "July 2026") {
		t.Error("the month nav should work without a staff calendar connected")
	}
	if !strings.Contains(buf.String(), `href="/events/no-tape"`) {
		t.Error("an event cell should link to the public event page")
	}

	// Configured, with a meeting merged into the same grid.
	calendar.Weeks[0][0].Entries = append(calendar.Weeks[0][0].Entries, CalEntry{
		Category: CalStaff,
		Title:    "Board meeting",
	})
	calendar.Count = 2
	page := StaffPage{Configured: true, Calendar: calendar, Venue: venue}
	buf.Reset()
	if err := AdminStaff(page, me).Render(context.Background(), &buf); err != nil {
		t.Fatalf("configured render: %v", err)
	}
	if !strings.Contains(buf.String(), "Board meeting") {
		t.Error("meeting title missing from rendered page")
	}
	if !strings.Contains(buf.String(), "No Tape") {
		t.Error("the event is missing from the merged grid")
	}

	// One grid, one nav, and the three category toggles.
	if got := strings.Count(buf.String(), `<table class="calendar">`); got != 1 {
		t.Errorf("rendered %d calendar grids, want 1 (events, requests, and meetings merged)", got)
	}
	if got := strings.Count(buf.String(), `class="pager mono"`); got != 1 {
		t.Errorf("rendered %d month navs, want 1", got)
	}
	for _, id := range []string{"cal-filter-event", "cal-filter-request", "cal-filter-staff"} {
		if !strings.Contains(buf.String(), `id="`+id+`"`) {
			t.Errorf("missing category toggle %q", id)
		}
	}

	// A fetch error shows its note but still renders.
	page.Error = "Couldn't reach the staff calendar."
	if err := AdminStaff(page, me).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("error-state render: %v", err)
	}
}
