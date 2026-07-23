package db

import "database/sql"

// Seed populates events and products on first run only, so it's safe
// to call on every startup.
func Seed(conn *sql.DB) error {
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM events`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		events := []struct {
			title, eventType, startsAt, endsAt string
		}{
			{"Free Mask Distro!", "Meetup", "2026-07-25T16:00:00-07:00", "2026-07-25T18:00:00-07:00"},
			{"Circuit Bending Workshop", "Tech", "2026-07-26T15:00:00-07:00", "2026-07-26T18:00:00-07:00"},
			{"Movie Club", "Film", "2026-07-28T19:00:00-07:00", "2026-07-28T21:00:00-07:00"},
			{"NO_TAPE", "Workshop", "2026-07-30T19:30:00-07:00", ""},
		}
		for _, ev := range events {
			var endsAt any
			if ev.endsAt != "" {
				endsAt = ev.endsAt
			}
			if _, err := conn.Exec(
				`INSERT INTO events (title, event_type, starts_at, ends_at, location) VALUES (?, ?, ?, ?, ?)`,
				ev.title, ev.eventType, ev.startsAt, endsAt, "402 Washington St NE, Olympia WA",
			); err != nil {
				return err
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
