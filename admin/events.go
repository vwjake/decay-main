package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"decay-main/bookingmail"
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

func registerEventRoutes(g *echo.Group, conn *sql.DB, uploadsDir string, bookingMailer *bookingmail.Handler, venue *time.Location) {
	g.GET("/events", listEvents(conn))
	g.POST("/events", createEvent(conn))
	g.POST("/events/:id/delete", deleteEvent(conn))
	g.GET("/events/:id", editEvent(conn, bookingMailer, venue))
	g.POST("/events/:id", saveEvent(conn, bookingMailer, venue))
	g.POST("/events/:id/flyer", uploadFlyer(conn, uploadsDir, bookingMailer, venue))
	g.POST("/events/:id/volunteers", saveVolunteers(conn))
	g.POST("/events/:id/repeat", repeatEvent(conn, bookingMailer, venue))
	g.POST("/events/:id/signups/:signupID/delete", deleteSignup(conn))
	g.POST("/events/:id/reply/preview", previewEventReply(conn, bookingMailer))
	g.POST("/events/:id/reply/send", sendEventReply(conn, bookingMailer))
}

// deleteSignup removes a volunteer offer once it's been handled.
func deleteSignup(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		signupID, err := strconv.ParseInt(c.Param("signupID"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteVolunteerSignup(conn, signupID); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/events/"+c.Param("id"))
	}
}

func listEvents(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		return renderEvents(c, conn, "")
	}
}

// renderEvents draws one page of the event table, keeping the ?page= the
// request arrived on so a validation error doesn't bounce back to page 1.
func renderEvents(c echo.Context, conn *sql.DB, msg string) error {
	// Upcoming only, soonest first — same starting point as the homepage.
	// Past events live on the calendar and (soon) behind a filter here.
	events, err := db.UpcomingEvents(conn)
	if err != nil {
		return err
	}
	events, page := db.Paginate(events, db.PageNumber(c.QueryParam("page")), db.PerPageAdmin)
	page.Path = "/admin/events"
	users, err := db.ListUsers(conn)
	if err != nil {
		return err
	}
	var prefill *db.BookingRequest
	if raw := c.QueryParam("from_booking"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			if b, err := db.BookingByID(conn, id); err == nil {
				prefill = &b
			}
		}
	}
	return views.AdminEvents(events, page, users, prefill, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}

// eventDetailData bundles an event for AdminEventEdit, looking up its email
// correspondence the same way viewBooking does for a booking. refresh
// forces a fresh IMAP read past the cache.
func eventDetailData(bookingMailer *bookingmail.Handler, venue *time.Location, ev db.Event, volunteers []db.EventVolunteer, signups []db.VolunteerSignup, users []db.User, refresh bool) views.EventDetailData {
	var thread bookingmail.Thread
	if ev.Email != "" {
		thread = bookingMailer.Thread([]string{ev.Email}, refresh)
	}
	return views.EventDetailData{
		Event:          ev,
		Volunteers:     volunteers,
		Signups:        signups,
		Users:          users,
		Thread:         thread,
		CanSend:        bookingMailer.CanSend(),
		MailboxAddress: bookingMailer.Address(),
		Venue:          venue,
	}
}

// editEvent is the per-event page where the flyer, volunteer roles, and
// organizer correspondence are managed — the things that don't fit in the
// create form.
func editEvent(conn *sql.DB, bookingMailer *bookingmail.Handler, venue *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		ev, volunteers, signups, users, err := loadEvent(conn, c.Param("id"))
		if err != nil {
			return err
		}
		data := eventDetailData(bookingMailer, venue, ev, volunteers, signups, users, c.QueryParam("refresh") == "1")
		if c.QueryParam("sent") == "1" {
			data.FlashNotice = "Reply sent."
			if c.QueryParam("warn") == "1" {
				data.FlashNotice += " It may take a moment to show up below — the copy couldn't be filed in Sent right away."
			}
		}
		if c.QueryParam("send_error") != "" {
			data.FlashError = replyErrorMessage(c.QueryParam("send_error"))
		}
		return views.AdminEventEdit(data, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}

// saveEvent applies edits to an event's details. The event keeps its id
// and uid throughout, so a correction reaches subscribed calendars as an
// update to the event they already have rather than as a new one.
func saveEvent(conn *sql.DB, bookingMailer *bookingmail.Handler, venue *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		ev, volunteers, signups, users, err := loadEvent(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			data := eventDetailData(bookingMailer, venue, ev, volunteers, signups, users, false)
			data.ErrorMsg = msg
			return views.AdminEventEdit(data, currentUser(c)).Render(c.Request().Context(), c.Response())
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
			Keyholder:   strings.TrimSpace(c.FormValue("keyholder")),
			ContactName: strings.TrimSpace(c.FormValue("contact_name")),
			Email:       strings.TrimSpace(c.FormValue("email")),
		}
		if err := db.UpdateEvent(conn, updated); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/events/"+c.Param("id"))
	}
}

func uploadFlyer(conn *sql.DB, uploadsDir string, bookingMailer *bookingmail.Handler, venue *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		ev, volunteers, signups, users, err := loadEvent(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			data := eventDetailData(bookingMailer, venue, ev, volunteers, signups, users, false)
			data.ErrorMsg = msg
			return views.AdminEventEdit(data, currentUser(c)).Render(c.Request().Context(), c.Response())
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

func loadEvent(conn *sql.DB, rawID string) (db.Event, []db.EventVolunteer, []db.VolunteerSignup, []db.User, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return db.Event{}, nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest)
	}
	ev, err := db.EventByID(conn, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Event{}, nil, nil, nil, echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return db.Event{}, nil, nil, nil, err
	}
	volunteers, err := db.VolunteersFor(conn, id)
	if err != nil {
		return db.Event{}, nil, nil, nil, err
	}
	signups, err := db.SignupsForEvent(conn, id)
	if err != nil {
		return db.Event{}, nil, nil, nil, err
	}
	users, err := db.ListUsers(conn)
	if err != nil {
		return db.Event{}, nil, nil, nil, err
	}
	return ev, volunteers, signups, users, nil
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
			Keyholder:   strings.TrimSpace(c.FormValue("keyholder")),
			ContactName: strings.TrimSpace(c.FormValue("contact_name")),
			Email:       strings.TrimSpace(c.FormValue("email")),
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

		// Reached via "Convert to event" on a booking request: go straight
		// to the new event's own page to finish the flyer, volunteers, and
		// keyholder rather than back to the list.
		if c.QueryParam("from_booking") != "" {
			return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/admin/events/%d", id))
		}
		return c.Redirect(http.StatusSeeOther, "/admin/events")
	}
}

// repeatEvent stamps out copies of an event on a schedule — a lightweight
// stand-in for calendar recurrence. Each copy is an ordinary, independently
// editable event, so there's no recurrence rule to maintain.
func repeatEvent(conn *sql.DB, bookingMailer *bookingmail.Handler, venue *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		ev, volunteers, signups, users, err := loadEvent(conn, c.Param("id"))
		if err != nil {
			return err
		}
		rerender := func(msg string) error {
			data := eventDetailData(bookingMailer, venue, ev, volunteers, signups, users, false)
			data.ErrorMsg = msg
			return views.AdminEventEdit(data, currentUser(c)).Render(c.Request().Context(), c.Response())
		}

		freq := c.FormValue("frequency")
		if !db.ValidRepeatFrequency(freq) {
			return rerender("Pick how often it repeats.")
		}
		count, err := strconv.Atoi(strings.TrimSpace(c.FormValue("count")))
		if err != nil || count < 1 {
			return rerender("How many copies? Enter a whole number of 1 or more.")
		}
		if count > 52 {
			count = 52
		}

		if _, err := db.RepeatEvent(conn, ev.ID, freq, count, pacific); err != nil {
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
// ("2026-07-25T16:00") into an instant in the venue's timezone.
func parseAdminTime(raw string) (time.Time, error) {
	return time.ParseInLocation(adminTimeLayout, raw, pacific)
}

func rerenderEventsError(c echo.Context, conn *sql.DB, msg string) error {
	return renderEvents(c, conn, msg)
}

func eventPath(id int64) string {
	return "/admin/events/" + strconv.FormatInt(id, 10)
}

// previewEventReply shows the admin the exact message a send will produce
// and issues a single-use confirmation token, mirroring the booking reply
// flow (see admin/bookings.go) for an event's organizer contact instead.
func previewEventReply(conn *sql.DB, bookingMailer *bookingmail.Handler) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		ev, err := db.EventByID(conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}

		subject := strings.TrimSpace(c.FormValue("subject"))
		body := strings.TrimSpace(c.FormValue("body"))
		if subject == "" || body == "" {
			return c.Redirect(http.StatusSeeOther, eventPath(ev.ID)+"?send_error=empty")
		}

		// Reply into the existing thread when there is one, so the
		// recipient's mail client keeps it in the same conversation.
		inReplyTo := ""
		if ev.Email != "" {
			thread := bookingMailer.Thread([]string{ev.Email}, false)
			if len(thread.Messages) > 0 {
				inReplyTo = thread.Messages[len(thread.Messages)-1].MessageID
			}
		}

		nonce, err := newNonce()
		if err != nil {
			return err
		}
		sess := getSession(c)
		sess.Values["event_reply_nonce"] = nonce
		if err := sess.Save(c.Request(), c.Response()); err != nil {
			return err
		}

		data := views.ReplyPreviewData{
			Kind:           "events",
			ID:             ev.ID,
			RecipientName:  ev.ContactName,
			RecipientEmail: ev.Email,
			MailboxAddress: bookingMailer.Address(),
			FromName:       "DECAY Events",
			Subject:        subject,
			Body:           body,
			SenderName:     currentUser(c).Name(),
			InReplyTo:      inReplyTo,
			Nonce:          nonce,
		}
		return views.AdminEmailReplyPreview(data, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}

// sendEventReply performs the actual send. The nonce is single-use, same as
// sendBookingReply, and kept under its own session key so an event reply
// and a booking reply open in different tabs can't confirm one another's.
func sendEventReply(conn *sql.DB, bookingMailer *bookingmail.Handler) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		ev, err := db.EventByID(conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}

		sess := getSession(c)
		expected, _ := sess.Values["event_reply_nonce"].(string)
		delete(sess.Values, "event_reply_nonce")
		_ = sess.Save(c.Request(), c.Response())

		supplied := c.FormValue("nonce")
		if expected == "" || supplied == "" || expected != supplied {
			return c.Redirect(http.StatusSeeOther, eventPath(ev.ID)+"?send_error=expired")
		}

		res, err := bookingMailer.Send(bookingmail.ReplyInput{
			To:         ev.Email,
			ToName:     ev.ContactName,
			Subject:    c.FormValue("subject"),
			Body:       c.FormValue("body"),
			SenderName: currentUser(c).Name(),
			InReplyTo:  c.FormValue("in_reply_to"),
		})
		if err != nil {
			log.Printf("event #%d reply to %s failed: %v", ev.ID, ev.Email, err)
			return c.Redirect(http.StatusSeeOther, eventPath(ev.ID)+"?send_error=1")
		}
		log.Printf("event #%d reply sent to %s by %s", ev.ID, ev.Email, currentUser(c).Name())

		redirect := eventPath(ev.ID) + "?sent=1"
		if res.Warning != "" {
			redirect += "&warn=1"
		}
		return c.Redirect(http.StatusSeeOther, redirect)
	}
}
