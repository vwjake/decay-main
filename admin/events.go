package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func registerEventRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/events", listEvents(conn))
	g.POST("/events", createEvent(conn))
	g.POST("/events/:id/delete", deleteEvent(conn))
	g.GET("/events/:id", editEvent(conn))
	g.POST("/events/:id/flyer", uploadFlyer(conn, uploadsDir))
	g.POST("/events/:id/volunteers", saveVolunteers(conn))
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

// editEvent is the per-event page where the flyer and volunteer roles are
// managed — the two things that don't fit in the create form.
func editEvent(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		ev, volunteers, err := loadEvent(conn, c.Param("id"))
		if err != nil {
			return err
		}
		return views.AdminEventEdit(ev, volunteers, "").Render(c.Request().Context(), c.Response())
	}
}

func uploadFlyer(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		ev, volunteers, err := loadEvent(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminEventEdit(ev, volunteers, msg).Render(c.Request().Context(), c.Response())
		}

		fileHeader, err := c.FormFile("flyer")
		if err != nil {
			return rerender("Choose an image to upload.")
		}
		filename, err := saveImage(fileHeader, filepath.Join(uploadsDir, flyersSubdir))
		if err != nil {
			if errors.Is(err, errNotAnImage) {
				return rerender("That file doesn't look like an image. Use jpg, png, gif, or webp.")
			}
			return err
		}

		previous, err := db.SetEventFlyer(conn, ev.ID, filename)
		if err != nil {
			return err
		}
		// Only remove the old file if nothing else still points at it —
		// recurring events share a flyer.
		if previous != "" && previous != filename {
			inUse, err := db.FlyerInUse(conn, previous)
			if err != nil {
				return err
			}
			if !inUse {
				_ = os.Remove(filepath.Join(uploadsDir, flyersSubdir, previous))
			}
		}
		return c.Redirect(http.StatusSeeOther, "/admin/events/"+c.Param("id"))
	}
}

func saveVolunteers(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}

		var roles []string
		for _, role := range db.VolunteerRoles {
			if c.FormValue("needed_"+role) != "" {
				roles = append(roles, role)
			}
		}
		if err := db.SetVolunteerRoles(conn, id, roles); err != nil {
			return err
		}
		// Names are saved for whichever roles are still marked needed.
		for _, role := range roles {
			if err := db.AssignVolunteer(conn, id, role, strings.TrimSpace(c.FormValue("name_"+role))); err != nil {
				return err
			}
		}
		return c.Redirect(http.StatusSeeOther, "/admin/events/"+c.Param("id"))
	}
}

func loadEvent(conn *sql.DB, rawID string) (db.Event, []db.EventVolunteer, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return db.Event{}, nil, echo.NewHTTPError(http.StatusBadRequest)
	}
	ev, err := db.EventByID(conn, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Event{}, nil, echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return db.Event{}, nil, err
	}
	volunteers, err := db.VolunteersFor(conn, id)
	if err != nil {
		return db.Event{}, nil, err
	}
	return ev, volunteers, nil
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
		id, err := db.CreateEvent(conn, ev)
		if err != nil {
			return err
		}
		// Volunteer roles come from checkboxes named the same as the roles.
		var roles []string
		for _, role := range db.VolunteerRoles {
			if c.FormValue("volunteer_"+role) != "" {
				roles = append(roles, role)
			}
		}
		if len(roles) > 0 {
			if err := db.SetVolunteerRoles(conn, id, roles); err != nil {
				return err
			}
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
