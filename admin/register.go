package admin

import (
	"database/sql"
	"net/http"
	"time"

	"decay-main/db"
	"decay-main/meetings"
	"decay-main/views"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

// Register wires up session middleware and every /admin route on e. venue
// is the timezone dates are read in, and meetingsURL is the shared .ics
// feed of the internal calendar (empty disables the meetings page).
func Register(e *echo.Echo, conn *sql.DB, sessionSecret []byte, uploadsDir string, venue *time.Location, meetingsURL string) {
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
	registerProductRoutes(products, conn, uploadsDir)

	posts := g.Group("", requirePermission(db.PermPosts))
	registerPostRoutes(posts, conn, uploadsDir)

	photos := g.Group("", requirePermission(db.PermPhotos))
	registerPhotoRoutes(photos, conn, uploadsDir)

	people := g.Group("", requirePermission(db.PermPeople))
	registerPeopleRoutes(people, conn, uploadsDir)

	groups := g.Group("", requirePermission(db.PermGroups))
	registerGroupRoutes(groups, conn, uploadsDir)

	media := g.Group("", requirePermission(db.PermMedia))
	registerMediaRoutes(media, conn)

	bookings := g.Group("", requirePermission(db.PermBookings))
	registerBookingRoutes(bookings, conn)

	reports := g.Group("", requirePermission(db.PermReports))
	registerReportRoutes(reports, conn)

	meetingsClient := meetings.NewClient(meetingsURL, venue)
	meetingsGroup := g.Group("", requirePermission(db.PermMeetings))
	registerMeetingRoutes(meetingsGroup, meetingsClient, venue)

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
