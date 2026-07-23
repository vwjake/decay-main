package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const timeLayout = "2006-01-02T15:04:05-07:00"

type Event struct {
	ID          int64
	Title       string
	EventType   string
	StartsAt    time.Time
	EndsAt      *time.Time
	Location    string
	Description string
	Link        string
}

func (e Event) Day() string   { return e.StartsAt.Format("02") }
func (e Event) Month() string { return strings.ToUpper(e.StartsAt.Format("Jan")) }

func (e Event) TimeRange() string {
	start := formatClock(e.StartsAt)
	if e.EndsAt != nil {
		return start + " – " + formatClock(*e.EndsAt)
	}
	return start
}

func formatClock(t time.Time) string {
	if t.Minute() == 0 {
		return t.Format("3 PM")
	}
	return t.Format("3:04 PM")
}

type Product struct {
	ID          int64
	Name        string
	PriceCents  int
	Placeholder string
	StripeURL   string
}

func (p Product) Price() string { return fmt.Sprintf("$%d", p.PriceCents/100) }

type Post struct {
	ID          int64
	Slug        string
	Title       string
	Body        string
	PublishedAt *time.Time
}

type Photo struct {
	ID       int64
	Filename string
	Caption  string
}

// ListUpcomingEvents returns up to limit events starting now or later,
// soonest first. Filtering happens in Go rather than SQL so the
// comparison is against real time.Time instants, not SQLite's UTC clock.
func ListUpcomingEvents(conn *sql.DB, limit int) ([]Event, error) {
	rows, err := conn.Query(`SELECT id, title, event_type, starts_at, ends_at, location, description, link FROM events ORDER BY starts_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	var events []Event
	for rows.Next() {
		var ev Event
		var startsAt string
		var endsAt sql.NullString
		if err := rows.Scan(&ev.ID, &ev.Title, &ev.EventType, &startsAt, &endsAt, &ev.Location, &ev.Description, &ev.Link); err != nil {
			return nil, err
		}
		ev.StartsAt, err = time.Parse(timeLayout, startsAt)
		if err != nil {
			return nil, err
		}
		if endsAt.Valid {
			end, err := time.Parse(timeLayout, endsAt.String)
			if err != nil {
				return nil, err
			}
			ev.EndsAt = &end
		}
		if ev.StartsAt.Before(now) {
			continue
		}
		events = append(events, ev)
		if len(events) == limit {
			break
		}
	}
	return events, rows.Err()
}

func ListProducts(conn *sql.DB) ([]Product, error) {
	rows, err := conn.Query(`SELECT id, name, price_cents, placeholder, stripe_url FROM products ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.PriceCents, &p.Placeholder, &p.StripeURL); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func ListPosts(conn *sql.DB) ([]Post, error) {
	rows, err := conn.Query(`SELECT id, slug, title, body_markdown, published_at FROM posts ORDER BY published_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		var publishedAt sql.NullString
		if err := rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &publishedAt); err != nil {
			return nil, err
		}
		if publishedAt.Valid {
			t, err := time.Parse(timeLayout, publishedAt.String)
			if err != nil {
				return nil, err
			}
			p.PublishedAt = &t
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func ListPhotos(conn *sql.DB) ([]Photo, error) {
	rows, err := conn.Query(`SELECT id, filename, caption FROM photos ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.Filename, &p.Caption); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}
