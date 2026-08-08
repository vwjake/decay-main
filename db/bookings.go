package db

import (
	"database/sql"
	"time"
)

// Booking request statuses.
const (
	BookingNew      = "new"
	BookingReviewed = "reviewed"
	BookingArchived = "archived"
)

// BookingRequest is a public request to use the space for an event.
type BookingRequest struct {
	ID                 int64
	Name               string
	Email              string
	Phone              string
	EventName          string
	Description        string
	PreferredDate      string
	ExpectedAttendance string
	Status             string
	CreatedAt          time.Time
}

func (b BookingRequest) IsNew() bool      { return b.Status == BookingNew }
func (b BookingRequest) IsArchived() bool { return b.Status == BookingArchived }

// When renders the submission time for the queue.
func (b BookingRequest) When() string { return b.CreatedAt.Format("Jan 2, 2006 · 3:04 PM") }

const bookingColumns = `id, name, email, phone, event_name, description, preferred_date, expected_attendance, status, created_at`

func scanBookings(rows *sql.Rows) ([]BookingRequest, error) {
	var out []BookingRequest
	for rows.Next() {
		var b BookingRequest
		var created string
		if err := rows.Scan(&b.ID, &b.Name, &b.Email, &b.Phone, &b.EventName, &b.Description,
			&b.PreferredDate, &b.ExpectedAttendance, &b.Status, &created); err != nil {
			return nil, err
		}
		t, err := time.Parse("2006-01-02 15:04:05", created)
		if err != nil {
			return nil, err
		}
		b.CreatedAt = t
		out = append(out, b)
	}
	return out, rows.Err()
}

// CreateBookingRequest records a submission from the public form.
func CreateBookingRequest(conn *sql.DB, b BookingRequest) error {
	_, err := conn.Exec(
		`INSERT INTO booking_requests (name, email, phone, event_name, description, preferred_date, expected_attendance)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.Name, b.Email, b.Phone, b.EventName, b.Description, b.PreferredDate, b.ExpectedAttendance,
	)
	return err
}

// ListBookingRequests returns requests for the admin queue, newest first.
// Archived ones are included only when includeArchived is set.
func ListBookingRequests(conn *sql.DB, includeArchived bool) ([]BookingRequest, error) {
	query := `SELECT ` + bookingColumns + ` FROM booking_requests`
	if !includeArchived {
		query += ` WHERE status <> '` + BookingArchived + `'`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBookings(rows)
}

// BookingByID looks up a single request, for the detail page.
func BookingByID(conn *sql.DB, id int64) (BookingRequest, error) {
	row := conn.QueryRow(`SELECT `+bookingColumns+` FROM booking_requests WHERE id = ?`, id)
	var b BookingRequest
	var created string
	if err := row.Scan(&b.ID, &b.Name, &b.Email, &b.Phone, &b.EventName, &b.Description,
		&b.PreferredDate, &b.ExpectedAttendance, &b.Status, &created); err != nil {
		return BookingRequest{}, err
	}
	t, err := time.Parse("2006-01-02 15:04:05", created)
	if err != nil {
		return BookingRequest{}, err
	}
	b.CreatedAt = t
	return b, nil
}

// SetBookingStatus moves a request between new, reviewed, and archived.
func SetBookingStatus(conn *sql.DB, id int64, status string) error {
	_, err := conn.Exec(`UPDATE booking_requests SET status = ? WHERE id = ?`, status, id)
	return err
}

// DeleteBookingRequest removes a request for good.
func DeleteBookingRequest(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM booking_requests WHERE id = ?`, id)
	return err
}

// CountNewBookings is how many requests still need a first look, for the
// dashboard's needs-attention list.
func CountNewBookings(conn *sql.DB) (int, error) {
	var n int
	err := conn.QueryRow(`SELECT count(*) FROM booking_requests WHERE status = ?`, BookingNew).Scan(&n)
	return n, err
}
