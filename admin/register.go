package admin

import (
	"database/sql"
	"net/http"

	"decay-main/db"
	"decay-main/views"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

// Register wires up session middleware and every /admin route on e.
func Register(e *echo.Echo, conn *sql.DB, sessionSecret []byte, uploadsDir string) {
	store := sessions.NewCookieStore(sessionSecret)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   8 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	e.Use(session.Middleware(store))

	e.GET("/admin/login", loginForm)
	e.POST("/admin/login", login(conn))
	e.POST("/admin/logout", logout)

	g := e.Group("/admin", requireAuth(conn))
	g.GET("", dashboard(conn))

	// Each section is gated on its own permission, so a role that grants
	// only some of them reaches only those pages.
	events := g.Group("", requirePermission(db.PermEvents))
	registerEventRoutes(events, conn, uploadsDir)

	products := g.Group("", requirePermission(db.PermShop))
	registerProductRoutes(products, conn)

	posts := g.Group("", requirePermission(db.PermPosts))
	registerPostRoutes(posts, conn)

	photos := g.Group("", requirePermission(db.PermPhotos))
	registerPhotoRoutes(photos, conn, uploadsDir)

	registerUserRoutes(g, conn)
}

func dashboard(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		counts, err := db.Summary(conn)
		if err != nil {
			return err
		}
		return views.AdminDashboard(counts, currentUser(c)).Render(c.Request().Context(), c.Response())
	}
}
