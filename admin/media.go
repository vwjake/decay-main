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

func registerMediaRoutes(g *echo.Group, conn *sql.DB) {
	g.GET("/media", listMedia(conn))
	g.POST("/media", createVideo(conn))
	g.POST("/media/:id", saveVideo(conn))
	g.POST("/media/:id/delete", deleteVideo(conn))
}

func listMedia(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		return renderMedia(c, conn, "")
	}
}

func renderMedia(c echo.Context, conn *sql.DB, msg string) error {
	videos, err := db.ListVideos(conn)
	if err != nil {
		return err
	}
	return views.AdminMedia(videos, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}

func createVideo(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, ok := db.ParseYouTubeID(c.FormValue("youtube"))
		if !ok {
			return renderMedia(c, conn, "That doesn't look like a YouTube link. Paste a video URL or its id.")
		}
		position, err := parsePosition(c.FormValue("position"))
		if err != nil {
			return renderMedia(c, conn, "Order has to be a whole number.")
		}
		if _, err := db.CreateVideo(conn, db.Video{
			YouTubeID: id,
			Title:     strings.TrimSpace(c.FormValue("title")),
			Position:  position,
		}); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/media")
	}
}

func saveVideo(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		v, err := db.VideoByID(conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}

		ytID, ok := db.ParseYouTubeID(c.FormValue("youtube"))
		if !ok {
			return renderMedia(c, conn, "That doesn't look like a YouTube link. Paste a video URL or its id.")
		}
		position, err := parsePosition(c.FormValue("position"))
		if err != nil {
			return renderMedia(c, conn, "Order has to be a whole number.")
		}

		v.YouTubeID = ytID
		v.Title = strings.TrimSpace(c.FormValue("title"))
		v.Position = position
		if err := db.UpdateVideo(conn, v); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/media")
	}
}

func deleteVideo(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteVideo(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/media")
	}
}
