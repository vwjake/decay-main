package admin

import (
	"database/sql"
	"time"

	"decay-main/db"
	"decay-main/staff"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// registerStaffRoutes wires the internal staff calendar. The meetings grid
// is read straight from the shared Nextcloud .ics feed on each view (cached
// inside the client), so there's nothing to create or edit here — just the
// page. The site's own events are read from the database and shown on a
// second grid below it, so what the org has planned and what the public
// sees can be read against each other in one place.
func registerStaffRoutes(g *echo.Group, conn *sql.DB, client *staff.Client, venue *time.Location) {
	g.GET("/staff", showStaff(conn, client, venue))
}

func showStaff(conn *sql.DB, client *staff.Client, venue *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		page := views.StaffPage{Configured: client.Configured(), Venue: venue}

		// One month drives both grids, so paging moves them together.
		month := staff.ParseMonth(c.QueryParam("month"), venue)

		if client.Configured() {
			all, err := client.Meetings()
			if err != nil {
				// A fetch failure still shows whatever was last cached, with
				// a note, rather than an empty error page.
				page.Error = "Couldn't reach the staff calendar just now — showing the last copy if there is one."
			}
			page.Month = staff.BuildMonth(all, month, venue)
			page.Upcoming = staff.Upcoming(all, time.Now(), 12)
		}

		// The events grid comes off the database, so it renders whether or
		// not Nextcloud is configured or reachable.
		events, err := db.EventsInMonth(conn, month)
		if err != nil {
			return err
		}
		page.Events = db.BuildCalendar(events, month, venue)

		return views.AdminStaff(page, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}
