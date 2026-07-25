package main

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	// Event times are resolved against the venue's timezone, and Windows
	// has no system timezone database to read it from.
	_ "time/tzdata"

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

	// Accounts live in the database. ADMIN_USERNAME/ADMIN_PASSWORD only
	// seed the very first one — after that, accounts are managed at
	// /admin/users and these are ignored.
	if err := bootstrapAdmin(conn); err != nil {
		log.Fatal(err)
	}

	// The calendar grid is laid out in the venue's own timezone, so a
	// late show lands on the day it started rather than the UTC one.
	venue, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Fatal(err)
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

	admin.Register(e, conn, sessionSecret(), uploadsDir)

	e.GET("/", func(c echo.Context) error {
		events, err := db.ListUpcomingEvents(conn, 4)
		if err != nil {
			return err
		}
		// The home preview shows only what can be bought; the full shop
		// page carries the sold-out items too.
		products, err := db.AvailableProducts(conn)
		if err != nil {
			return err
		}
		return views.Home(events, products).Render(c.Request().Context(), c.Response())
	})

	e.GET("/about", func(c echo.Context) error {
		people, err := db.ListPeople(conn)
		if err != nil {
			return err
		}
		return views.About(people).Render(c.Request().Context(), c.Response())
	})

	e.GET("/support", func(c echo.Context) error {
		return views.Support().Render(c.Request().Context(), c.Response())
	})

	e.GET("/policies", func(c echo.Context) error {
		return views.Policies().Render(c.Request().Context(), c.Response())
	})

	e.GET("/events", func(c echo.Context) error {
		events, err := db.UpcomingEvents(conn)
		if err != nil {
			return err
		}
		events, page := db.Paginate(events, db.PageNumber(c.QueryParam("page")), db.PerPagePublic)
		page.Path = "/events"
		return views.Events(db.GroupByMonth(events), page).Render(c.Request().Context(), c.Response())
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

	e.GET("/calendar", func(c echo.Context) error {
		month := db.ParseMonth(c.QueryParam("month"), venue)
		events, err := db.EventsInMonth(conn, month)
		if err != nil {
			return err
		}
		return views.Calendar(db.BuildCalendar(events, month, venue)).Render(c.Request().Context(), c.Response())
	})

	e.GET("/events/archive", func(c echo.Context) error {
		events, err := db.PastEvents(conn)
		if err != nil {
			return err
		}
		events, page := db.Paginate(events, db.PageNumber(c.QueryParam("page")), db.PerPagePublic)
		page.Path = "/events/archive"
		return views.EventArchive(db.GroupByMonth(events), page).Render(c.Request().Context(), c.Response())
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

	e.GET("/shop", func(c echo.Context) error {
		products, err := db.ListProducts(conn)
		if err != nil {
			return err
		}
		return views.Shop(products).Render(c.Request().Context(), c.Response())
	})

	e.GET("/groups", func(c echo.Context) error {
		groups, err := db.EnabledGroups(conn)
		if err != nil {
			return err
		}
		return views.Groups(groups).Render(c.Request().Context(), c.Response())
	})

	e.GET("/groups/:slug", func(c echo.Context) error {
		group, err := db.GroupBySlug(conn, c.Param("slug"))
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}
		upcoming, err := db.UpcomingForGroup(conn, group, 6)
		if err != nil {
			return err
		}
		return views.GroupPage(group, upcoming).Render(c.Request().Context(), c.Response())
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

	// PORT lets the host pick the listen port; it defaults to 8080 for
	// local development.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	e.Logger.Fatal(e.Start(":" + port))
}

// bootstrapAdmin creates the first master account from the environment
// when the database has none. Once any account exists this does nothing,
// so ADMIN_PASSWORD stops being a live credential the moment the panel is
// set up — it can't be used to log in, only to create that first account.
func bootstrapAdmin(conn *sql.DB) error {
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		return errors.New("no accounts exist yet — set ADMIN_PASSWORD to create the first one (see .env.example)")
	}
	username := os.Getenv("ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}

	created, err := db.EnsureFirstUser(conn, username, password)
	if err != nil {
		return fmt.Errorf("creating the first account: %w", err)
	}
	if created {
		log.Printf("created the first admin account %q from ADMIN_USERNAME/ADMIN_PASSWORD — "+
			"manage accounts at /admin/users from now on", username)
		if len([]rune(password)) < db.MinPasswordLength {
			log.Printf("WARNING: that password is under %d characters. Change it at /admin/users; "+
				"ADMIN_PASSWORD is ignored from here on.", db.MinPasswordLength)
		}
	}
	return nil
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
