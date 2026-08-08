package admin

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"decay-main/bookingmail"
	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

func registerBookingRoutes(g *echo.Group, conn *sql.DB, bookingMailer *bookingmail.Handler, venue *time.Location) {
	g.GET("/bookings", listBookings(conn))
	g.GET("/bookings/:id", viewBooking(conn, bookingMailer, venue))
	g.POST("/bookings/:id/status", setBookingStatus(conn))
	g.POST("/bookings/:id/delete", deleteBooking(conn))
	g.POST("/bookings/:id/reply/preview", previewBookingReply(conn, bookingMailer))
	g.POST("/bookings/:id/reply/send", sendBookingReply(conn, bookingMailer))
}

func listBookings(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		showArchived := c.QueryParam("archived") != ""
		requests, err := db.ListBookingRequests(conn, showArchived)
		if err != nil {
			return err
		}
		return views.AdminBookings(requests, showArchived, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

// validBookingStatuses guards the status write to the known set, so a
// hand-crafted form can't set an arbitrary value.
var validBookingStatuses = map[string]bool{
	db.BookingNew: true, db.BookingReviewed: true, db.BookingArchived: true,
}

func setBookingStatus(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		status := c.FormValue("status")
		if !validBookingStatuses[status] {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.SetBookingStatus(conn, id, status); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/bookings")
	}
}

func deleteBooking(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteBookingRequest(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/bookings")
	}
}

// bookingIDParam parses :id, the same way every booking route needs to.
func bookingIDParam(c echo.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

// loadBooking fetches a booking by :id, translating "not found" into a 404
// so a bad or stale link fails cleanly instead of panicking downstream.
func loadBooking(c echo.Context, conn *sql.DB) (db.BookingRequest, error) {
	id, err := bookingIDParam(c)
	if err != nil {
		return db.BookingRequest{}, echo.NewHTTPError(http.StatusBadRequest)
	}
	b, err := db.BookingByID(conn, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.BookingRequest{}, echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return db.BookingRequest{}, err
	}
	return b, nil
}

func viewBooking(conn *sql.DB, bookingMailer *bookingmail.Handler, venue *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		b, err := loadBooking(c, conn)
		if err != nil {
			return err
		}

		var thread bookingmail.Thread
		if b.Email != "" {
			thread = bookingMailer.Thread([]string{b.Email}, c.QueryParam("refresh") == "1")
		}

		data := views.BookingDetailData{
			Booking:        b,
			Thread:         thread,
			CanSend:        bookingMailer.CanSend(),
			MailboxAddress: bookingMailer.Address(),
			Venue:          venue,
		}
		if c.QueryParam("sent") == "1" {
			data.FlashNotice = "Reply sent."
			if c.QueryParam("warn") == "1" {
				data.FlashNotice += " It may take a moment to show up below — the copy couldn't be filed in Sent right away."
			}
		}
		if c.QueryParam("send_error") != "" {
			data.FlashError = replyErrorMessage(c.QueryParam("send_error"))
		}

		return views.AdminBookingDetail(data, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}

func replyErrorMessage(code string) string {
	switch code {
	case "empty":
		return "Please add both a subject and a message."
	case "expired":
		return "That confirmation expired or was already used — nothing was sent. Try again."
	default:
		return "The reply could not be sent. Check the server log for details."
	}
}

// previewBookingReply shows the admin the exact message a send will produce
// and issues a single-use confirmation token, mirroring the archive/reject
// confirm step already used elsewhere in the admin: a destructive or
// externally-visible action gets a look-before-you-leap page rather than
// firing straight off a button.
func previewBookingReply(conn *sql.DB, bookingMailer *bookingmail.Handler) echo.HandlerFunc {
	return func(c echo.Context) error {
		b, err := loadBooking(c, conn)
		if err != nil {
			return err
		}
		subject := strings.TrimSpace(c.FormValue("subject"))
		body := strings.TrimSpace(c.FormValue("body"))
		if subject == "" || body == "" {
			return c.Redirect(http.StatusSeeOther, bookingPath(b.ID)+"?send_error=empty")
		}

		// Reply into the existing thread when there is one, so the
		// recipient's mail client keeps it in the same conversation.
		inReplyTo := ""
		if b.Email != "" {
			thread := bookingMailer.Thread([]string{b.Email}, false)
			if len(thread.Messages) > 0 {
				inReplyTo = thread.Messages[len(thread.Messages)-1].MessageID
			}
		}

		nonce, err := newNonce()
		if err != nil {
			return err
		}
		sess := getSession(c)
		sess.Values["booking_reply_nonce"] = nonce
		if err := sess.Save(c.Request(), c.Response()); err != nil {
			return err
		}

		data := views.ReplyPreviewData{
			Booking:        b,
			MailboxAddress: bookingMailer.Address(),
			FromName:       "DECAY Booking",
			Subject:        subject,
			Body:           body,
			SenderName:     currentUser(c).Name(),
			InReplyTo:      inReplyTo,
			Nonce:          nonce,
		}
		return views.AdminBookingReplyPreview(data, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}

// sendBookingReply performs the actual send. The nonce is single-use: it's
// removed from the session before Send runs, so a double submit (a refresh
// on this page, a repeated click) can't send the same reply twice.
func sendBookingReply(conn *sql.DB, bookingMailer *bookingmail.Handler) echo.HandlerFunc {
	return func(c echo.Context) error {
		b, err := loadBooking(c, conn)
		if err != nil {
			return err
		}

		sess := getSession(c)
		expected, _ := sess.Values["booking_reply_nonce"].(string)
		delete(sess.Values, "booking_reply_nonce")
		_ = sess.Save(c.Request(), c.Response())

		supplied := c.FormValue("nonce")
		if expected == "" || supplied == "" || expected != supplied {
			return c.Redirect(http.StatusSeeOther, bookingPath(b.ID)+"?send_error=expired")
		}

		res, err := bookingMailer.Send(bookingmail.ReplyInput{
			To:         b.Email,
			ToName:     b.Name,
			Subject:    c.FormValue("subject"),
			Body:       c.FormValue("body"),
			SenderName: currentUser(c).Name(),
			InReplyTo:  c.FormValue("in_reply_to"),
		})
		if err != nil {
			log.Printf("booking #%d reply to %s failed: %v", b.ID, b.Email, err)
			return c.Redirect(http.StatusSeeOther, bookingPath(b.ID)+"?send_error=1")
		}
		log.Printf("booking #%d reply sent to %s by %s", b.ID, b.Email, currentUser(c).Name())

		redirect := bookingPath(b.ID) + "?sent=1"
		if res.Warning != "" {
			redirect += "&warn=1"
		}
		return c.Redirect(http.StatusSeeOther, redirect)
	}
}

func bookingPath(id int64) string {
	return "/admin/bookings/" + strconv.FormatInt(id, 10)
}

func newNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
