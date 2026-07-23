package main

import (
	"embed"
	"io/fs"
	"log"

	"decay-main/db"
	"decay-main/views"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed static
var staticFS embed.FS

func main() {
	conn, err := db.Open("decay.db")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	if err := db.Seed(conn); err != nil {
		log.Fatal(err)
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	static, _ := fs.Sub(staticFS, "static")
	e.StaticFS("/static", static)

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
