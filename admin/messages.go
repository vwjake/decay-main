package admin

import (
	"database/sql"
	"net/http"
	"strconv"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

func registerMessageRoutes(g *echo.Group, conn *sql.DB) {
	g.GET("/messages", listMessages(conn))
	g.POST("/messages/:id/status", setMessageStatus(conn))
	g.POST("/messages/:id/delete", deleteMessage(conn))
}

func listMessages(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		showArchived := c.QueryParam("archived") != ""
		messages, err := db.ListContactMessages(conn, showArchived)
		if err != nil {
			return err
		}
		return views.AdminMessages(messages, showArchived, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

// validMessageStatuses guards the status write to the known set, so a
// hand-crafted form can't set an arbitrary value.
var validMessageStatuses = map[string]bool{
	db.ContactNew: true, db.ContactReviewed: true, db.ContactArchived: true,
}

func setMessageStatus(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		status := c.FormValue("status")
		if !validMessageStatuses[status] {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.SetContactStatus(conn, id, status); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/messages")
	}
}

func deleteMessage(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteContactMessage(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/messages")
	}
}
