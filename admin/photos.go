package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"decay-main/db"
	"decay-main/images"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

func registerPhotoRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/photos", listPhotos(conn))
	g.POST("/photos", uploadPhoto(conn, uploadsDir))
	g.POST("/photos/:id", savePhoto(conn))
	g.POST("/photos/:id/delete", deletePhoto(conn, uploadsDir))
}

func savePhoto(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.UpdatePhoto(conn, id, c.FormValue("caption"), parseGroupID(c.FormValue("group_id"))); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/photos")
	}
}

func listPhotos(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		return renderPhotos(c, conn, "")
	}
}

func renderPhotos(c echo.Context, conn *sql.DB, msg string) error {
	photos, err := db.ListPhotos(conn)
	if err != nil {
		return err
	}
	groups, err := db.ListGroups(conn)
	if err != nil {
		return err
	}
	return views.AdminPhotos(photos, groups, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}

// parseGroupID reads the photo tag dropdown: an empty value is "no group"
// (nil), anything else the chosen group's id.
func parseGroupID(raw string) *int64 {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &id
}

func uploadPhoto(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		fileHeader, err := c.FormFile("photo")
		if err != nil {
			return rerenderPhotosError(c, conn, "Choose an image to upload.")
		}

		dir := filepath.Join(uploadsDir, db.PhotosSubdir)
		filename, err := saveImage(fileHeader, dir)
		if err != nil {
			if errors.Is(err, errNotAnImage) {
				return rerenderPhotosError(c, conn, "That file doesn't look like an image. Use jpg, png, gif, or webp.")
			}
			return err
		}
		// Gallery shots come straight off a phone at full resolution, so
		// the page gets a web-sized copy and the original stays behind it.
		if err := images.MakeWeb(
			filepath.Join(dir, filename),
			filepath.Join(dir, "web", images.WebName(filename)),
		); err != nil {
			return err
		}

		if err := db.CreatePhoto(conn, filename, c.FormValue("caption"), parseGroupID(c.FormValue("group_id"))); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/photos")
	}
}

func deletePhoto(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		filename, err := db.DeletePhoto(conn, id)
		if err != nil {
			return err
		}
		dir := filepath.Join(uploadsDir, db.PhotosSubdir)
		_ = os.Remove(filepath.Join(dir, filename))
		_ = os.Remove(filepath.Join(dir, "web", images.WebName(filename)))
		return c.Redirect(http.StatusSeeOther, "/admin/photos")
	}
}

func rerenderPhotosError(c echo.Context, conn *sql.DB, msg string) error {
	return renderPhotos(c, conn, msg)
}
