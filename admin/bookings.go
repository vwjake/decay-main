package admin

import (
	"database/sql"
	"net/http"
	"strconv"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

func registerBookingRoutes(g *echo.Group, conn *sql.DB) {
	g.GET("/bookings", listBookings(conn))
	g.POST("/bookings/:id/status", setBookingStatus(conn))
	g.POST("/bookings/:id/delete", deleteBooking(conn))
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
