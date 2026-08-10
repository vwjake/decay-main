package db

import (
	"database/sql"
	"strings"
	"time"
)

// BookingNew is the only status a booking request carries — there's no
// reviewed/archived workflow, just a queue worked through with notes and
// deletion. See the comment on booking_requests in schema.sql.
const BookingNew = "new"

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
	// Notes are the admin's own, private context on a request — never shown
	// to the requester.
	Notes     string
	CreatedAt time.Time
}

func (b BookingRequest) IsNew() bool { return b.Status == BookingNew }

// preferredDateLayouts are the formats PreferredDate might have been typed
// in as — it's a free-text field on the public form, not a date picker.
var preferredDateLayouts = []string{
	"2006-01-02",
	"01/02/2006",
	"1/2/2006",
	"January 2, 2006",
	"Jan 2, 2006",
	"January 2 2006",
	"Jan 2 2006",
}

// ParsePreferredDate best-effort parses PreferredDate into a real date, for
// places (like the admin calendar) that want to plot a request against a
// month grid. Most submissions won't parse — that's expected, not an
// error — so it just reports whether it found one.
func ParsePreferredDate(raw string, loc *time.Location) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range preferredDateLayouts {
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// When renders the submission time for the queue.
func (b BookingRequest) When() string { return b.CreatedAt.Format("Jan 2, 2006 · 3:04 PM") }

const bookingColumns = `id, name, email, phone, event_name, description, preferred_date, expected_attendance, status, notes, created_at`

func scanBookings(rows *sql.Rows) ([]BookingRequest, error) {
	var out []BookingRequest
	for rows.Next() {
		var b BookingRequest
		var created string
		if err := rows.Scan(&b.ID, &b.Name, &b.Email, &b.Phone, &b.EventName, &b.Description,
			&b.PreferredDate, &b.ExpectedAttendance, &b.Status, &b.Notes, &created); err != nil {
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

// ListBookingRequests returns every request for the admin queue, newest
// first.
func ListBookingRequests(conn *sql.DB) ([]BookingRequest, error) {
	rows, err := conn.Query(`SELECT ` + bookingColumns + ` FROM booking_requests ORDER BY created_at DESC`)
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
		&b.PreferredDate, &b.ExpectedAttendance, &b.Status, &b.Notes, &created); err != nil {
		return BookingRequest{}, err
	}
	t, err := time.Parse("2006-01-02 15:04:05", created)
	if err != nil {
		return BookingRequest{}, err
	}
	b.CreatedAt = t
	return b, nil
}

// SetBookingNotes saves the admin's own context on a request — never shown
// to the requester, just a place to jot what's been discussed or decided.
func SetBookingNotes(conn *sql.DB, id int64, notes string) error {
	_, err := conn.Exec(`UPDATE booking_requests SET notes = ? WHERE id = ?`, notes, id)
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
