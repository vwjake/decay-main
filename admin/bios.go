package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"decay-main/db"

	"github.com/labstack/echo/v4"
)

func registerBioRoutes(g *echo.Group, conn *sql.DB) {
	g.GET("/bios", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/admin/people")
	})
	g.POST("/bios", createBio(conn))
	g.POST("/bios/:id", saveBio(conn))
	g.POST("/bios/:id/delete", deleteBio(conn))
}

// renderBios re-renders the merged People page with a message attributed to
// the community bios section.
func renderBios(c echo.Context, conn *sql.DB, msg string) error {
	return renderPeoplePage(c, conn, "", msg)
}

func createBio(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		b, ok, msg := bioFromRequest(c)
		if !ok {
			return renderBios(c, conn, msg)
		}
		if _, err := db.CreateCommunityBio(conn, b); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/people")
	}
}

func saveBio(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		existing, err := db.CommunityBioByID(conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}
		b, ok, msg := bioFromRequest(c)
		if !ok {
			return renderBios(c, conn, msg)
		}
		b.ID = existing.ID
		if err := db.UpdateCommunityBio(conn, b); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/people")
	}
}

func deleteBio(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteCommunityBio(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/people")
	}
}

// bioFromRequest reads and validates the shared add/edit fields.
func bioFromRequest(c echo.Context) (db.CommunityBio, bool, string) {
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return db.CommunityBio{}, false, "Name is required."
	}
	position, err := parsePosition(c.FormValue("position"))
	if err != nil {
		return db.CommunityBio{}, false, "Order has to be a whole number."
	}
	return db.CommunityBio{
		Name:     name,
		Pronouns: strings.TrimSpace(c.FormValue("pronouns")),
		Role:     strings.TrimSpace(c.FormValue("role")),
		Bio:      c.FormValue("bio"),
		Public:   c.FormValue("public") != "",
		Position: position,
	}, true, ""
}
