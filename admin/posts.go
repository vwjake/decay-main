package admin

import (
	"database/sql"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

var slugSanitizer = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeSlug(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "-")
	return slugSanitizer.ReplaceAllString(s, "")
}

func registerPostRoutes(g *echo.Group, conn *sql.DB) {
	g.GET("/posts", listPosts(conn))
	g.POST("/posts", createPost(conn))
	g.POST("/posts/:id/publish", publishPost(conn))
	g.POST("/posts/:id/delete", deletePost(conn))
}

func listPosts(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		posts, err := db.ListAllPosts(conn)
		if err != nil {
			return err
		}
		return views.AdminPosts(posts, "").Render(c.Request().Context(), c.Response())
	}
}

func createPost(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		slug := sanitizeSlug(c.FormValue("slug"))
		title := c.FormValue("title")
		body := c.FormValue("body")
		if slug == "" || title == "" || body == "" {
			return rerenderPostsError(c, conn, "Title, slug, and body are required.")
		}
		publish := c.FormValue("publish") == "on"
		if err := db.CreatePost(conn, slug, title, body, publish); err != nil {
			return rerenderPostsError(c, conn, "That slug is already taken.")
		}
		return c.Redirect(http.StatusSeeOther, "/admin/posts")
	}
}

func publishPost(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.PublishPost(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/posts")
	}
}

func deletePost(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeletePost(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/posts")
	}
}

func rerenderPostsError(c echo.Context, conn *sql.DB, msg string) error {
	posts, err := db.ListAllPosts(conn)
	if err != nil {
		return err
	}
	return views.AdminPosts(posts, msg).Render(c.Request().Context(), c.Response())
}
