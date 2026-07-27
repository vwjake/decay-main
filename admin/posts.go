package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"decay-main/db"
	"decay-main/embed"
	"decay-main/images"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

var slugSanitizer = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeSlug(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "-")
	return slugSanitizer.ReplaceAllString(s, "")
}

func registerPostRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/posts", listPosts(conn))
	g.POST("/posts", createPost(conn))
	g.GET("/posts/:id", editPost(conn))
	g.POST("/posts/:id", savePost(conn))
	g.POST("/posts/:id/publish", publishPost(conn))
	g.POST("/posts/:id/unpublish", unpublishPost(conn))
	g.POST("/posts/:id/delete", deletePost(conn, uploadsDir))
	g.POST("/posts/:id/images", uploadPostImage(conn, uploadsDir))
	g.POST("/posts/:id/images/:imageID/delete", deletePostImage(conn, uploadsDir))
	g.POST("/posts/embed", resolveEmbed())
}

// resolveEmbed turns a pasted YouTube or Bandcamp link into the line the
// editor should insert (see package embed). It's the one place a Bandcamp
// page is fetched, so it stays off the render path. Errors come back in the
// JSON body for the toolbar to show, not as an HTTP error.
func resolveEmbed() echo.HandlerFunc {
	return func(c echo.Context) error {
		insert, err := embed.Resolve(c.FormValue("url"))
		if err != nil {
			return c.JSON(http.StatusOK, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"insert": insert})
	}
}

func editPost(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		post, err := loadPost(conn, c.Param("id"))
		if err != nil {
			return err
		}
		return renderPostEdit(c, conn, post, "")
	}
}

// renderPostEdit loads a post's images and renders the edit page, so the
// image strip is always in sync whether the page is shown fresh or after a
// validation error.
func renderPostEdit(c echo.Context, conn *sql.DB, post db.Post, msg string) error {
	imgs, err := db.ListPostImages(conn, post.ID)
	if err != nil {
		return err
	}
	return views.AdminPostEdit(post, imgs, currentUser(c), msg).Render(c.Request().Context(), c.Response())
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
			return renderPostEdit(c, conn, post, "Title, address, and body are required.")
		}

		if err := db.UpdatePost(conn, post.ID, slug, title, body); err != nil {
			return renderPostEdit(c, conn, post, "That address is already taken.")
		}
		return c.Redirect(http.StatusSeeOther, "/admin/posts/"+c.Param("id"))
	}
}

// uploadPostImage saves an image for a post and records it. On the edit
// page the author then inserts it into the body by clicking it.
func uploadPostImage(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		post, err := loadPost(conn, c.Param("id"))
		if err != nil {
			return err
		}

		fileHeader, err := c.FormFile("image")
		if err != nil {
			return renderPostEdit(c, conn, post, "Choose an image to upload.")
		}

		dir := filepath.Join(uploadsDir, db.PostImagesSubdir)
		filename, err := saveImage(fileHeader, dir)
		if err != nil {
			if errors.Is(err, errNotAnImage) {
				return renderPostEdit(c, conn, post, "That file doesn't look like an image. Use jpg, png, gif, or webp.")
			}
			return err
		}
		// Blog images come off phones and design tools at full size, so the
		// body gets a web-sized copy with the original kept behind it.
		if err := images.MakeWeb(
			filepath.Join(dir, filename),
			filepath.Join(dir, "web", images.WebName(filename)),
		); err != nil {
			return err
		}
		if err := db.CreatePostImage(conn, post.ID, filename); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/posts/"+c.Param("id"))
	}
}

// deletePostImage removes one image and its files. It doesn't touch the
// post body, so a still-referenced image would 404 in the body — the author
// removes the Markdown line themselves.
func deletePostImage(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		postID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		imageID, err := strconv.ParseInt(c.Param("imageID"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		filename, err := db.DeletePostImage(conn, postID, imageID)
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}
		removePostImageFiles(uploadsDir, filename)
		return c.Redirect(http.StatusSeeOther, "/admin/posts/"+c.Param("id"))
	}
}

func removePostImageFiles(uploadsDir, filename string) {
	dir := filepath.Join(uploadsDir, db.PostImagesSubdir)
	_ = os.Remove(filepath.Join(dir, filename))
	_ = os.Remove(filepath.Join(dir, "web", images.WebName(filename)))
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

func deletePost(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		// Remove the image files first; the rows themselves cascade with the
		// post, but the files on disk don't.
		imgs, err := db.ListPostImages(conn, id)
		if err != nil {
			return err
		}
		if err := db.DeletePost(conn, id); err != nil {
			return err
		}
		for _, img := range imgs {
			removePostImageFiles(uploadsDir, img.Filename)
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
