package db

import (
	"database/sql"
	"net/url"
	"strings"
)

// ExternalForm is a link to an off-site form — a Nextcloud Forms survey and
// the like — listed on the public Get Involved page. The site never hosts the
// form; it only points at it.
type ExternalForm struct {
	ID          int64
	Title       string
	Description string
	URL         string
	Position    int
	Enabled     bool
}

// NormalizeFormURL trims the input and confirms it's an absolute http(s) URL,
// so the admin form can reject a typo before it reaches the public page.
func NormalizeFormURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	return raw, true
}

const externalFormColumns = `id, title, description, url, position, enabled`

func scanExternalForms(rows *sql.Rows) ([]ExternalForm, error) {
	var out []ExternalForm
	for rows.Next() {
		var f ExternalForm
		if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.URL, &f.Position, &f.Enabled); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListExternalForms returns every form for the admin screen, in display order.
func ListExternalForms(conn *sql.DB) ([]ExternalForm, error) {
	rows, err := conn.Query(`SELECT ` + externalFormColumns + ` FROM external_forms ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExternalForms(rows)
}

// EnabledExternalForms returns just the forms shown publicly, in order.
func EnabledExternalForms(conn *sql.DB) ([]ExternalForm, error) {
	rows, err := conn.Query(`SELECT ` + externalFormColumns + ` FROM external_forms WHERE enabled = 1 ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExternalForms(rows)
}

// ExternalFormByID fetches one form for editing.
func ExternalFormByID(conn *sql.DB, id int64) (ExternalForm, error) {
	rows, err := conn.Query(`SELECT `+externalFormColumns+` FROM external_forms WHERE id = ?`, id)
	if err != nil {
		return ExternalForm{}, err
	}
	defer rows.Close()
	forms, err := scanExternalForms(rows)
	if err != nil {
		return ExternalForm{}, err
	}
	if len(forms) == 0 {
		return ExternalForm{}, sql.ErrNoRows
	}
	return forms[0], nil
}

// CreateExternalForm adds a form link and returns its new id.
func CreateExternalForm(conn *sql.DB, f ExternalForm) (int64, error) {
	res, err := conn.Exec(
		`INSERT INTO external_forms (title, description, url, position, enabled) VALUES (?, ?, ?, ?, ?)`,
		f.Title, f.Description, f.URL, f.Position, f.Enabled,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateExternalForm saves edits to a form link.
func UpdateExternalForm(conn *sql.DB, f ExternalForm) error {
	_, err := conn.Exec(
		`UPDATE external_forms SET title = ?, description = ?, url = ?, position = ?, enabled = ? WHERE id = ?`,
		f.Title, f.Description, f.URL, f.Position, f.Enabled, f.ID,
	)
	return err
}

// DeleteExternalForm removes a form link.
func DeleteExternalForm(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM external_forms WHERE id = ?`, id)
	return err
}
