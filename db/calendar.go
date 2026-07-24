package db

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CalendarDay is one cell of the month grid.
type CalendarDay struct {
	Date time.Time
	// InMonth is false for the leading and trailing cells that pad the
	// grid out to whole weeks.
	InMonth bool
	Today   bool
	Events  []Event
}

func (d CalendarDay) Day() string { return strconv.Itoa(d.Date.Day()) }

// CalendarMonth is a month laid out as whole weeks, Sunday first, the way
// the old site's calendar read.
type CalendarMonth struct {
	Month time.Time
	Weeks [][]CalendarDay
	// Prev and Next are "2006-01" values for the navigation links.
	Prev string
	Next string
	// Count is how many events fall in the month.
	Count int
}

func (m CalendarMonth) Label() string { return m.Month.Format("January 2006") }

// Weekdays are the column headings. Single letters keep the columns
// narrow enough to fit a phone.
var Weekdays = []string{"S", "M", "T", "W", "T", "F", "S"}

// ParseMonth reads a "2006-01" value, falling back to the current month
// in the venue's timezone when it's missing or malformed.
func ParseMonth(raw string, loc *time.Location) time.Time {
	if t, err := time.ParseInLocation("2006-01", strings.TrimSpace(raw), loc); err == nil {
		return t
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
}

// EventsInMonth returns every event starting in the given month, soonest
// first.
func EventsInMonth(conn *sql.DB, month time.Time) ([]Event, error) {
	all, err := ListAllEvents(conn)
	if err != nil {
		return nil, err
	}

	start := monthStart(month)
	end := start.AddDate(0, 1, 0)

	var events []Event
	// ListAllEvents is newest first; walk it backwards for soonest first.
	for i := len(all) - 1; i >= 0; i-- {
		at := all[i].StartsAt
		if !at.Before(start) && at.Before(end) {
			events = append(events, all[i])
		}
	}
	return events, nil
}

// BuildCalendar lays events out on a month grid. Events must already be
// sorted by start time; each day keeps that order.
func BuildCalendar(events []Event, month time.Time, loc *time.Location) CalendarMonth {
	start := monthStart(month)

	byDay := map[string][]Event{}
	for _, ev := range events {
		key := ev.StartsAt.In(loc).Format("2006-01-02")
		byDay[key] = append(byDay[key], ev)
	}

	today := time.Now().In(loc).Format("2006-01-02")

	// Back up to the Sunday on or before the first of the month, then run
	// forward in whole weeks until the month is covered.
	cursor := start.AddDate(0, 0, -int(start.Weekday()))
	end := start.AddDate(0, 1, 0)

	var weeks [][]CalendarDay
	for cursor.Before(end) {
		week := make([]CalendarDay, 7)
		for i := range week {
			key := cursor.Format("2006-01-02")
			week[i] = CalendarDay{
				Date:    cursor,
				InMonth: cursor.Month() == start.Month() && cursor.Year() == start.Year(),
				Today:   key == today,
				Events:  byDay[key],
			}
			cursor = cursor.AddDate(0, 0, 1)
		}
		weeks = append(weeks, week)
	}

	return CalendarMonth{
		Month: start,
		Weeks: weeks,
		Prev:  start.AddDate(0, -1, 0).Format("2006-01"),
		Next:  start.AddDate(0, 1, 0).Format("2006-01"),
		Count: len(events),
	}
}

// MonthURL builds a link to another month of the calendar.
func MonthURL(month string) string { return "/calendar?month=" + month }

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// ShortTitle trims a title to fit a calendar cell without wrapping into a
// wall of text. The old site cut at 20 characters and so does this.
func ShortTitle(title string) string {
	const limit = 20
	runes := []rune(title)
	if len(runes) <= limit {
		return title
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

// StartClock is the compact time shown next to a title in a cell, e.g.
// "7:30p".
func (e Event) StartClock() string {
	t := e.StartsAt
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
