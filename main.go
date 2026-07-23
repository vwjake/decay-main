package main

import (
	"crypto/rand"
	"embed"
	"io/fs"
	"log"
	"os"

	"decay-main/admin"
	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed static
var staticFS embed.FS

const uploadsDir = "uploads"

func main() {
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

	e.GET("/blog", func(c echo.Context) error {
		posts, err := db.ListPosts(conn)
		if err != nil {
			return err
		}
		return views.Blog(posts).Render(c.Request().Context(), c.Response())
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
