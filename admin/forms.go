package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

func registerFormRoutes(g *echo.Group, conn *sql.DB) {
	g.GET("/forms", listForms(conn))
	g.POST("/forms", createForm(conn))
	g.POST("/forms/:id", saveForm(conn))
	g.POST("/forms/:id/delete", deleteForm(conn))
}

func listForms(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		return renderForms(c, conn, "")
	}
}

func renderForms(c echo.Context, conn *sql.DB, msg string) error {
	forms, err := db.ListExternalForms(conn)
	if err != nil {
		return err
	}
	return views.AdminForms(forms, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}

func createForm(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		f, ok, msg := formFromRequest(c)
		if !ok {
			return renderForms(c, conn, msg)
		}
		if _, err := db.CreateExternalForm(conn, f); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/forms")
	}
}

func saveForm(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		existing, err := db.ExternalFormByID(conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}
		f, ok, msg := formFromRequest(c)
		if !ok {
			return renderForms(c, conn, msg)
		}
		f.ID = existing.ID
		if err := db.UpdateExternalForm(conn, f); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/forms")
	}
}

func deleteForm(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteExternalForm(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/forms")
	}
}

// formFromRequest reads and validates the shared add/edit fields. On a bad
// input it returns ok=false and a message to re-render with.
func formFromRequest(c echo.Context) (db.ExternalForm, bool, string) {
	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" {
		return db.ExternalForm{}, false, "Give the form a title."
	}
	link, ok := db.NormalizeFormURL(c.FormValue("url"))
	if !ok {
		return db.ExternalForm{}, false, "That doesn't look like a link. Paste the full form URL, including https://."
	}
	position, err := parsePosition(c.FormValue("position"))
	if err != nil {
		return db.ExternalForm{}, false, "Order has to be a whole number."
	}
	return db.ExternalForm{
		Title:       title,
		Description: strings.TrimSpace(c.FormValue("description")),
		URL:         link,
		Position:    position,
		Enabled:     c.FormValue("enabled") != "",
	}, true, ""
}
