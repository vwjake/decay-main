package db

import (
	"path/filepath"
	"testing"
	"time"
)

func venueTZ(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("venue timezone unavailable: %v", err)
	}
	return loc
}

func TestRepeatDatesWeekly(t *testing.T) {
	loc := venueTZ(t)
	start := time.Date(2026, 8, 1, 19, 0, 0, 0, loc) // a 7pm Saturday
	dates := RepeatDates(start, "weekly", 3, loc)
	if len(dates) != 3 {
		t.Fatalf("got %d dates, want 3", len(dates))
	}
	for i, d := range dates {
		wantDay := 8 + i*7 // Aug 8, 15, 22
		if d.Day() != wantDay || d.Month() != time.August {
			t.Errorf("date %d = %s, want Aug %d", i, d.Format("Jan 2"), wantDay)
		}
		if d.Hour() != 19 {
			t.Errorf("date %d hour = %d, want 19 (wall-clock preserved)", i, d.Hour())
		}
	}
}

func TestRepeatDatesMonthlyAndBiweekly(t *testing.T) {
	loc := venueTZ(t)
	start := time.Date(2026, 1, 15, 18, 30, 0, 0, loc)
	if got := RepeatDates(start, "monthly", 2, loc); got[0].Month() != time.February || got[1].Month() != time.March {
		t.Errorf("monthly = %v, want Feb then Mar", got)
	}
	if got := RepeatDates(start, "biweekly", 1, loc); got[0].Day() != 29 || got[0].Month() != time.January {
		t.Errorf("biweekly = %s, want Jan 29", got[0].Format("Jan 2"))
	}
	// Unknown frequency and non-positive counts yield nothing.
	if RepeatDates(start, "yearly", 3, loc) != nil {
		t.Error("unknown frequency should return nil")
	}
	if RepeatDates(start, "weekly", 0, loc) != nil {
		t.Error("count 0 should return nil")
	}
}

// TestRepeatDatesAcrossDST is the case the timezone handling exists for: a
// weekly event that crosses the November DST change must keep its wall-clock
// time (7pm), which means its UTC offset flips from -07:00 to -08:00.
func TestRepeatDatesAcrossDST(t *testing.T) {
	loc := venueTZ(t)
	start := time.Date(2026, 10, 25, 19, 0, 0, 0, loc) // PDT (-07:00)
	dates := RepeatDates(start, "weekly", 1, loc)      // Nov 1, after DST ends
	got := dates[0]
	if got.Hour() != 19 {
		t.Errorf("hour = %d, want 19 (wall-clock preserved across DST)", got.Hour())
	}
	_, offset := got.Zone()
	if offset != -8*3600 {
		t.Errorf("offset = %ds, want -28800 (PST) after DST ends", offset)
	}
}

func TestRepeatEvent(t *testing.T) {
	loc := venueTZ(t)
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	start := time.Date(2026, 8, 1, 19, 0, 0, 0, loc)
	end := start.Add(3 * time.Hour)
	srcID, err := CreateEvent(conn, Event{
		Title: "Open Draw", EventType: "Drawing", StartsAt: start, EndsAt: &end,
		Location: "The space", Description: "Weekly figure drawing", Flyer: "draw.jpg",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Source needs two roles; one already covered.
	if err := SetVolunteerRoles(conn, srcID, []string{"door", "cleanup"}); err != nil {
		t.Fatal(err)
	}
	if err := AssignVolunteer(conn, srcID, "door", "Ada"); err != nil {
		t.Fatal(err)
	}

	ids, seriesID, err := RepeatEvent(conn, srcID, "weekly", 2, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("created %d copies, want 2", len(ids))
	}
	if seriesID != srcID {
		t.Errorf("series id = %d, want the source's own id %d (its first time repeating)", seriesID, srcID)
	}

	src, _ := EventByID(conn, srcID)
	if src.SeriesID != seriesID {
		t.Errorf("source series id = %d, want %d", src.SeriesID, seriesID)
	}
	for i, id := range ids {
		cp, err := EventByID(conn, id)
		if err != nil {
			t.Fatal(err)
		}
		// Details carried over, flyer shared.
		if cp.Title != src.Title || cp.Flyer != src.Flyer || cp.Description != src.Description {
			t.Errorf("copy %d didn't carry details: %+v", i, cp)
		}
		if cp.SeriesID != seriesID {
			t.Errorf("copy %d series id = %d, want %d", i, cp.SeriesID, seriesID)
		}
		// Fresh, distinct identity.
		if cp.UID == "" || cp.UID == src.UID {
			t.Errorf("copy %d uid = %q, want a fresh one distinct from source", i, cp.UID)
		}
		if cp.Slug == "" || cp.Slug == src.Slug {
			t.Errorf("copy %d slug = %q, want a fresh one distinct from source", i, cp.Slug)
		}
		// Date shifted; length preserved.
		if cp.StartsAt.Day() != 8+i*7 {
			t.Errorf("copy %d starts %s, want Aug %d", i, cp.StartsAt.Format("Jan 2"), 8+i*7)
		}
		if cp.EndsAt == nil || cp.EndsAt.Sub(cp.StartsAt) != 3*time.Hour {
			t.Errorf("copy %d length wrong: %v", i, cp.EndsAt)
		}
		// Roles copied but unassigned — each occurrence starts open.
		vols, _ := VolunteersFor(conn, id)
		if len(vols) != 2 {
			t.Errorf("copy %d has %d roles, want 2", i, len(vols))
		}
		for _, v := range vols {
			if v.Name != "" {
				t.Errorf("copy %d role %q came over assigned to %q, want open", i, v.Role, v.Name)
			}
		}
	}

	// Source is untouched — still has Ada on the door.
	if vols, _ := VolunteersFor(conn, srcID); volunteerNamed(vols, "door") != "Ada" {
		t.Error("source event's assignment was disturbed")
	}

	// Repeating one of the copies (already in the series) joins the same
	// group rather than starting a new one.
	moreIDs, moreSeriesID, err := RepeatEvent(conn, ids[0], "weekly", 1, loc)
	if err != nil {
		t.Fatal(err)
	}
	if moreSeriesID != seriesID {
		t.Errorf("repeating a series member started series %d, want the existing %d", moreSeriesID, seriesID)
	}
	more, err := EventByID(conn, moreIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if more.SeriesID != seriesID {
		t.Errorf("new copy series id = %d, want %d", more.SeriesID, seriesID)
	}

	all, err := EventsInSeries(conn, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Errorf("series has %d events, want 4 (source + 2 + 1 more)", len(all))
	}
}

func volunteerNamed(vols []EventVolunteer, role string) string {
	for _, v := range vols {
		if v.Role == role {
			return v.Name
		}
	}
	return ""
}
