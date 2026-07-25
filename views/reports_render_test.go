package views

import (
	"context"
	"io"
	"testing"
	"time"

	"decay-main/db"
)

// TestReportViewsRender exercises the report templates end to end, catching
// any runtime template failure that a type-check alone wouldn't — a bad
// SafeURL, a nil deref in a helper — with data shaped like a real page.
func TestReportViewsRender(t *testing.T) {
	me := db.User{Username: "smoke", Role: db.RoleMaster}
	att := int64(42)
	door := int64(12000)
	eventID := int64(7)
	now := time.Date(2026, 8, 1, 20, 0, 0, 0, time.UTC)

	page := ReportPage{
		Title:     "Q3 2026",
		IsQuarter: true,
		Selected:  db.Quarter{Year: 2026, Q: 3},
		FromStr:   "2026-07-01",
		ToStr:     "2026-09-30",
		Quarters:  []db.Quarter{{Year: 2026, Q: 3}, {Year: 2026, Q: 2}},
		Stats: db.Stats{
			Events: 12, Attendance: 300, AttendanceEvents: 10,
			DoorCents: 45000, DonationCents: 8000, DonationCount: 3,
			VolunteerRolesFilled: 5,
			ByType:               []db.TypeStat{{Type: "Music", Events: 8, Attendance: 250}},
		},
		Donations: []db.Donation{
			{ID: 1, EventID: &eventID, EventTitle: "Show A", AmountCents: 5000, Source: "jar", ReceivedAt: now},
			{ID: 2, AmountCents: 3000, Source: "online", ReceivedAt: now},
		},
	}
	if err := AdminReport(page, me).Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminReport render: %v", err)
	}

	rows := []EntryRow{
		{
			Event:         db.Event{ID: 7, Title: "Show A", EventType: "Music", StartsAt: now},
			Report:        db.EventReport{EventID: 7, Attendance: &att, DoorCents: &door, Notes: "packed", Recorded: true},
			DonationCents: 5000,
		},
		{
			Event: db.Event{ID: 8, Title: "Open Draw", EventType: "Open Draw", StartsAt: now},
			// Zero-value report: nothing recorded yet.
		},
	}
	pg := db.Page{Number: 1, Total: 2, Count: 60, PerPage: 50, Path: "/admin/reports/entry"}
	if err := AdminReportsEntry(rows, pg, me, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatalf("AdminReportsEntry render: %v", err)
	}
}
