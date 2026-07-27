package admin

import (
	"time"

	"decay-main/meetings"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// registerMeetingRoutes wires the internal meetings calendar. It's read
// straight from the shared Nextcloud .ics feed on each view (cached inside
// the client), so there's nothing to create or edit here — just the page.
func registerMeetingRoutes(g *echo.Group, client *meetings.Client, venue *time.Location) {
	g.GET("/meetings", showMeetings(client, venue))
}

func showMeetings(client *meetings.Client, venue *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		page := views.MeetingsPage{Configured: client.Configured()}

		if client.Configured() {
			all, err := client.Meetings()
			if err != nil {
				// A fetch failure still shows whatever was last cached, with
				// a note, rather than an empty error page.
				page.Error = "Couldn't reach the internal calendar just now — showing the last copy if there is one."
			}
			month := meetings.ParseMonth(c.QueryParam("month"), venue)
			page.Month = meetings.BuildMonth(all, month, venue)
			page.Upcoming = meetings.Upcoming(all, time.Now(), 12)
			page.Venue = venue
		}

		return views.AdminMeetings(page, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}
