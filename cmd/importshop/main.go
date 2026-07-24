// Command importshop pulls DECAY's merch into db/products.json from the
// shop.decay.events export — product names, prices, and photos only. The
// shop itself stays where it is; this site just shows what's for sale and
// links out.
//
//	go run ./cmd/importshop -export ../shop-decay-events-export-2026-07-24-002012
//
// Two things about the source data are worth knowing. The point-of-sale
// export lists prices *without* Washington sales tax, which is why a
// $30 shirt appears as 27.32 — the tax rate is a column in the same file
// and is applied back here so the site shows the price people actually
// pay. And the export has no image column at all, so products are matched
// to photos by the table below.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"decay-main/images"
)

// SeedProduct is one row of db/products.json.
type SeedProduct struct {
	Name     string `json:"name"`
	Price    int    `json:"price_cents"`
	Image    string `json:"image,omitempty"`
	Variants string `json:"variants,omitempty"`
}

// productImages maps a product name from the export to a file in the
// export's WordPress uploads. The point-of-sale CSV carries no image
// reference, so this is the join, and it's by hand.
var productImages = map[string]string{
	"Tshirt - LOGO":  "2025/11/File_000.png",
	"Tshirt - STAMP": "2025/11/STAMP_Logo.png",
	"Tshirt - SKULL": "2025/11/IMG_5590.jpg",
	"Tote":           "2025/11/TOTE-GLOBE.png",
	"Bandana":        "2025/11/Bandana-BKBU.png",
}

// displayNames turn point-of-sale shorthand into something that reads on
// a website.
var displayNames = map[string]string{
	"Tshirt - LOGO":  "Logo T-Shirt",
	"Tshirt - STAMP": "Stamp T-Shirt",
	"Tshirt - SKULL": "Skull T-Shirt",
	"Tote":           "Canvas Tote",
	"Bandana":        "Bandana",
}

// sellOnline is the category worth listing. Concessions (popcorn, coffee)
// are sold at the door, and Donations aren't merchandise.
const sellOnline = "Merch"

func main() {
	export := flag.String("export", filepath.Join("..", "shop-decay-events-export-2026-07-24-002012"), "unpacked shop.decay.events export")
	out := flag.String("out", filepath.Join("db", "products.json"), "seed file to write")
	imagesOut := flag.String("images-out", filepath.Join("uploads", "products"), "where to copy product photos (\"\" to skip)")
	flag.Parse()

	uploads := filepath.Join(*export, "shop.decay.events", "wp-content", "uploads")

	csvPath, err := newestExport(filepath.Join(uploads, "wc-imports"))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("reading %s", csvPath)

	products, err := readProducts(csvPath)
	if err != nil {
		log.Fatal(err)
	}

	copied, err := copyImages(products, uploads, *imagesOut)
	if err != nil {
		log.Fatal(err)
	}

	data, err := json.MarshalIndent(products, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d products to %s, copied %d photos to %s\n", len(products), *out, copied, *imagesOut)
}

// newestExport picks the richest product export in the directory. The
// shop wrote several, and only the widest one carries categories and
// variant options.
func newestExport(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "product-export_*.csv"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no product export found in %s", dir)
	}

	best, bestCols := "", -1
	for _, path := range matches {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		header, err := csv.NewReader(f).Read()
		f.Close()
		if err != nil {
			continue
		}
		if len(header) > bestCols {
			best, bestCols = path, len(header)
		}
	}
	if best == "" {
		return "", fmt.Errorf("no readable product export in %s", dir)
	}
	return best, nil
}

func readProducts(path string) ([]SeedProduct, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("%s has no rows", path)
	}

	index := map[string]int{}
	for i, name := range records[0] {
		// The export is written with a UTF-8 byte-order mark, which
		// otherwise ends up glued to the first column's name.
		index[strings.TrimSpace(strings.TrimPrefix(name, "\ufeff"))] = i
	}
	get := func(row []string, col string) string {
		i, ok := index[col]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	taxRate, taxColumn := 0.0, ""
	for name := range index {
		// The tax column is named for the jurisdiction and rate, e.g.
		// "Olympia, WA (9.8%)".
		if open := strings.Index(name, "("); open >= 0 && strings.HasSuffix(name, "%)") {
			if rate, err := strconv.ParseFloat(name[open+1:len(name)-2], 64); err == nil {
				taxRate, taxColumn = rate/100, name
			}
		}
	}
	if taxColumn == "" {
		return nil, fmt.Errorf("no tax-rate column found in %s", path)
	}
	log.Printf("applying %s to the listed prices", taxColumn)

	// Variant rows repeat under a blank name, so the last non-empty name
	// carries down the file.
	type gathered struct {
		category string
		prices   []float64
		taxed    bool
		options  map[string][]string
		order    []string
	}
	byName := map[string]*gathered{}
	var order []string
	current := ""

	for _, row := range records[1:] {
		if name := get(row, "Name"); name != "" {
			current = name
		}
		if current == "" {
			continue
		}
		g, ok := byName[current]
		if !ok {
			g = &gathered{options: map[string][]string{}}
			byName[current] = g
			order = append(order, current)
		}
		if cat := get(row, "Category"); cat != "" {
			g.category = cat
		}
		if strings.EqualFold(get(row, taxColumn), "YES") {
			g.taxed = true
		}
		if raw := get(row, "Price"); raw != "" {
			if price, err := strconv.ParseFloat(raw, 64); err == nil && price > 0 {
				g.prices = append(g.prices, price)
			}
		}
		for _, pair := range [][2]string{
			{"Option1 Name", "Option1 Value"},
			{"Option2 Name", "Option2 Value"},
			{"Option3 Name", "Option3 Value"},
		} {
			label, value := get(row, pair[0]), get(row, pair[1])
			if label == "" || value == "" {
				continue
			}
			if _, seen := g.options[label]; !seen {
				g.order = append(g.order, label)
			}
			if !contains(g.options[label], value) {
				g.options[label] = append(g.options[label], value)
			}
		}
	}

	var products []SeedProduct
	for _, name := range order {
		g := byName[name]
		if !strings.EqualFold(g.category, sellOnline) || len(g.prices) == 0 {
			continue
		}

		// Every variant of a product is the same price in this catalogue;
		// take the highest so a listing never under-promises.
		sort.Float64s(g.prices)
		price := g.prices[len(g.prices)-1]
		if g.taxed {
			price *= 1 + taxRate
		}

		display := displayNames[name]
		if display == "" {
			display = name
		}
		products = append(products, SeedProduct{
			Name:     display,
			Price:    int(math.Round(price * 100)),
			Image:    filepath.Base(productImages[name]),
			Variants: describeVariants(g.order, g.options),
		})
	}
	return products, nil
}

// describeVariants renders the option lists as one readable line, e.g.
// "Size: S, M, L · Style: Front Print, Back Print".
func describeVariants(order []string, options map[string][]string) string {
	var parts []string
	for _, label := range order {
		values := options[label]
		if len(values) == 0 {
			continue
		}
		parts = append(parts, label+": "+strings.Join(values, ", "))
	}
	return strings.Join(parts, " · ")
}

// copyImages pulls each product's photo out of the export's WordPress
// uploads and makes a web-sized copy alongside it, the same way flyers
// work.
func copyImages(products []SeedProduct, uploads, dest string) (int, error) {
	if dest == "" {
		return 0, nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, err
	}

	byBase := map[string]string{}
	for _, rel := range productImages {
		byBase[filepath.Base(rel)] = rel
	}

	copied := 0
	for i := range products {
		name := products[i].Image
		if name == "" {
			continue
		}
		from := filepath.Join(uploads, filepath.FromSlash(byBase[name]))
		if _, err := os.Stat(from); err != nil {
			log.Printf("photo missing, listing %s without one: %s", products[i].Name, name)
			products[i].Image = ""
			continue
		}
		to := filepath.Join(dest, name)
		if err := copyFile(from, to); err != nil {
			return copied, err
		}
		if err := images.MakeWeb(to, filepath.Join(dest, "web", images.WebName(name))); err != nil {
			return copied, fmt.Errorf("resizing %s: %w", name, err)
		}
		copied++
	}
	return copied, nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(to)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
