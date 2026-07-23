package db

import (
	"database/sql"
	_ "embed"
	"encoding/json"
)

// events.json is DECAY's real event archive, generated from the old PHP
// site by `go run ./cmd/importevents`.
//
//go:embed events.json
var eventsSeed []byte

type seedEvent struct {
	UID         string `json:"uid"`
	Title       string `json:"title"`
	EventType   string `json:"event_type"`
	StartsAt    string `json:"starts_at"`
	EndsAt      string `json:"ends_at"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Link        string `json:"link"`
	Slug        string `json:"slug"`
	Flyer       string `json:"flyer"`
	Volunteers  []struct {
		Role string `json:"role"`
		Name string `json:"name"`
	} `json:"volunteers"`
}

// Seed populates events and products on first run only, so it's safe
// to call on every startup.
func Seed(conn *sql.DB) error {
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM events`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		var events []seedEvent
		if err := json.Unmarshal(eventsSeed, &events); err != nil {
			return err
		}
		for _, ev := range events {
			// ends_at is genuinely unknown for some events, so it stays
			// NULL rather than becoming an empty timestamp string.
			var endsAt any
			if ev.EndsAt != "" {
				endsAt = ev.EndsAt
			}
			link := ev.Link
			if link == "" {
				link = "#"
			}
			res, err := conn.Exec(
				`INSERT INTO events (title, event_type, starts_at, ends_at, location, description, link, uid, slug, flyer) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				ev.Title, ev.EventType, ev.StartsAt, endsAt, ev.Location, ev.Description, link, ev.UID, ev.Slug, ev.Flyer,
			)
			if err != nil {
				return err
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			for _, v := range ev.Volunteers {
				if err := AddVolunteer(conn, id, v.Role, v.Name); err != nil {
					return err
				}
			}
		}
	}

	if err := conn.QueryRow(`SELECT count(*) FROM products`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		products := []struct {
			name        string
			priceCents  int
			placeholder string
		}{
			{"Logo Tee", 2800, "product photo"},
			{"Static Hoodie", 5800, "product photo"},
			{"Enamel Pin", 1000, "product photo"},
			{"Risograph Print", 1800, "product photo"},
		}
		for _, p := range products {
			if _, err := conn.Exec(
				`INSERT INTO products (name, price_cents, placeholder) VALUES (?, ?, ?)`,
				p.name, p.priceCents, p.placeholder,
			); err != nil {
				return err
			}
		}
	}

	return nil
}
