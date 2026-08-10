package admin

import (
	"database/sql"
	"strconv"
	"time"

	"decay-main/db"
	"decay-main/staff"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// registerStaffRoutes wires the admin calendar: events and booking requests
// come off the database, staff meetings are read straight from the shared
// Nextcloud .ics feed on each view (cached inside the client) — nothing to
// create or edit here, just the page.
func registerStaffRoutes(g *echo.Group, conn *sql.DB, client *staff.Client, venue *time.Location) {
	g.GET("/staff", showStaff(conn, client, venue))
}

func showStaff(conn *sql.DB, client *staff.Client, venue *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		me := currentUser(c)
		page := views.StaffPage{Configured: client.Configured(), Venue: venue}

		month := db.ParseMonth(c.QueryParam("month"), venue)

		var meetings []staff.Meeting
		if client.Configured() {
			all, err := client.Meetings()
			if err != nil {
				// A fetch failure still shows whatever was last cached, with
				// a note, rather than an empty error page.
				page.Error = "Couldn't reach the staff calendar just now — showing the last copy if there is one."
			}
			meetings = all
		}

		events, err := db.EventsInMonth(conn, month)
		if err != nil {
			return err
		}
		requests, err := db.ListBookingRequests(conn)
		if err != nil {
			return err
		}

		page.Calendar = buildAdminCalendar(events, requests, meetings, me.Can(db.PermBookings), month, venue)

		return views.AdminStaff(page, me).Render(c.Request().Context(), c.Response())
	}
}

// buildAdminCalendar lays events, booking requests (those with a
// parseable preferred date), and staff meetings out on one shared month
// grid. Requests and meetings are passed in unfiltered (the full queue,
// the full feed) — only the ones that land in this month's date range
// count or show, same as the events grid always worked.
func buildAdminCalendar(events []db.Event, requests []db.BookingRequest, meetings []staff.Meeting, canViewBookings bool, month time.Time, loc *time.Location) views.AdminCalMonth {
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0)
	inMonth := func(t time.Time) bool { return !t.Before(start) && t.Before(end) }

	byDay := map[string][]views.CalEntry{}
	count := 0

	for _, ev := range events {
		t := ev.StartsAt.In(loc)
		key := t.Format("2006-01-02")
		byDay[key] = append(byDay[key], views.CalEntry{
			Category: views.CalEvent,
			Time:     ev.StartClock(),
			Title:    ev.Title,
			Href:     ev.Path(),
			Linkable: true,
		})
		if inMonth(t) {
			count++
		}
	}

	for _, r := range requests {
		t, ok := db.ParsePreferredDate(r.PreferredDate, loc)
		if !ok {
			continue
		}
		title := r.EventName
		if title == "" {
			title = r.Name
		}
		key := t.Format("2006-01-02")
		byDay[key] = append(byDay[key], views.CalEntry{
			Category: views.CalRequest,
			Title:    title,
			Href:     "/admin/bookings/" + strconv.FormatInt(r.ID, 10),
			Linkable: canViewBookings,
		})
		if inMonth(t) {
			count++
		}
	}

	for _, m := range meetings {
		t := m.Start.In(loc)
		key := t.Format("2006-01-02")
		byDay[key] = append(byDay[key], views.CalEntry{
			Category: views.CalStaff,
			Time:     m.StartClock(),
			Title:    m.Title(),
		})
		if inMonth(t) {
			count++
		}
	}

	today := time.Now().In(loc).Format("2006-01-02")
	cursor := start.AddDate(0, 0, -int(start.Weekday()))

	var weeks [][]views.AdminCalDay
	for cursor.Before(end) {
		week := make([]views.AdminCalDay, 7)
		for i := range week {
			key := cursor.Format("2006-01-02")
			week[i] = views.AdminCalDay{
				Date:    cursor,
				InMonth: cursor.Month() == start.Month() && cursor.Year() == start.Year(),
				Today:   key == today,
				Entries: byDay[key],
			}
			cursor = cursor.AddDate(0, 0, 1)
		}
		weeks = append(weeks, week)
	}

	return views.AdminCalMonth{
		Month: start,
		Weeks: weeks,
		Prev:  start.AddDate(0, -1, 0).Format("2006-01"),
		Next:  start.AddDate(0, 1, 0).Format("2006-01"),
		Count: count,
	}
}
