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

// products.json is DECAY's real merch, generated from the shop export by
// `go run ./cmd/importshop`. Unlike the flyers, the photos it names are
// committed (uploads/products/), so a fresh checkout renders the real shop
// rather than falling back to placeholder text.
//
//go:embed products.json
var productsSeed []byte

type seedProduct struct {
	Name       string `json:"name"`
	PriceCents int    `json:"price_cents"`
	Image      string `json:"image"`
	Variants   string `json:"variants"`
}

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
	// Roles the event needed covered. The seed never carries who filled
	// them — that's recorded in the admin panel, not in this file.
	Volunteers []struct {
		Role string `json:"role"`
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
				if err := AddVolunteer(conn, id, v.Role, ""); err != nil {
					return err
				}
			}
		}
	}

	// The board members the old About page hard-coded, so switching that
	// list to real profiles doesn't blank the page. Bios and photos are
	// filled in from the admin panel; role and order are seeded here.
	if err := conn.QueryRow(`SELECT count(*) FROM people`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		board := []string{
			"Moon Fery", "Abe Burt",
			"Liam Mooney", "Ray Malmrose", "Heather Hemann",
		}
		for i, name := range board {
			if _, err := conn.Exec(
				`INSERT INTO people (name, role, position) VALUES (?, 'Board of Directors', ?)`,
				name, i,
			); err != nil {
				return err
			}
		}
	}

	if err := seedGroups(conn); err != nil {
		return err
	}
	if err := backfillGroupMatchTerms(conn); err != nil {
		return err
	}
	if err := migrateGroupsToCategories(conn); err != nil {
		return err
	}

	if err := conn.QueryRow(`SELECT count(*) FROM products`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		var products []seedProduct
		if err := json.Unmarshal(productsSeed, &products); err != nil {
			return err
		}
		for _, p := range products {
			if _, err := conn.Exec(
				`INSERT INTO products (name, price_cents, placeholder, image, variants) VALUES (?, ?, ?, ?, ?)`,
				p.Name, p.PriceCents, "product photo", p.Image, p.Variants,
			); err != nil {
				return err
			}
		}
	}
	if err := backfillProductVariants(conn); err != nil {
		return err
	}

	return nil
}

// backfillProductVariants fills in the seed's variant text on databases
// that were populated before it was written — the block above only runs on
// an empty table, so an existing shop would never see it otherwise. Like
// backfillGroupMatchTerms it only touches rows still left blank, so an
// admin's own wording stands.
func backfillProductVariants(conn *sql.DB) error {
	var products []seedProduct
	if err := json.Unmarshal(productsSeed, &products); err != nil {
		return err
	}
	for _, p := range products {
		if p.Variants == "" {
			continue
		}
		if _, err := conn.Exec(
			`UPDATE products SET variants = ? WHERE name = ? AND variants = ''`,
			p.Variants, p.Name,
		); err != nil {
			return err
		}
	}
	return nil
}
