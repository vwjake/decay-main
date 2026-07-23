package admin

import (
	"database/sql"
	"net/http"

	"decay-main/views"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

// Register wires up session middleware and every /admin route on e.
func Register(e *echo.Echo, conn *sql.DB, cfg Config, sessionSecret []byte, uploadsDir string) {
	store := sessions.NewCookieStore(sessionSecret)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   8 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	e.Use(session.Middleware(store))

	e.GET("/admin/login", loginForm)
	e.POST("/admin/login", login(cfg))
	e.POST("/admin/logout", logout)

	g := e.Group("/admin", requireAuth)
	g.GET("", dashboard)
	registerEventRoutes(g, conn, uploadsDir)
	registerProductRoutes(g, conn)
	registerPostRoutes(g, conn)
	registerPhotoRoutes(g, conn, uploadsDir)
}

func dashboard(c echo.Context) error {
	return views.AdminDashboard().Render(c.Request().Context(), c.Response())
}
