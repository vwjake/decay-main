package db

import (
	"database/sql"
	_ "embed"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

func Open(path string) (*sql.DB, error) {
	// Enforce foreign keys, which SQLite leaves off by default — without
	// this the schema's ON DELETE CASCADE / SET NULL clauses do nothing.
	// The _pragma runs on every pooled connection, not just the first.
	conn, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, err
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// migrate adds columns that schema.sql gained after a database was first
// created — CREATE TABLE IF NOT EXISTS leaves existing tables alone, and
// SQLite has no ADD COLUMN IF NOT EXISTS, so each one is checked first.
func migrate(conn *sql.DB) error {
	migrations := []struct{ table, column, ddl string }{
		{"events", "uid", `ALTER TABLE events ADD COLUMN uid TEXT NOT NULL DEFAULT ''`},
		{"events", "flyer", `ALTER TABLE events ADD COLUMN flyer TEXT NOT NULL DEFAULT ''`},
		{"events", "slug", `ALTER TABLE events ADD COLUMN slug TEXT NOT NULL DEFAULT ''`},
		{"products", "image", `ALTER TABLE products ADD COLUMN image TEXT NOT NULL DEFAULT ''`},
		{"products", "variants", `ALTER TABLE products ADD COLUMN variants TEXT NOT NULL DEFAULT ''`},
		{"groups", "match_terms", `ALTER TABLE groups ADD COLUMN match_terms TEXT NOT NULL DEFAULT ''`},
		{"photos", "group_id", `ALTER TABLE photos ADD COLUMN group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL`},
		{"products", "description", `ALTER TABLE products ADD COLUMN description TEXT NOT NULL DEFAULT ''`},
		{"products", "sold_out", `ALTER TABLE products ADD COLUMN sold_out INTEGER NOT NULL DEFAULT 0`},
		{"products", "position", `ALTER TABLE products ADD COLUMN position INTEGER NOT NULL DEFAULT 0`},
		{"products", "stripe_price_id", `ALTER TABLE products ADD COLUMN stripe_price_id TEXT NOT NULL DEFAULT ''`},
		{"users", "photo", `ALTER TABLE users ADD COLUMN photo TEXT NOT NULL DEFAULT ''`},
		{"users", "blurb", `ALTER TABLE users ADD COLUMN blurb TEXT NOT NULL DEFAULT ''`},
	}
	for _, m := range migrations {
		has, err := hasColumn(conn, m.table, m.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := conn.Exec(m.ddl); err != nil {
			return err
		}
	}
	return nil
}

func hasColumn(conn *sql.DB, table, column string) (bool, error) {
	rows, err := conn.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
