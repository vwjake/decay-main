package admin

import (
	"database/sql"
	"net/http"
	"time"

	"decay-main/bookingmail"
	"decay-main/db"
	"decay-main/mail"
	"decay-main/staff"
	"decay-main/views"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

// Register wires up session middleware and every /admin route on e. venue
// is the timezone dates are read in, staffURL is the shared .ics feed of the
// internal staff calendar (empty disables the staff page), and bookingMailer
// reads/replies to the booking mailbox for the booking detail page (a
// disabled Handler just hides that panel). secureCookies marks the session
// cookie Secure (HTTPS-only) in production. mailer sends invite emails (a
// disabled Mailer just means the link has to be shared by hand), and
// siteURL is what a signup link is built on.
func Register(e *echo.Echo, conn *sql.DB, sessionSecret []byte, uploadsDir string, venue *time.Location, staffURL string, bookingMailer *bookingmail.Handler, secureCookies bool, mailer *mail.Mailer, siteURL string) {
	store := sessions.NewCookieStore(sessionSecret)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   8 * 60 * 60,
		HttpOnly: true,
		Secure:   secureCookies,
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
	registerEventRoutes(events, conn, uploadsDir, bookingMailer, venue)

	products := g.Group("", requirePermission(db.PermShop))
	registerProductRoutes(products, conn, uploadsDir)

	posts := g.Group("", requirePermission(db.PermPosts))
	registerPostRoutes(posts, conn, uploadsDir)

	people := g.Group("", requirePermission(db.PermPeople))
	registerPeopleRoutes(people, conn, uploadsDir)
	registerBioRoutes(people, conn)

	groups := g.Group("", requirePermission(db.PermGroups))
	registerGroupRoutes(groups, conn, uploadsDir)

	media := g.Group("", requirePermission(db.PermMedia))
	registerMediaRoutes(media, conn)
	registerPhotoRoutes(media, conn, uploadsDir)

	bookings := g.Group("", requirePermission(db.PermBookings))
	registerBookingRoutes(bookings, conn, bookingMailer, venue)

	messages := g.Group("", requirePermission(db.PermMessages))
	registerMessageRoutes(messages, conn)

	forms := g.Group("", requirePermission(db.PermForms))
	registerFormRoutes(forms, conn)

	reports := g.Group("", requirePermission(db.PermReports))
	registerReportRoutes(reports, conn)

	staffClient := staff.NewClient(staffURL, venue)
	staffGroup := g.Group("", requirePermission(db.PermStaff))
	registerStaffRoutes(staffGroup, conn, staffClient, venue)

	registerUserRoutes(g, conn, uploadsDir)
	registerInviteRoutes(g, conn, mailer, siteURL)

	// Everyone has an account page, whatever else their role reaches.
	registerAccountRoutes(g, conn, uploadsDir)

	// The signup form itself is public — the token in the link is what
	// stands in for being signed in.
	registerSignupRoutes(e, conn)
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
