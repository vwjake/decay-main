package admin

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

var allowedPhotoExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

func registerPhotoRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/photos", listPhotos(conn))
	g.POST("/photos", uploadPhoto(conn, uploadsDir))
	g.POST("/photos/:id/delete", deletePhoto(conn, uploadsDir))
}

func listPhotos(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		photos, err := db.ListPhotos(conn)
		if err != nil {
			return err
		}
		return views.AdminPhotos(photos, "").Render(c.Request().Context(), c.Response())
	}
}

func uploadPhoto(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		fileHeader, err := c.FormFile("photo")
		if err != nil {
			return rerenderPhotosError(c, conn, "Choose an image to upload.")
		}

		ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
		if !allowedPhotoExt[ext] {
			return rerenderPhotosError(c, conn, "Unsupported file type. Use jpg, png, gif, or webp.")
		}

		src, err := fileHeader.Open()
		if err != nil {
			return err
		}
		defer src.Close()

		// Extension whitelist plus a content sniff — belt and suspenders
		// against a renamed non-image ending up on a publicly served path.
		buf := make([]byte, 512)
		n, _ := io.ReadFull(src, buf)
		if !strings.HasPrefix(http.DetectContentType(buf[:n]), "image/") {
			return rerenderPhotosError(c, conn, "That file doesn't look like an image.")
		}
		if _, err := src.Seek(0, io.SeekStart); err != nil {
			return err
		}

		// Filename is entirely server-generated, never derived from the
		// upload's original name, so there's no path-traversal surface.
		filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
		dst, err := os.Create(filepath.Join(uploadsDir, filename))
		if err != nil {
			return err
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return err
		}

		if err := db.CreatePhoto(conn, filename, c.FormValue("caption")); err != nil {
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
		_ = os.Remove(filepath.Join(uploadsDir, filename))
		return c.Redirect(http.StatusSeeOther, "/admin/photos")
	}
}

func rerenderPhotosError(c echo.Context, conn *sql.DB, msg string) error {
	photos, err := db.ListPhotos(conn)
	if err != nil {
		return err
	}
	return views.AdminPhotos(photos, msg).Render(c.Request().Context(), c.Response())
}
