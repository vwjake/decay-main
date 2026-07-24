package admin

import (
	"database/sql"
	"errors"
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
	g.GET("/posts/:id", editPost(conn))
	g.POST("/posts/:id", savePost(conn))
	g.POST("/posts/:id/publish", publishPost(conn))
	g.POST("/posts/:id/unpublish", unpublishPost(conn))
	g.POST("/posts/:id/delete", deletePost(conn))
}

func editPost(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		post, err := loadPost(conn, c.Param("id"))
		if err != nil {
			return err
		}
		return views.AdminPostEdit(post, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

func savePost(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		post, err := loadPost(conn, c.Param("id"))
		if err != nil {
			return err
		}

		slug := sanitizeSlug(c.FormValue("slug"))
		title := strings.TrimSpace(c.FormValue("title"))
		body := c.FormValue("body")
		if slug == "" || title == "" || body == "" {
			return views.AdminPostEdit(post, currentUser(c), "Title, address, and body are required.").
				Render(c.Request().Context(), c.Response())
		}

		if err := db.UpdatePost(conn, post.ID, slug, title, body); err != nil {
			return views.AdminPostEdit(post, currentUser(c), "That address is already taken.").
				Render(c.Request().Context(), c.Response())
		}
		return c.Redirect(http.StatusSeeOther, "/admin/posts/"+c.Param("id"))
	}
}

func unpublishPost(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.UnpublishPost(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/posts")
	}
}

func loadPost(conn *sql.DB, rawID string) (db.Post, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return db.Post{}, echo.NewHTTPError(http.StatusBadRequest)
	}
	post, err := db.PostByID(conn, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Post{}, echo.NewHTTPError(http.StatusNotFound)
	}
	return post, err
}

func listPosts(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		posts, err := db.ListAllPosts(conn)
		if err != nil {
			return err
		}
		return views.AdminPosts(posts, currentUser(c), "").Render(c.Request().Context(), c.Response())
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
	return views.AdminPosts(posts, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}
