package admin

import (
	"time"

	"decay-main/staff"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// registerStaffRoutes wires the internal staff calendar. It's read straight
// from the shared Nextcloud .ics feed on each view (cached inside the
// client), so there's nothing to create or edit here — just the page.
func registerStaffRoutes(g *echo.Group, client *staff.Client, venue *time.Location) {
	g.GET("/staff", showStaff(client, venue))
}

func showStaff(client *staff.Client, venue *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		page := views.StaffPage{Configured: client.Configured()}

		if client.Configured() {
			all, err := client.Meetings()
			if err != nil {
				// A fetch failure still shows whatever was last cached, with
				// a note, rather than an empty error page.
				page.Error = "Couldn't reach the staff calendar just now — showing the last copy if there is one."
			}
			month := staff.ParseMonth(c.QueryParam("month"), venue)
			page.Month = staff.BuildMonth(all, month, venue)
			page.Upcoming = staff.Upcoming(all, time.Now(), 12)
			page.Venue = venue
		}

		return views.AdminStaff(page, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}
