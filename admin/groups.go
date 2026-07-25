package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"decay-main/db"
	"decay-main/images"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

// groupsSubdir keeps group hero images apart from the other uploads.
const groupsSubdir = "groups"

func registerGroupRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/groups", listGroups(conn))
	g.POST("/groups", createGroup(conn))
	g.GET("/groups/:id", editGroup(conn))
	g.POST("/groups/:id", saveGroup(conn))
	g.POST("/groups/:id/hero", uploadGroupHero(conn, uploadsDir))
	g.POST("/groups/:id/delete", deleteGroup(conn, uploadsDir))
}

func listGroups(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		return renderGroups(c, conn, "")
	}
}

func renderGroups(c echo.Context, conn *sql.DB, msg string) error {
	groups, err := db.ListGroups(conn)
	if err != nil {
		return err
	}
	return views.AdminGroups(groups, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}

func createGroup(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		name := strings.TrimSpace(c.FormValue("name"))
		if name == "" {
			return renderGroups(c, conn, "Name is required.")
		}
		slug := sanitizeSlug(c.FormValue("slug"))
		if slug == "" {
			slug = sanitizeSlug(name)
		}
		if slug == "" {
			return renderGroups(c, conn, "Give the group a name that makes a valid web address.")
		}
		taken, err := db.GroupSlugTaken(conn, slug, 0)
		if err != nil {
			return err
		}
		if taken {
			return renderGroups(c, conn, "Another group already uses that address.")
		}

		g := db.Group{
			Slug:    slug,
			Name:    name,
			Summary: strings.TrimSpace(c.FormValue("summary")),
			Enabled: c.FormValue("enabled") != "",
		}
		id, err := db.CreateGroup(conn, g)
		if err != nil {
			return err
		}
		// On to the edit page, where the body and hero image go.
		return c.Redirect(http.StatusSeeOther, "/admin/groups/"+strconv.FormatInt(id, 10))
	}
}

func editGroup(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		g, err := loadGroup(conn, c.Param("id"))
		if err != nil {
			return err
		}
		return views.AdminGroupEdit(g, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

func saveGroup(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		g, err := loadGroup(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminGroupEdit(g, currentUser(c), msg).Render(c.Request().Context(), c.Response())
		}

		name := strings.TrimSpace(c.FormValue("name"))
		if name == "" {
			return rerender("Name is required.")
		}
		slug := sanitizeSlug(c.FormValue("slug"))
		if slug == "" {
			slug = g.Slug
		}
		if slug != g.Slug {
			taken, err := db.GroupSlugTaken(conn, slug, g.ID)
			if err != nil {
				return err
			}
			if taken {
				return rerender("Another group already uses that address.")
			}
		}
		position, err := parsePosition(c.FormValue("position"))
		if err != nil {
			return rerender("Order has to be a whole number.")
		}

		g.Name = name
		g.Slug = slug
		g.Summary = strings.TrimSpace(c.FormValue("summary"))
		g.Description = strings.TrimSpace(c.FormValue("description"))
		g.Pills = normalizeLines(c.FormValue("pills"))
		g.HeroAlt = strings.TrimSpace(c.FormValue("hero_alt"))
		g.Body = c.FormValue("body")
		g.MatchTerms = normalizeLines(c.FormValue("match_terms"))
		g.Position = position
		g.Enabled = c.FormValue("enabled") != ""
		if err := db.UpdateGroup(conn, g); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/groups")
	}
}

func uploadGroupHero(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		g, err := loadGroup(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminGroupEdit(g, currentUser(c), msg).Render(c.Request().Context(), c.Response())
		}

		fileHeader, err := c.FormFile("hero")
		if err != nil {
			return rerender("Choose an image to upload.")
		}
		dir := filepath.Join(uploadsDir, groupsSubdir)
		filename, err := saveImage(fileHeader, dir)
		if err != nil {
			if errors.Is(err, errNotAnImage) {
				return rerender("That file doesn't look like an image. Use jpg, png, gif, or webp.")
			}
			return err
		}
		if err := images.MakeWeb(
			filepath.Join(dir, filename),
			filepath.Join(dir, "web", images.WebName(filename)),
		); err != nil {
			return err
		}

		previous, err := db.SetGroupHero(conn, g.ID, filename)
		if err != nil {
			return err
		}
		if previous != "" && previous != filename {
			_ = os.Remove(filepath.Join(dir, previous))
			_ = os.Remove(filepath.Join(dir, "web", images.WebName(previous)))
		}
		return c.Redirect(http.StatusSeeOther, "/admin/groups/"+c.Param("id"))
	}
}

func deleteGroup(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		hero, err := db.DeleteGroup(conn, id)
		if err != nil {
			return err
		}
		if hero != "" {
			dir := filepath.Join(uploadsDir, groupsSubdir)
			_ = os.Remove(filepath.Join(dir, hero))
			_ = os.Remove(filepath.Join(dir, "web", images.WebName(hero)))
		}
		return c.Redirect(http.StatusSeeOther, "/admin/groups")
	}
}

func loadGroup(conn *sql.DB, rawID string) (db.Group, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return db.Group{}, echo.NewHTTPError(http.StatusBadRequest)
	}
	g, err := db.GroupByID(conn, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Group{}, echo.NewHTTPError(http.StatusNotFound)
	}
	return g, err
}

// normalizeLines trims each line and drops blank ones, so the stored pills
// are clean however they were typed (one per line).
func normalizeLines(raw string) string {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
