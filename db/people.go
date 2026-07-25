package db

import (
	"database/sql"
	"path"
	"strings"
)

// PeopleSubdir keeps board/staff portraits apart from flyers, product
// shots, and gallery photos inside the uploads directory.
const PeopleSubdir = "people"

// Person is one board member or staff profile shown on the About page.
type Person struct {
	ID       int64
	Name     string
	Pronouns string
	Role     string
	Bio      string
	// Photo is a filename under uploads/people/, empty when there's none.
	Photo string
	// Position orders the list; lower comes first, so the board can be
	// placed ahead of staff without a rigid category.
	Position int
}

func (p Person) HasPhoto() bool { return p.Photo != "" }

// PhotoPath is the web-sized copy the page displays; portraits come off a
// phone at full resolution like gallery shots do.
func (p Person) PhotoPath() string {
	return "/uploads/" + PeopleSubdir + "/web/" + strings.TrimSuffix(p.Photo, path.Ext(p.Photo)) + ".jpg"
}

// HasPronouns and HasRole report whether the optional lines are worth
// rendering.
func (p Person) HasPronouns() bool { return p.Pronouns != "" }
func (p Person) HasRole() bool     { return p.Role != "" }

// Paragraphs splits a bio into blocks on blank lines, so a template can
// render it without handing raw HTML to the browser.
func (p Person) Paragraphs() []string {
	var out []string
	for _, block := range strings.Split(strings.ReplaceAll(p.Bio, "\r\n", "\n"), "\n\n") {
		if block = strings.TrimSpace(block); block != "" {
			out = append(out, block)
		}
	}
	return out
}

const personColumns = `id, name, pronouns, role, bio, photo, position`

func scanPeople(rows *sql.Rows) ([]Person, error) {
	var people []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.Pronouns, &p.Role, &p.Bio, &p.Photo, &p.Position); err != nil {
			return nil, err
		}
		people = append(people, p)
	}
	return people, rows.Err()
}

// ListPeople returns every profile in display order — by position, then by
// id so profiles sharing a position keep a stable order.
func ListPeople(conn *sql.DB) ([]Person, error) {
	rows, err := conn.Query(`SELECT ` + personColumns + ` FROM people ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPeople(rows)
}

// PersonByID fetches one profile for editing.
func PersonByID(conn *sql.DB, id int64) (Person, error) {
	rows, err := conn.Query(`SELECT `+personColumns+` FROM people WHERE id = ?`, id)
	if err != nil {
		return Person{}, err
	}
	defer rows.Close()
	people, err := scanPeople(rows)
	if err != nil {
		return Person{}, err
	}
	if len(people) == 0 {
		return Person{}, sql.ErrNoRows
	}
	return people[0], nil
}

// CreatePerson inserts a profile and returns its new id.
func CreatePerson(conn *sql.DB, p Person) (int64, error) {
	res, err := conn.Exec(
		`INSERT INTO people (name, pronouns, role, bio, position) VALUES (?, ?, ?, ?, ?)`,
		p.Name, p.Pronouns, p.Role, p.Bio, p.Position,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdatePerson saves edits to a profile's text fields. The photo is set
// separately through SetPersonPhoto.
func UpdatePerson(conn *sql.DB, p Person) error {
	_, err := conn.Exec(
		`UPDATE people SET name = ?, pronouns = ?, role = ?, bio = ?, position = ? WHERE id = ?`,
		p.Name, p.Pronouns, p.Role, p.Bio, p.Position, p.ID,
	)
	return err
}

// SetPersonPhoto points a profile at an uploaded portrait, returning the
// filename it replaced so the caller can delete it.
func SetPersonPhoto(conn *sql.DB, id int64, filename string) (string, error) {
	var previous string
	if err := conn.QueryRow(`SELECT photo FROM people WHERE id = ?`, id).Scan(&previous); err != nil {
		return "", err
	}
	if _, err := conn.Exec(`UPDATE people SET photo = ? WHERE id = ?`, filename, id); err != nil {
		return "", err
	}
	return previous, nil
}

// DeletePerson removes the row and returns its photo filename so the caller
// can also remove the file from disk.
func DeletePerson(conn *sql.DB, id int64) (string, error) {
	var photo string
	if err := conn.QueryRow(`SELECT photo FROM people WHERE id = ?`, id).Scan(&photo); err != nil {
		return "", err
	}
	if _, err := conn.Exec(`DELETE FROM people WHERE id = ?`, id); err != nil {
		return "", err
	}
	return photo, nil
}
