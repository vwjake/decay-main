package staff

import (
	"fmt"
	"strconv"
	"time"
)

// Day is one cell of the meetings month grid.
type Day struct {
	Date     time.Time
	InMonth  bool
	Today    bool
	Meetings []Meeting
}

func (d Day) Number() string { return strconv.Itoa(d.Date.Day()) }

// Month is the internal calendar laid out as whole weeks, Sunday first, to
// match the public calendar grid.
type Month struct {
	Month time.Time
	Weeks [][]Day
	// Prev and Next are "2006-01" values for the month navigation.
	Prev  string
	Next  string
	Count int
}

func (m Month) Label() string { return m.Month.Format("January 2006") }

// ParseMonth reads a "2006-01" value, falling back to the current month in
// the given zone when it's missing or malformed.
func ParseMonth(raw string, loc *time.Location) time.Time {
	if t, err := time.ParseInLocation("2006-01", raw, loc); err == nil {
		return t
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
}

// BuildMonth lays meetings out on a month grid in the given zone. A meeting
// lands on the day its start falls on locally.
func BuildMonth(all []Meeting, month time.Time, loc *time.Location) Month {
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0)

	byDay := map[string][]Meeting{}
	count := 0
	for _, m := range all {
		key := m.Start.In(loc).Format("2006-01-02")
		byDay[key] = append(byDay[key], m)
		if !m.Start.Before(start) && m.Start.Before(end) {
			count++
		}
	}

	today := time.Now().In(loc).Format("2006-01-02")

	cursor := start.AddDate(0, 0, -int(start.Weekday()))
	var weeks [][]Day
	for cursor.Before(end) {
		week := make([]Day, 7)
		for i := range week {
			key := cursor.Format("2006-01-02")
			week[i] = Day{
				Date:     cursor,
				InMonth:  cursor.Month() == start.Month() && cursor.Year() == start.Year(),
				Today:    key == today,
				Meetings: byDay[key],
			}
			cursor = cursor.AddDate(0, 0, 1)
		}
		weeks = append(weeks, week)
	}

	return Month{
		Month: start,
		Weeks: weeks,
		Prev:  start.AddDate(0, -1, 0).Format("2006-01"),
		Next:  start.AddDate(0, 1, 0).Format("2006-01"),
		Count: count,
	}
}

// StartClock is the compact time shown in a cell, e.g. "7:30p"; an all-day
// entry shows "all day".
func (m Meeting) StartClock() string {
	if m.AllDay {
		return "all day"
	}
	t := m.Start
	suffix := "a"
	if t.Hour() >= 12 {
		suffix = "p"
	}
	hour := t.Hour() % 12
	if hour == 0 {
		hour = 12
	}
	if t.Minute() == 0 {
		return fmt.Sprintf("%d%s", hour, suffix)
	}
	return fmt.Sprintf("%d:%02d%s", hour, t.Minute(), suffix)
}

// DayLabel is the date shown in the upcoming list, e.g. "Sun, Jul 26".
func (m Meeting) DayLabel(loc *time.Location) string {
	return m.Start.In(loc).Format("Mon, Jan 2")
}

// TimeRange is the span shown in the upcoming list. An all-day meeting says
// so; otherwise it's start–end, or just the start when there's no distinct
// end.
func (m Meeting) TimeRange(loc *time.Location) string {
	if m.AllDay {
		return "All day"
	}
	start := clock(m.Start.In(loc))
	if m.End.IsZero() || m.End.Equal(m.Start) {
		return start
	}
	return start + "–" + clock(m.End.In(loc))
}

func clock(t time.Time) string {
	suffix := "am"
	if t.Hour() >= 12 {
		suffix = "pm"
	}
	hour := t.Hour() % 12
	if hour == 0 {
		hour = 12
	}
	if t.Minute() == 0 {
		return fmt.Sprintf("%d%s", hour, suffix)
	}
	return fmt.Sprintf("%d:%02d%s", hour, t.Minute(), suffix)
}

// Title returns the meeting's summary, or a placeholder when the source
// entry has none, so a cell is never blank.
func (m Meeting) Title() string {
	if m.Summary == "" {
		return "(untitled)"
	}
	return m.Summary
}
