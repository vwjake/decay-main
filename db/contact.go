package db

import (
	"database/sql"
	"strings"
	"time"
)

// Contact message statuses. Same lifecycle as booking requests.
const (
	ContactNew      = "new"
	ContactReviewed = "reviewed"
	ContactArchived = "archived"
)

// ContactMessage is a submission from the public contact form.
type ContactMessage struct {
	ID        int64
	Name      string
	Email     string
	Subject   string
	Message   string
	Status    string
	CreatedAt time.Time
}

func (m ContactMessage) IsNew() bool      { return m.Status == ContactNew }
func (m ContactMessage) IsArchived() bool { return m.Status == ContactArchived }

// When renders the submission time for the queue.
func (m ContactMessage) When() string { return m.CreatedAt.Format("Jan 2, 2006 · 3:04 PM") }

// SubjectOr returns the subject, or a stand-in when the sender left it blank,
// so the queue and any notification email always have a heading.
func (m ContactMessage) SubjectOr() string {
	if strings.TrimSpace(m.Subject) == "" {
		return "General inquiry"
	}
	return m.Subject
}

const contactColumns = `id, name, email, subject, message, status, created_at`

func scanContactMessages(rows *sql.Rows) ([]ContactMessage, error) {
	var out []ContactMessage
	for rows.Next() {
		var m ContactMessage
		var created string
		if err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Subject, &m.Message, &m.Status, &created); err != nil {
			return nil, err
		}
		t, err := time.Parse("2006-01-02 15:04:05", created)
		if err != nil {
			return nil, err
		}
		m.CreatedAt = t
		out = append(out, m)
	}
	return out, rows.Err()
}

// CreateContactMessage records a submission from the public form.
func CreateContactMessage(conn *sql.DB, m ContactMessage) error {
	_, err := conn.Exec(
		`INSERT INTO contact_messages (name, email, subject, message) VALUES (?, ?, ?, ?)`,
		m.Name, m.Email, m.Subject, m.Message,
	)
	return err
}

// ListContactMessages returns messages for the admin queue, newest first.
// Archived ones are included only when includeArchived is set.
func ListContactMessages(conn *sql.DB, includeArchived bool) ([]ContactMessage, error) {
	query := `SELECT ` + contactColumns + ` FROM contact_messages`
	if !includeArchived {
		query += ` WHERE status <> '` + ContactArchived + `'`
	}
	// id breaks ties so messages saved in the same second (created_at is
	// second-resolution) still come back newest-first deterministically.
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanContactMessages(rows)
}

// SetContactStatus moves a message between new, reviewed, and archived.
func SetContactStatus(conn *sql.DB, id int64, status string) error {
	_, err := conn.Exec(`UPDATE contact_messages SET status = ? WHERE id = ?`, status, id)
	return err
}

// DeleteContactMessage removes a message for good.
func DeleteContactMessage(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM contact_messages WHERE id = ?`, id)
	return err
}

// CountNewMessages is how many messages still need a first look, for the
// dashboard's needs-attention list.
func CountNewMessages(conn *sql.DB) (int, error) {
	var n int
	err := conn.QueryRow(`SELECT count(*) FROM contact_messages WHERE status = ?`, ContactNew).Scan(&n)
	return n, err
}
