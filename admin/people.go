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

// peopleSubdir keeps board/staff portraits apart from the other uploads.
const peopleSubdir = "people"

func registerPeopleRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/people", listPeople(conn))
	g.POST("/people", createPerson(conn))
	g.GET("/people/:id", editPerson(conn))
	g.POST("/people/:id", savePerson(conn))
	g.POST("/people/:id/photo", uploadPersonPhoto(conn, uploadsDir))
	g.POST("/people/:id/delete", deletePerson(conn, uploadsDir))
}

func listPeople(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		return renderPeople(c, conn, "")
	}
}

func renderPeople(c echo.Context, conn *sql.DB, msg string) error {
	people, err := db.ListPeople(conn)
	if err != nil {
		return err
	}
	return views.AdminPeople(people, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}

func createPerson(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		name := strings.TrimSpace(c.FormValue("name"))
		if name == "" {
			return renderPeople(c, conn, "Name is required.")
		}
		position, err := parsePosition(c.FormValue("position"))
		if err != nil {
			return renderPeople(c, conn, "Position has to be a whole number.")
		}
		p := db.Person{
			Name:     name,
			Pronouns: strings.TrimSpace(c.FormValue("pronouns")),
			Role:     strings.TrimSpace(c.FormValue("role")),
			Bio:      c.FormValue("bio"),
			Position: position,
		}
		id, err := db.CreatePerson(conn, p)
		if err != nil {
			return err
		}
		// Straight to the edit page so the photo can go on next.
		return c.Redirect(http.StatusSeeOther, "/admin/people/"+strconv.FormatInt(id, 10))
	}
}

func editPerson(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		p, err := loadPerson(conn, c.Param("id"))
		if err != nil {
			return err
		}
		return views.AdminPersonEdit(p, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

func savePerson(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		p, err := loadPerson(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminPersonEdit(p, currentUser(c), msg).Render(c.Request().Context(), c.Response())
		}

		name := strings.TrimSpace(c.FormValue("name"))
		if name == "" {
			return rerender("Name is required.")
		}
		position, err := parsePosition(c.FormValue("position"))
		if err != nil {
			return rerender("Position has to be a whole number.")
		}

		p.Name = name
		p.Pronouns = strings.TrimSpace(c.FormValue("pronouns"))
		p.Role = strings.TrimSpace(c.FormValue("role"))
		p.Bio = c.FormValue("bio")
		p.Position = position
		if err := db.UpdatePerson(conn, p); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/people")
	}
}

func uploadPersonPhoto(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		p, err := loadPerson(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminPersonEdit(p, currentUser(c), msg).Render(c.Request().Context(), c.Response())
		}

		fileHeader, err := c.FormFile("photo")
		if err != nil {
			return rerender("Choose an image to upload.")
		}
		dir := filepath.Join(uploadsDir, peopleSubdir)
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

		previous, err := db.SetPersonPhoto(conn, p.ID, filename)
		if err != nil {
			return err
		}
		if previous != "" && previous != filename {
			_ = os.Remove(filepath.Join(dir, previous))
			_ = os.Remove(filepath.Join(dir, "web", images.WebName(previous)))
		}
		return c.Redirect(http.StatusSeeOther, "/admin/people/"+c.Param("id"))
	}
}

func deletePerson(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		photo, err := db.DeletePerson(conn, id)
		if err != nil {
			return err
		}
		if photo != "" {
			dir := filepath.Join(uploadsDir, peopleSubdir)
			_ = os.Remove(filepath.Join(dir, photo))
			_ = os.Remove(filepath.Join(dir, "web", images.WebName(photo)))
		}
		return c.Redirect(http.StatusSeeOther, "/admin/people")
	}
}

func loadPerson(conn *sql.DB, rawID string) (db.Person, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return db.Person{}, echo.NewHTTPError(http.StatusBadRequest)
	}
	p, err := db.PersonByID(conn, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Person{}, echo.NewHTTPError(http.StatusNotFound)
	}
	return p, err
}

// parsePosition reads the ordering field, treating blank as 0 so it's an
// optional detail rather than something to fill in on every profile.
func parsePosition(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}
