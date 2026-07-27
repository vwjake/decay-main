package db

import (
	"database/sql"
	"strings"
)

// CommunityBio is a profile of someone in the wider DECAY community, shown on
// the public /bios page. Broader than the board/staff People table and entered
// by admins rather than self-submitted.
type CommunityBio struct {
	ID       int64
	Name     string
	Pronouns string
	Role     string
	Bio      string
	// Public is whether it shows on the site; a false one stays on file
	// (e.g. gathered for a grant application) without being published.
	Public   bool
	Position int
}

func (b CommunityBio) HasPronouns() bool { return b.Pronouns != "" }
func (b CommunityBio) HasRole() bool     { return b.Role != "" }

// Paragraphs splits a bio into blocks on blank lines, so a template can render
// it without handing raw HTML to the browser.
func (b CommunityBio) Paragraphs() []string {
	var out []string
	for _, block := range strings.Split(strings.ReplaceAll(b.Bio, "\r\n", "\n"), "\n\n") {
		if block = strings.TrimSpace(block); block != "" {
			out = append(out, block)
		}
	}
	return out
}

const communityBioColumns = `id, name, pronouns, role, bio, public, position`

func scanCommunityBios(rows *sql.Rows) ([]CommunityBio, error) {
	var out []CommunityBio
	for rows.Next() {
		var b CommunityBio
		if err := rows.Scan(&b.ID, &b.Name, &b.Pronouns, &b.Role, &b.Bio, &b.Public, &b.Position); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListCommunityBios returns every bio for the admin screen, in display order.
func ListCommunityBios(conn *sql.DB) ([]CommunityBio, error) {
	rows, err := conn.Query(`SELECT ` + communityBioColumns + ` FROM community_bios ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommunityBios(rows)
}

// PublicCommunityBios returns just the bios shown on the site, in order.
func PublicCommunityBios(conn *sql.DB) ([]CommunityBio, error) {
	rows, err := conn.Query(`SELECT ` + communityBioColumns + ` FROM community_bios WHERE public = 1 ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommunityBios(rows)
}

// CommunityBioByID fetches one bio for editing.
func CommunityBioByID(conn *sql.DB, id int64) (CommunityBio, error) {
	rows, err := conn.Query(`SELECT `+communityBioColumns+` FROM community_bios WHERE id = ?`, id)
	if err != nil {
		return CommunityBio{}, err
	}
	defer rows.Close()
	bios, err := scanCommunityBios(rows)
	if err != nil {
		return CommunityBio{}, err
	}
	if len(bios) == 0 {
		return CommunityBio{}, sql.ErrNoRows
	}
	return bios[0], nil
}

// CreateCommunityBio inserts a bio and returns its new id.
func CreateCommunityBio(conn *sql.DB, b CommunityBio) (int64, error) {
	res, err := conn.Exec(
		`INSERT INTO community_bios (name, pronouns, role, bio, public, position) VALUES (?, ?, ?, ?, ?, ?)`,
		b.Name, b.Pronouns, b.Role, b.Bio, b.Public, b.Position,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateCommunityBio saves edits to a bio.
func UpdateCommunityBio(conn *sql.DB, b CommunityBio) error {
	_, err := conn.Exec(
		`UPDATE community_bios SET name = ?, pronouns = ?, role = ?, bio = ?, public = ?, position = ? WHERE id = ?`,
		b.Name, b.Pronouns, b.Role, b.Bio, b.Public, b.Position, b.ID,
	)
	return err
}

// DeleteCommunityBio removes a bio.
func DeleteCommunityBio(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM community_bios WHERE id = ?`, id)
	return err
}
