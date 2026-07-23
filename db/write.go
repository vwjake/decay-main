package db

import (
	"database/sql"
	"time"
)

func CreateEvent(conn *sql.DB, e Event) error {
	var endsAt any
	if e.EndsAt != nil {
		endsAt = e.EndsAt.Format(timeLayout)
	}
	_, err := conn.Exec(
		`INSERT INTO events (title, event_type, starts_at, ends_at, location, description, link) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Title, e.EventType, e.StartsAt.Format(timeLayout), endsAt, e.Location, e.Description, e.Link,
	)
	return err
}

func DeleteEvent(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM events WHERE id = ?`, id)
	return err
}

func CreateProduct(conn *sql.DB, p Product) error {
	_, err := conn.Exec(
		`INSERT INTO products (name, price_cents, placeholder, stripe_url) VALUES (?, ?, ?, ?)`,
		p.Name, p.PriceCents, p.Placeholder, p.StripeURL,
	)
	return err
}

func DeleteProduct(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM products WHERE id = ?`, id)
	return err
}

func CreatePost(conn *sql.DB, slug, title, body string, publish bool) error {
	var publishedAt any
	if publish {
		publishedAt = time.Now().UTC().Format(timeLayout)
	}
	_, err := conn.Exec(
		`INSERT INTO posts (slug, title, body_markdown, published_at) VALUES (?, ?, ?, ?)`,
		slug, title, body, publishedAt,
	)
	return err
}

func PublishPost(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`UPDATE posts SET published_at = ? WHERE id = ?`, time.Now().UTC().Format(timeLayout), id)
	return err
}

func DeletePost(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM posts WHERE id = ?`, id)
	return err
}

func CreatePhoto(conn *sql.DB, filename, caption string) error {
	_, err := conn.Exec(`INSERT INTO photos (filename, caption) VALUES (?, ?)`, filename, caption)
	return err
}

// DeletePhoto removes the DB row and returns its filename so the caller
// can also remove the file from the uploads directory.
func DeletePhoto(conn *sql.DB, id int64) (string, error) {
	var filename string
	if err := conn.QueryRow(`SELECT filename FROM photos WHERE id = ?`, id).Scan(&filename); err != nil {
		return "", err
	}
	if _, err := conn.Exec(`DELETE FROM photos WHERE id = ?`, id); err != nil {
		return "", err
	}
	return filename, nil
}
