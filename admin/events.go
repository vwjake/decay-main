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
	"decay-main/images"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// adminTimeLayout is the value a <input type="datetime-local"> submits.
const adminTimeLayout = "2006-01-02T15:04"

// Event times are entered in the venue's own timezone; there's one venue,
// so there's no per-event timezone picker. The zone has to be resolved
// against the event's own date rather than pinned to a fixed offset —
// half the year is PST and half is PDT, and pinning one silently shifts
// every event in the other half by an hour, including in the calendar
// feed people subscribe to.
var pacific = func() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic("loading venue timezone: " + err.Error())
	}
	return loc
}()

func registerEventRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/events", listEvents(conn))
	g.POST("/events", createEvent(conn))
	g.POST("/events/:id/delete", deleteEvent(conn))
	g.GET("/events/:id", editEvent(conn))
	g.POST("/events/:id", saveEvent(conn))
	g.POST("/events/:id/flyer", uploadFlyer(conn, uploadsDir))
	g.POST("/events/:id/volunteers", saveVolunteers(conn))
}

func listEvents(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		events, err := db.ListAllEvents(conn)
		if err != nil {
			return err
		}
		return views.AdminEvents(events, currentUser(c), "").Render(c.Request().Context(), c.Response())
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
		return views.AdminEventEdit(ev, volunteers, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

// saveEvent applies edits to an event's details. The event keeps its id
// and uid throughout, so a correction reaches subscribed calendars as an
// update to the event they already have rather than as a new one.
func saveEvent(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		ev, volunteers, err := loadEvent(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminEventEdit(ev, volunteers, currentUser(c), msg).Render(c.Request().Context(), c.Response())
		}

		title := strings.TrimSpace(c.FormValue("title"))
		eventType := strings.TrimSpace(c.FormValue("event_type"))
		if title == "" || eventType == "" {
			return rerender("Title and type are required.")
		}

		startsAt, err := parseAdminTime(c.FormValue("starts_at"))
		if err != nil {
			return rerender("Invalid start time.")
		}
		var endsAt *time.Time
		if raw := c.FormValue("ends_at"); raw != "" {
			t, err := parseAdminTime(raw)
			if err != nil {
				return rerender("Invalid end time.")
			}
			if !t.After(startsAt) {
				return rerender("The end time has to come after the start time.")
			}
			endsAt = &t
		}

		slug := sanitizeSlug(c.FormValue("slug"))
		if slug == "" {
			slug = ev.Slug
		}
		if slug != ev.Slug {
			taken, err := db.SlugTaken(conn, slug, ev.ID)
			if err != nil {
				return err
			}
			if taken {
				return rerender("Another event already uses that address.")
			}
		}

		link := strings.TrimSpace(c.FormValue("link"))
		if link == "" {
			link = "#"
		}

		updated := db.Event{
			ID:          ev.ID,
			Title:       title,
			EventType:   eventType,
			StartsAt:    startsAt,
			EndsAt:      endsAt,
			Location:    strings.TrimSpace(c.FormValue("location")),
			Description: c.FormValue("description"),
			Link:        link,
			Slug:        slug,
		}
		if err := db.UpdateEvent(conn, updated); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/events/"+c.Param("id"))
	}
}

func uploadFlyer(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		ev, volunteers, err := loadEvent(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminEventEdit(ev, volunteers, currentUser(c), msg).Render(c.Request().Context(), c.Response())
		}

		fileHeader, err := c.FormFile("flyer")
		if err != nil {
			return rerender("Choose an image to upload.")
		}
		flyerDir := filepath.Join(uploadsDir, flyersSubdir)
		filename, err := saveImage(fileHeader, flyerDir)
		if err != nil {
			if errors.Is(err, errNotAnImage) {
				return rerender("That file doesn't look like an image. Use jpg, png, gif, or webp.")
			}
			return err
		}
		if err := images.MakeWeb(
			filepath.Join(flyerDir, filename),
			filepath.Join(flyerDir, "web", images.WebName(filename)),
		); err != nil {
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
				_ = os.Remove(filepath.Join(flyerDir, previous))
				_ = os.Remove(filepath.Join(flyerDir, "web", images.WebName(previous)))
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
// ("2026-07-25T16:00") into an instant in the venue's timezone.
func parseAdminTime(raw string) (time.Time, error) {
	return time.ParseInLocation(adminTimeLayout, raw, pacific)
}

func rerenderEventsError(c echo.Context, conn *sql.DB, msg string) error {
	events, err := db.ListAllEvents(conn)
	if err != nil {
		return err
	}
	return views.AdminEvents(events, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}
