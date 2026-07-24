package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"decay-main/db"
	"decay-main/images"
	"decay-main/views"

	"github.com/labstack/echo/v4"
)

func registerProductRoutes(g *echo.Group, conn *sql.DB, uploadsDir string) {
	g.GET("/products", listProducts(conn))
	g.POST("/products", createProduct(conn))
	g.GET("/products/:id", editProduct(conn))
	g.POST("/products/:id", saveProduct(conn))
	g.POST("/products/:id/image", uploadProductImage(conn, uploadsDir))
	g.POST("/products/:id/delete", deleteProduct(conn))
}

// productsSubdir keeps shop photos apart from flyers and gallery photos.
const productsSubdir = "products"

func uploadProductImage(conn *sql.DB, uploadsDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		p, err := loadProduct(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminProductEdit(p, currentUser(c), msg).Render(c.Request().Context(), c.Response())
		}

		fileHeader, err := c.FormFile("image")
		if err != nil {
			return rerender("Choose an image to upload.")
		}
		dir := filepath.Join(uploadsDir, productsSubdir)
		filename, err := saveImage(fileHeader, dir)
		if err != nil {
			if errors.Is(err, errNotAnImage) {
				return rerender("That file doesn't look like an image. Use jpg, png, gif, or webp.")
			}
			return err
		}
		if err := images.MakeWeb(
			filepath.Join(dir, filename),
			filepath.Join(dir, "web", images.WebName(filename)),
		); err != nil {
			return err
		}

		previous, err := db.SetProductImage(conn, p.ID, filename)
		if err != nil {
			return err
		}
		if previous != "" && previous != filename {
			_ = os.Remove(filepath.Join(dir, previous))
			_ = os.Remove(filepath.Join(dir, "web", images.WebName(previous)))
		}
		return c.Redirect(http.StatusSeeOther, "/admin/products/"+c.Param("id"))
	}
}

func editProduct(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		p, err := loadProduct(conn, c.Param("id"))
		if err != nil {
			return err
		}
		return views.AdminProductEdit(p, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

func saveProduct(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		p, err := loadProduct(conn, c.Param("id"))
		if err != nil {
			return err
		}

		rerender := func(msg string) error {
			return views.AdminProductEdit(p, currentUser(c), msg).Render(c.Request().Context(), c.Response())
		}

		name := strings.TrimSpace(c.FormValue("name"))
		if name == "" {
			return rerender("Name is required.")
		}
		priceDollars, err := strconv.ParseFloat(c.FormValue("price"), 64)
		if err != nil || priceDollars < 0 {
			return rerender("Invalid price.")
		}
		placeholder := strings.TrimSpace(c.FormValue("placeholder"))
		if placeholder == "" {
			placeholder = "product photo"
		}

		p.Name = name
		p.PriceCents = int(priceDollars*100 + 0.5)
		p.Placeholder = placeholder
		p.StripeURL = strings.TrimSpace(c.FormValue("stripe_url"))
		p.Variants = strings.TrimSpace(c.FormValue("variants"))
		if err := db.UpdateProduct(conn, p); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/products")
	}
}

func loadProduct(conn *sql.DB, rawID string) (db.Product, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return db.Product{}, echo.NewHTTPError(http.StatusBadRequest)
	}
	p, err := db.ProductByID(conn, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Product{}, echo.NewHTTPError(http.StatusNotFound)
	}
	return p, err
}

func listProducts(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		products, err := db.ListProducts(conn)
		if err != nil {
			return err
		}
		return views.AdminProducts(products, currentUser(c), "").Render(c.Request().Context(), c.Response())
	}
}

func createProduct(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		name := c.FormValue("name")
		if name == "" {
			return rerenderProductsError(c, conn, "Name is required.")
		}
		priceDollars, err := strconv.ParseFloat(c.FormValue("price"), 64)
		if err != nil || priceDollars < 0 {
			return rerenderProductsError(c, conn, "Invalid price.")
		}

		placeholder := c.FormValue("placeholder")
		if placeholder == "" {
			placeholder = "product photo"
		}

		p := db.Product{
			Name:        name,
			PriceCents:  int(priceDollars*100 + 0.5),
			Placeholder: placeholder,
			StripeURL:   c.FormValue("stripe_url"),
		}
		if err := db.CreateProduct(conn, p); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/products")
	}
}

func deleteProduct(conn *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		if err := db.DeleteProduct(conn, id); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/products")
	}
}

func rerenderProductsError(c echo.Context, conn *sql.DB, msg string) error {
	products, err := db.ListProducts(conn)
	if err != nil {
		return err
	}
	return views.AdminProducts(products, currentUser(c), msg).Render(c.Request().Context(), c.Response())
}
