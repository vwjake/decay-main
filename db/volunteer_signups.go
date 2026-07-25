package db

import (
	"database/sql"
	"time"
)

// VolunteerSignup is a public offer to help with an event, carrying the
// contact details organizers need to follow up. It's separate from
// EventVolunteer, which is the admin's record of who is actually covering
// a role.
type VolunteerSignup struct {
	ID        int64
	EventID   int64
	Role      string // empty means "wherever needed"
	Name      string
	Contact   string
	Note      string
	CreatedAt time.Time
}

// RoleLabel renders the signed-up role, or a general label when the
// volunteer didn't pick one.
func (s VolunteerSignup) RoleLabel() string {
	if s.Role == "" {
		return "Wherever needed"
	}
	return RoleLabel(s.Role)
}

func (s VolunteerSignup) When() string { return s.CreatedAt.Format("Jan 2, 2006 · 3:04 PM") }

// CreateVolunteerSignup records a submission from the public form.
func CreateVolunteerSignup(conn *sql.DB, s VolunteerSignup) error {
	_, err := conn.Exec(
		`INSERT INTO volunteer_signups (event_id, role, name, contact, note) VALUES (?, ?, ?, ?, ?)`,
		s.EventID, s.Role, s.Name, s.Contact, s.Note,
	)
	return err
}

// SignupsForEvent returns the offers received for an event, newest first,
// for the admin event page.
func SignupsForEvent(conn *sql.DB, eventID int64) ([]VolunteerSignup, error) {
	rows, err := conn.Query(
		`SELECT id, event_id, role, name, contact, note, created_at
		 FROM volunteer_signups WHERE event_id = ? ORDER BY created_at DESC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VolunteerSignup
	for rows.Next() {
		var s VolunteerSignup
		var created string
		if err := rows.Scan(&s.ID, &s.EventID, &s.Role, &s.Name, &s.Contact, &s.Note, &created); err != nil {
			return nil, err
		}
		t, err := time.Parse("2006-01-02 15:04:05", created)
		if err != nil {
			return nil, err
		}
		s.CreatedAt = t
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteVolunteerSignup removes a signup once it's been dealt with.
func DeleteVolunteerSignup(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM volunteer_signups WHERE id = ?`, id)
	return err
}

// CountSignupsForUpcoming counts offers attached to events that haven't
// happened yet, for the dashboard's needs-attention list.
func CountSignupsForUpcoming(conn *sql.DB) (int, error) {
	var n int
	err := conn.QueryRow(
		`SELECT count(*) FROM volunteer_signups v
		 JOIN events e ON e.id = v.event_id
		 WHERE e.starts_at > ?`,
		time.Now().Format(timeLayout),
	).Scan(&n)
	return n, err
}
