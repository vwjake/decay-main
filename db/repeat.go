package db

import (
	"database/sql"
	"fmt"
	"time"
)

// RepeatFrequency is one schedule the event duplicate tool offers.
type RepeatFrequency struct {
	Value string
	Label string
}

// RepeatFrequencies are the schedules offered on the edit page.
var RepeatFrequencies = []RepeatFrequency{
	{"weekly", "Every week"},
	{"biweekly", "Every 2 weeks"},
	{"every-4-weeks", "Every 4 weeks"},
	{"monthly", "Every month"},
}

// ValidRepeatFrequency reports whether freq is one we offer.
func ValidRepeatFrequency(freq string) bool {
	for _, f := range RepeatFrequencies {
		if f.Value == freq {
			return true
		}
	}
	return false
}

// RepeatDates returns the start times of count occurrences after start, spaced
// by freq. The times are anchored in loc (the venue timezone) so the
// wall-clock time is preserved across a daylight-saving change rather than
// drifting an hour — the stored times carry a fixed offset, so without
// re-anchoring a weekly repeat would keep the summer offset into the winter.
// Returns nil for an unknown frequency or a count below 1.
func RepeatDates(start time.Time, freq string, count int, loc *time.Location) []time.Time {
	if count < 1 || !ValidRepeatFrequency(freq) {
		return nil
	}
	start = start.In(loc)
	var out []time.Time
	for i := 1; i <= count; i++ {
		var d time.Time
		switch freq {
		case "weekly":
			d = start.AddDate(0, 0, 7*i)
		case "biweekly":
			d = start.AddDate(0, 0, 14*i)
		case "every-4-weeks":
			d = start.AddDate(0, 0, 28*i)
		case "monthly":
			d = start.AddDate(0, i, 0)
		}
		out = append(out, d)
	}
	return out
}

// RepeatEvent creates count copies of an event, spaced by freq after it. Each
// copy is an independent event with its own uid and slug, carrying the title,
// type, description, location, link, flyer, and the same volunteer roles (left
// unassigned). It does not copy who signed up, assigned names, or any reports
// or donations. The source and every copy share a series id — the source's
// own, if this isn't its first time being repeated, or a freshly assigned one
// otherwise — so the series page can find and sync them later. Returns the
// new ids in date order, and the series id they (and the source) now share.
func RepeatEvent(conn *sql.DB, sourceID int64, freq string, count int, loc *time.Location) ([]int64, int64, error) {
	src, err := EventByID(conn, sourceID)
	if err != nil {
		return nil, 0, err
	}
	dates := RepeatDates(src.StartsAt, freq, count, loc)
	if len(dates) == 0 {
		return nil, 0, fmt.Errorf("repeat: bad frequency %q or count %d", freq, count)
	}

	seriesID := src.SeriesID
	if seriesID == 0 {
		seriesID = src.ID
		if _, err := conn.Exec(`UPDATE events SET series_id = ? WHERE id = ?`, seriesID, src.ID); err != nil {
			return nil, 0, err
		}
	}

	// The roles the source needs, recreated as open (unassigned) on each copy.
	vols, err := VolunteersFor(conn, sourceID)
	if err != nil {
		return nil, 0, err
	}
	var roles []string
	for _, v := range vols {
		roles = append(roles, v.Role)
	}

	var dur time.Duration
	if src.EndsAt != nil {
		dur = src.EndsAt.Sub(src.StartsAt)
	}

	var ids []int64
	for _, d := range dates {
		dup := Event{
			Title:       src.Title,
			EventType:   src.EventType,
			StartsAt:    d,
			Location:    src.Location,
			Description: src.Description,
			Link:        src.Link,
			Flyer:       src.Flyer,
			SeriesID:    seriesID,
			// UID and Slug left blank so CreateEvent mints fresh unique ones.
		}
		if src.EndsAt != nil {
			end := d.Add(dur)
			dup.EndsAt = &end
		}
		id, err := CreateEvent(conn, dup)
		if err != nil {
			return ids, seriesID, err
		}
		if len(roles) > 0 {
			if err := SetVolunteerRoles(conn, id, roles); err != nil {
				return ids, seriesID, err
			}
		}
		ids = append(ids, id)
	}
	return ids, seriesID, nil
}

// UngroupedEvents returns every event not yet part of a series, soonest
// first — the pool the "group events into series" admin tool scans for
// repeats that predate series tracking.
func UngroupedEvents(conn *sql.DB) ([]Event, error) {
	rows, err := conn.Query(`SELECT ` + eventColumns + ` FROM events WHERE series_id = 0 ORDER BY starts_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// GroupIntoSeries links every event in ids into one series, the same way
// Repeat links a source event to its copies — sharing the id of whichever
// event in the group is oldest. Every id must not already belong to a
// series; the caller (the admin grouping tool) is responsible for checking
// that, same as it's responsible for deciding the events actually belong
// together.
func GroupIntoSeries(conn *sql.DB, ids []int64) (int64, error) {
	if len(ids) < 2 {
		return 0, fmt.Errorf("group into series: need at least 2 events, got %d", len(ids))
	}
	seriesID := ids[0]
	for _, id := range ids[1:] {
		if id < seriesID {
			seriesID = id
		}
	}
	tx, err := conn.Begin()
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := tx.Exec(`UPDATE events SET series_id = ? WHERE id = ?`, seriesID, id); err != nil {
			tx.Rollback()
			return 0, err
		}
	}
	return seriesID, tx.Commit()
}
