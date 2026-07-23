package main

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

type Event struct {
	Day, Month, Title, Type, Time, Link string
}

type MerchItem struct {
	Name, Price, Placeholder string
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func main() {
	e := echo.New()
	e.Renderer = &Template{templates: template.Must(template.ParseFS(templatesFS, "templates/*.html"))}

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	static, _ := fs.Sub(staticFS, "static")
	e.StaticFS("/static", static)

	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "index.html", map[string]interface{}{
			"Events": []Event{
				{Day: "25", Month: "JUL", Title: "Free Mask Distro!", Type: "Meetup", Time: "4 – 6 PM", Link: "#"},
				{Day: "26", Month: "JUL", Title: "Circuit Bending Workshop", Type: "Tech", Time: "3 – 6 PM", Link: "#"},
				{Day: "28", Month: "JUL", Title: "Movie Club", Type: "Film", Time: "7 – 9 PM", Link: "#"},
				{Day: "30", Month: "JUL", Title: "NO_TAPE", Type: "Workshop", Time: "7:30 PM", Link: "#"},
			},
			"Merch": []MerchItem{
				{Name: "Logo Tee", Price: "$28", Placeholder: "product photo"},
				{Name: "Static Hoodie", Price: "$58", Placeholder: "product photo"},
				{Name: "Enamel Pin", Price: "$10", Placeholder: "product photo"},
				{Name: "Risograph Print", Price: "$18", Placeholder: "product photo"},
			},
		})
	})

	e.Logger.Fatal(e.Start(":8080"))
}
