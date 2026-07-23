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
				{Day: "08", Month: "AUG", Title: "Open Studio Night", Type: "Studio", Time: "7 – 11 PM", Link: "#"},
				{Day: "15", Month: "AUG", Title: "Noise / Feedback Showcase", Type: "Live Show", Time: "8 PM", Link: "#"},
				{Day: "22", Month: "AUG", Title: "Screenprinting Workshop", Type: "Workshop", Time: "2 – 5 PM", Link: "#"},
				{Day: "30", Month: "AUG", Title: "Member Group Show Opening", Type: "Exhibition", Time: "6 PM", Link: "#"},
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
