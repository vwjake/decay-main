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

func (p Product) Price() string {
	if p.PriceCents%100 == 0 {
		return fmt.Sprintf("$%d", p.PriceCents/100)
	}
	return fmt.Sprintf("$%.2f", float64(p.PriceCents)/100)
}

type Post struct {
	ID          int64
	Slug        string
	Title       string
	Body        string
	PublishedAt *time.Time
	CreatedAt   time.Time
}

func (p Post) Published() bool { return p.PublishedAt != nil }

type Photo struct {
	ID       int64
	Filename string
	Caption  string
}

func scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var ev Event
		var startsAt string
		var endsAt sql.NullString
		if err := rows.Scan(&ev.ID, &ev.Title, &ev.EventType, &startsAt, &endsAt, &ev.Location, &ev.Description, &ev.Link); err != nil {
			return nil, err
		}
		var err error
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
		events = append(events, ev)
	}
	return events, rows.Err()
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

	all, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var events []Event
	for _, ev := range all {
		if ev.StartsAt.Before(now) {
			continue
		}
		events = append(events, ev)
		if len(events) == limit {
			break
		}
	}
	return events, nil
}

// ListAllEvents returns every event, most recent start time first, for
// admin management (unlike ListUpcomingEvents it includes past events).
func ListAllEvents(conn *sql.DB) ([]Event, error) {
	rows, err := conn.Query(`SELECT id, title, event_type, starts_at, ends_at, location, description, link FROM events ORDER BY starts_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
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

func scanPosts(rows *sql.Rows) ([]Post, error) {
	var posts []Post
	for rows.Next() {
		var p Post
		var publishedAt sql.NullString
		var createdAt string
		if err := rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &publishedAt, &createdAt); err != nil {
			return nil, err
		}
		if publishedAt.Valid {
			t, err := time.Parse(timeLayout, publishedAt.String)
			if err != nil {
				return nil, err
			}
			p.PublishedAt = &t
		}
		created, err := time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return nil, err
		}
		p.CreatedAt = created
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// ListPosts returns published posts only, newest first, for the public blog.
func ListPosts(conn *sql.DB) ([]Post, error) {
	rows, err := conn.Query(`SELECT id, slug, title, body_markdown, published_at, created_at FROM posts WHERE published_at IS NOT NULL ORDER BY published_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPosts(rows)
}

// ListAllPosts returns every post including drafts, newest first, for admin management.
func ListAllPosts(conn *sql.DB) ([]Post, error) {
	rows, err := conn.Query(`SELECT id, slug, title, body_markdown, published_at, created_at FROM posts ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPosts(rows)
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
