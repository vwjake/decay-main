package admin

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// fullTimeLayout matches db's own layout so values round-trip cleanly.
const fullTimeLayout = "2006-01-02T15:04:05-07:00"

// Event start/end times are entered in the venue's own timezone (Pacific);
// there's one venue, so there's no need for a per-event timezone picker.
const pacificSuffix = ":00-07:00"

func registerEventRoutes(g *echo.Group, conn *sql.DB) {
	g.GET("/events", listEvents(conn))
	g.POST("/events", createEvent(conn))
	g.POST("/events/:id/delete", deleteEvent(conn))
}

func listEvents(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		events, err := db.ListAllEvents(conn)
		if err != nil {
			return err
		}
		return views.AdminEvents(events, "").Render(c.Request().Context(), c.Response())
	}
}

func createEvent(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		startsAt, err := parseAdminTime(c.FormValue("starts_at"))
		if err != nil {
			return rerenderEventsError(c, conn, "Invalid start time.")
		}

		var endsAt *time.Time
		if raw := c.FormValue("ends_at"); raw != "" {
			t, err := parseAdminTime(raw)
			if err != nil {
				return rerenderEventsError(c, conn, "Invalid end time.")
			}
			endsAt = &t
		}

		link := c.FormValue("link")
		if link == "" {
			link = "#"
		}

		ev := db.Event{
			Title:       c.FormValue("title"),
			EventType:   c.FormValue("event_type"),
			StartsAt:    startsAt,
			EndsAt:      endsAt,
			Location:    c.FormValue("location"),
			Description: c.FormValue("description"),
			Link:        link,
		}
		if ev.Title == "" || ev.EventType == "" {
			return rerenderEventsError(c, conn, "Title and type are required.")
		}
		if err := db.CreateEvent(conn, ev); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/events")
	}
}

func deleteEvent(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteEvent(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/events")
	}
}

// parseAdminTime turns a <input type="datetime-local"> value
// ("2026-07-25T16:00") into a time.Time in the venue's fixed offset.
func parseAdminTime(raw string) (time.Time, error) {
	return time.Parse(fullTimeLayout, raw+pacificSuffix)
}

func rerenderEventsError(c echo.Context, conn *sql.DB, msg string) error {
	events, err := db.ListAllEvents(conn)
	if err != nil {
		return err
	}
	return views.AdminEvents(events, msg).Render(c.Request().Context(), c.Response())
}
