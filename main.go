package main

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"

	"decay-main/admin"
	"decay-main/db"
	"decay-main/ics"
	"decay-main/views"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed static
var staticFS embed.FS

const uploadsDir = "uploads"

func main() {
	// Loads .env into the process environment if the file exists; a
	// missing .env (e.g. in production, where vars are set directly) is
	// not an error.
	_ = godotenv.Load()

	conn, err := db.Open("decay.db")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	if err := db.Seed(conn); err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		log.Fatal(err)
	}

	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		log.Fatal("ADMIN_PASSWORD must be set to run the admin panel (see .env.example)")
	}
	adminUsername := os.Getenv("ADMIN_USERNAME")
	if adminUsername == "" {
		adminUsername = "admin"
	}

	// Calendar subscribers keep absolute links, so the feed needs to know
	// where the site actually lives.
	siteURL := os.Getenv("SITE_URL")
	if siteURL == "" {
		siteURL = "http://localhost:8080"
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	static, _ := fs.Sub(staticFS, "static")
	e.StaticFS("/static", static)
	e.Static("/uploads", uploadsDir)

	admin.Register(e, conn, admin.Config{
		Username: adminUsername,
		Password: adminPassword,
	}, sessionSecret(), uploadsDir)

	e.GET("/", func(c echo.Context) error {
		events, err := db.ListUpcomingEvents(conn, 4)
		if err != nil {
			return err
		}
		products, err := db.ListProducts(conn)
		if err != nil {
			return err
		}
		return views.Home(events, products).Render(c.Request().Context(), c.Response())
	})

	e.GET("/events", func(c echo.Context) error {
		events, err := db.UpcomingEvents(conn)
		if err != nil {
			return err
		}
		return views.Events(db.GroupByMonth(events)).Render(c.Request().Context(), c.Response())
	})

	// The subscribable feed carries the whole calendar, past included, so
	// there's no window rule for subscribers to be surprised by.
	e.GET("/events.ics", func(c echo.Context) error {
		events, err := db.ListAllEvents(conn)
		if err != nil {
			return err
		}
		return c.Blob(http.StatusOK, "text/calendar; charset=utf-8", ics.Calendar("DECAY", siteURL, events))
	})

	e.GET("/events/archive", func(c echo.Context) error {
		events, err := db.PastEvents(conn)
		if err != nil {
			return err
		}
		return views.EventArchive(db.GroupByMonth(events)).Render(c.Request().Context(), c.Response())
	})

	e.GET("/events/:slug", func(c echo.Context) error {
		ev, err := db.EventBySlug(conn, c.Param("slug"))
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}
		volunteers, err := db.VolunteersFor(conn, ev.ID)
		if err != nil {
			return err
		}
		return views.EventDetail(ev, db.OpenRoles(volunteers)).Render(c.Request().Context(), c.Response())
	})

	e.GET("/blog", func(c echo.Context) error {
		posts, err := db.ListPosts(conn)
		if err != nil {
			return err
		}
		return views.Blog(posts).Render(c.Request().Context(), c.Response())
	})

	e.GET("/blog/:slug", func(c echo.Context) error {
		post, err := db.PostBySlug(conn, c.Param("slug"))
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}
		return views.PostPage(post).Render(c.Request().Context(), c.Response())
	})

	e.GET("/photos", func(c echo.Context) error {
		photos, err := db.ListPhotos(conn)
		if err != nil {
			return err
		}
		return views.Photos(photos).Render(c.Request().Context(), c.Response())
	})

	e.Logger.Fatal(e.Start(":8080"))
}

func sessionSecret() []byte {
	if s := os.Getenv("SESSION_SECRET"); s != "" {
		return []byte(s)
	}
	log.Println("SESSION_SECRET not set — generating an ephemeral one; admin sessions won't survive a restart. Set SESSION_SECRET in production.")
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatal(err)
	}
	return secret
}
