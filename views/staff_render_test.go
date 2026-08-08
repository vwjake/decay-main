package views

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"decay-main/db"
	"decay-main/staff"
)

// TestStaffViewRenders exercises the admin staff-calendar screen in each of
// its states — not configured, configured-and-populated, and a fetch error —
// catching runtime template failures a type-check wouldn't.
func TestStaffViewRenders(t *testing.T) {
	me := db.User{Username: "smoke", Role: db.RoleMaster}
	venue, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 7, 20, 19, 0, 0, 0, venue)
	events := []db.Event{{ID: 7, Title: "No Tape", Slug: "no-tape", StartsAt: start.Add(24 * time.Hour)}}
	calendar := db.BuildCalendar(events, start, venue)

	// Not configured: the how-to-connect note stands in for the meetings
	// grid, but the events grid comes off the database and shows anyway.
	var buf strings.Builder
	if err := AdminStaff(StaffPage{Configured: false, Events: calendar, Venue: venue}, me).
		Render(context.Background(), &buf); err != nil {
		t.Fatalf("unconfigured render: %v", err)
	}
	if !strings.Contains(buf.String(), "STAFF_ICS_URL") {
		t.Error("unconfigured page doesn't explain how to connect the calendar")
	}
	if !strings.Contains(buf.String(), "No Tape") {
		t.Error("the events grid should render without a staff calendar connected")
	}
	if !strings.Contains(buf.String(), "July 2026") {
		t.Error("the month nav should work without a staff calendar connected")
	}

	// Configured with a meeting on the grid and in the upcoming list.
	ms := []staff.Meeting{{
		UID: "m1", Summary: "Board meeting", Location: "DECAY",
		Start: start, End: start.Add(time.Hour),
	}}
	page := StaffPage{
		Configured: true,
		Month:      staff.BuildMonth(ms, start, venue),
		Upcoming:   ms,
		Events:     calendar,
		Venue:      venue,
	}
	buf.Reset()
	if err := AdminStaff(page, me).Render(context.Background(), &buf); err != nil {
		t.Fatalf("configured render: %v", err)
	}
	if !strings.Contains(buf.String(), "Board meeting") {
		t.Error("meeting title missing from rendered page")
	}

	// Both calendars, one below the other, off the same month.
	if got := strings.Count(buf.String(), `<table class="calendar">`); got != 2 {
		t.Errorf("rendered %d calendar grids, want 2 (meetings and events)", got)
	}
	if !strings.Contains(buf.String(), "No Tape") {
		t.Error("the events grid is missing its event")
	}
	if !strings.Contains(buf.String(), `href="/events/no-tape"`) {
		t.Error("an event cell should link to the public event page")
	}
	meetings, ev := strings.Index(buf.String(), "Board meeting"), strings.Index(buf.String(), "No Tape")
	if meetings > ev {
		t.Error("the events grid should come below the meetings grid")
	}
	// One nav for both grids, so paging moves them together.
	if got := strings.Count(buf.String(), `class="pager mono"`); got != 1 {
		t.Errorf("rendered %d month navs, want 1 driving both grids", got)
	}

	// A fetch error shows its note but still renders.
	page.Error = "Couldn't reach the staff calendar."
	if err := AdminStaff(page, me).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("error-state render: %v", err)
	}
}
