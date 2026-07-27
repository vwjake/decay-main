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

	// Not configured: the how-to-connect note stands in for the grid.
	var buf strings.Builder
	if err := AdminStaff(StaffPage{Configured: false}, me).Render(context.Background(), &buf); err != nil {
		t.Fatalf("unconfigured render: %v", err)
	}
	if !strings.Contains(buf.String(), "STAFF_ICS_URL") {
		t.Error("unconfigured page doesn't explain how to connect the calendar")
	}

	// Configured with a meeting on the grid and in the upcoming list.
	start := time.Date(2026, 7, 20, 19, 0, 0, 0, venue)
	ms := []staff.Meeting{{
		UID: "m1", Summary: "Board meeting", Location: "DECAY",
		Start: start, End: start.Add(time.Hour),
	}}
	page := StaffPage{
		Configured: true,
		Month:      staff.BuildMonth(ms, start, venue),
		Upcoming:   ms,
		Venue:      venue,
	}
	buf.Reset()
	if err := AdminStaff(page, me).Render(context.Background(), &buf); err != nil {
		t.Fatalf("configured render: %v", err)
	}
	if !strings.Contains(buf.String(), "Board meeting") {
		t.Error("meeting title missing from rendered page")
	}

	// A fetch error shows its note but still renders.
	page.Error = "Couldn't reach the staff calendar."
	if err := AdminStaff(page, me).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("error-state render: %v", err)
	}
}
