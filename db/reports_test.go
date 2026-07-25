package db

import (
	"path/filepath"
	"testing"
)

// TestRangeStats seeds a quarter's worth of events, numbers, and donations
// and checks the aggregate only counts what falls inside the range, and
// attributes door take by event date but donations by date received.
func TestRangeStats(t *testing.T) {
	loc := pacificZone(t)
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	q := Quarter{Year: 2026, Q: 3} // Jul–Sep 2026
	from, to := q.Range(loc)

	// In-range event with full numbers.
	inA, err := CreateEvent(conn, Event{Title: "Show A", EventType: "Music", StartsAt: at(t, loc, "2026-07-10 20:00")})
	if err != nil {
		t.Fatal(err)
	}
	// In-range event, attendance only (no door figure).
	inB, err := CreateEvent(conn, Event{Title: "Draw B", EventType: "Open Draw", StartsAt: at(t, loc, "2026-08-05 18:00")})
	if err != nil {
		t.Fatal(err)
	}
	// Out-of-range event (next quarter) that must not be counted.
	outC, err := CreateEvent(conn, Event{Title: "Show C", EventType: "Music", StartsAt: at(t, loc, "2026-10-02 20:00")})
	if err != nil {
		t.Fatal(err)
	}

	att40, att15 := int64(40), int64(15)
	door := int64(12000) // $120
	if err := SaveEventReport(conn, inA, &att40, &door, "packed"); err != nil {
		t.Fatal(err)
	}
	if err := SaveEventReport(conn, inB, &att15, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := SaveEventReport(conn, outC, &att40, &door, ""); err != nil {
		t.Fatal(err)
	}

	if err := AddVolunteer(conn, inA, "door", "Jamie"); err != nil {
		t.Fatal(err)
	}

	// A gift at event A, a standalone gift in-quarter, and one after it.
	if err := AddDonation(conn, &inA, 5000, "jar", "", at(t, loc, "2026-07-10 22:00")); err != nil {
		t.Fatal(err)
	}
	if err := AddDonation(conn, nil, 2500, "online", "", at(t, loc, "2026-09-01 12:00")); err != nil {
		t.Fatal(err)
	}
	if err := AddDonation(conn, nil, 9999, "late", "", at(t, loc, "2026-10-15 12:00")); err != nil {
		t.Fatal(err)
	}

	s, err := RangeStats(conn, from, to)
	if err != nil {
		t.Fatal(err)
	}

	if s.Events != 2 {
		t.Errorf("Events = %d, want 2", s.Events)
	}
	if s.Attendance != 55 || s.AttendanceEvents != 2 {
		t.Errorf("Attendance = %d over %d events, want 55 over 2", s.Attendance, s.AttendanceEvents)
	}
	if s.DoorCents != 12000 {
		t.Errorf("DoorCents = %d, want 12000", s.DoorCents)
	}
	if s.DonationCents != 7500 || s.DonationCount != 2 {
		t.Errorf("Donations = %d over %d, want 7500 over 2", s.DonationCents, s.DonationCount)
	}
	if s.VolunteerRolesFilled != 1 {
		t.Errorf("VolunteerRolesFilled = %d, want 1", s.VolunteerRolesFilled)
	}
	if s.AvgAttendance() != "28" { // 55/2 = 27.5 -> 28
		t.Errorf("AvgAttendance = %q, want 28", s.AvgAttendance())
	}

	// By type: Music (A) with 40, Open Draw (B) with 15.
	byType := map[string]TypeStat{}
	for _, ts := range s.ByType {
		byType[ts.Type] = ts
	}
	if len(s.ByType) != 2 || byType["Music"].Attendance != 40 || byType["Open Draw"].Attendance != 15 {
		t.Errorf("ByType = %+v", s.ByType)
	}

	// Donations list for the period excludes the October gift.
	list, err := ListDonations(conn, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("ListDonations returned %d, want 2", len(list))
	}
}
