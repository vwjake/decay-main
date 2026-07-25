package db

import (
	"database/sql"
	"fmt"
	"time"
)

// dateLayout is how a donation's received-on date is stored and how the
// reports screen's date-range inputs (<input type="date">) submit.
const dateLayout = "2006-01-02"

// Quarter is a calendar quarter of a given year, the natural reporting
// period for a nonprofit that files quarterly.
type Quarter struct {
	Year int
	Q    int // 1–4
}

// QuarterOf returns the quarter a moment falls in, read in loc so an event
// just before midnight lands in the local quarter, not the UTC one.
func QuarterOf(t time.Time, loc *time.Location) Quarter {
	t = t.In(loc)
	return Quarter{Year: t.Year(), Q: int(t.Month()-1)/3 + 1}
}

// Range returns the half-open span [from, to) the quarter covers, as
// midnights in loc.
func (q Quarter) Range(loc *time.Location) (from, to time.Time) {
	startMonth := time.Month((q.Q-1)*3 + 1)
	from = time.Date(q.Year, startMonth, 1, 0, 0, 0, 0, loc)
	to = from.AddDate(0, 3, 0)
	return from, to
}

func (q Quarter) Label() string  { return fmt.Sprintf("Q%d %d", q.Q, q.Year) }
func (q Quarter) String() string { return fmt.Sprintf("%d-Q%d", q.Year, q.Q) }

// Prev and Next step to the neighbouring quarter, rolling the year over.
func (q Quarter) Prev() Quarter {
	if q.Q == 1 {
		return Quarter{Year: q.Year - 1, Q: 4}
	}
	return Quarter{Year: q.Year, Q: q.Q - 1}
}

func (q Quarter) Next() Quarter {
	if q.Q == 4 {
		return Quarter{Year: q.Year + 1, Q: 1}
	}
	return Quarter{Year: q.Year, Q: q.Q + 1}
}

// ParseQuarter reads a "2026-Q3" token. It returns ok=false rather than an
// error so a stale or hand-edited query param falls back to a default
// instead of erroring the page.
func ParseQuarter(s string) (Quarter, bool) {
	var q Quarter
	if _, err := fmt.Sscanf(s, "%d-Q%d", &q.Year, &q.Q); err != nil {
		return Quarter{}, false
	}
	if q.Q < 1 || q.Q > 4 || q.Year < 2000 || q.Year > 9999 {
		return Quarter{}, false
	}
	return q, true
}

// EventReport holds the post-event numbers. Attendance and DoorCents are
// pointers so an unrecorded number reads as nil rather than a misleading
// zero; Recorded reports whether a row exists at all.
type EventReport struct {
	EventID    int64
	Attendance *int64
	DoorCents  *int64
	Notes      string
	Recorded   bool
}

// HasAttendance and HasDoor report whether each number was entered.
func (r EventReport) HasAttendance() bool { return r.Attendance != nil }
func (r EventReport) HasDoor() bool       { return r.DoorCents != nil }

// AttendanceValue and DoorValue give the number for form fields, treating
// "not recorded" as an empty edit rather than a zero.
func (r EventReport) AttendanceStr() string {
	if r.Attendance == nil {
		return ""
	}
	return fmt.Sprintf("%d", *r.Attendance)
}

func (r EventReport) DoorStr() string {
	if r.DoorCents == nil {
		return ""
	}
	return dollarsPlain(*r.DoorCents)
}

// DoorDisplay renders the door take for reading, or a dash when unrecorded.
func (r EventReport) DoorDisplay() string {
	if r.DoorCents == nil {
		return "—"
	}
	return Dollars(*r.DoorCents)
}

// dollarsPlain is Dollars without the "$" or thousands commas, so it round
// trips back through ParseDollars in a form field.
func dollarsPlain(cents int64) string {
	if cents%100 == 0 {
		return fmt.Sprintf("%d", cents/100)
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// EventReportFor loads one event's report, or a zero-value (Recorded false)
// report when none has been entered yet.
func EventReportFor(conn *sql.DB, eventID int64) (EventReport, error) {
	r := EventReport{EventID: eventID}
	var attendance, doorCents sql.NullInt64
	err := conn.QueryRow(
		`SELECT attendance, door_cents, notes FROM event_reports WHERE event_id = ?`, eventID,
	).Scan(&attendance, &doorCents, &r.Notes)
	if err == sql.ErrNoRows {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	r.Recorded = true
	if attendance.Valid {
		r.Attendance = &attendance.Int64
	}
	if doorCents.Valid {
		r.DoorCents = &doorCents.Int64
	}
	return r, nil
}

// EventReportsFor loads the reports for a set of events in one query,
// keyed by event id. Events without a row are simply absent from the map.
func EventReportsFor(conn *sql.DB, ids []int64) (map[int64]EventReport, error) {
	out := map[int64]EventReport{}
	if len(ids) == 0 {
		return out, nil
	}
	query, args := inClause(`SELECT event_id, attendance, door_cents, notes FROM event_reports WHERE event_id IN (`, ids)
	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		r := EventReport{Recorded: true}
		var attendance, door sql.NullInt64
		if err := rows.Scan(&r.EventID, &attendance, &door, &r.Notes); err != nil {
			return nil, err
		}
		if attendance.Valid {
			r.Attendance = &attendance.Int64
		}
		if door.Valid {
			r.DoorCents = &door.Int64
		}
		out[r.EventID] = r
	}
	return out, rows.Err()
}

// DonationTotalsFor sums donations tied to each of the given events, keyed
// by event id. Events with no donations are absent from the map.
func DonationTotalsFor(conn *sql.DB, ids []int64) (map[int64]int64, error) {
	out := map[int64]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	query, args := inClause(`SELECT event_id, SUM(amount_cents) FROM donations WHERE event_id IN (`, ids)
	rows, err := conn.Query(query+` GROUP BY event_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, total int64
		if err := rows.Scan(&id, &total); err != nil {
			return nil, err
		}
		out[id] = total
	}
	return out, rows.Err()
}

// inClause finishes a "... IN (" prefix with the right number of
// placeholders and returns the args to pass alongside it.
func inClause(prefix string, ids []int64) (string, []any) {
	args := make([]any, len(ids))
	var b []byte
	b = append(b, prefix...)
	for i, id := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
		args[i] = id
	}
	b = append(b, ')')
	return string(b), args
}

// SaveEventReport upserts an event's numbers. Nil attendance or door means
// "leave blank", stored as SQL NULL so it stays distinct from a real zero.
func SaveEventReport(conn *sql.DB, eventID int64, attendance, doorCents *int64, notes string) error {
	_, err := conn.Exec(
		`INSERT INTO event_reports (event_id, attendance, door_cents, notes, updated_at)
		 VALUES (?, ?, ?, ?, datetime('now'))
		 ON CONFLICT (event_id) DO UPDATE SET
		     attendance = excluded.attendance,
		     door_cents = excluded.door_cents,
		     notes = excluded.notes,
		     updated_at = excluded.updated_at`,
		eventID, nullInt(attendance), nullInt(doorCents), notes,
	)
	return err
}

func nullInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// Donation is one gift of money, optionally tied to the event it came in
// at. A nil EventID is a standalone donation (online, a mailed cheque).
type Donation struct {
	ID          int64
	EventID     *int64
	EventTitle  string // filled by ListDonations for display
	AmountCents int64
	Source      string
	Note        string
	ReceivedAt  time.Time
}

func (d Donation) Amount() string { return Dollars(d.AmountCents) }
func (d Donation) Date() string   { return d.ReceivedAt.Format("Jan 2, 2006") }

// AddDonation records a donation. receivedAt is a "2006-01-02" date;
// an empty eventID makes it standalone.
func AddDonation(conn *sql.DB, eventID *int64, amountCents int64, source, note string, receivedAt time.Time) error {
	_, err := conn.Exec(
		`INSERT INTO donations (event_id, amount_cents, source, note, received_at)
		 VALUES (?, ?, ?, ?, ?)`,
		nullInt(eventID), amountCents, source, note, receivedAt.Format(dateLayout),
	)
	return err
}

// DeleteDonation removes a donation row.
func DeleteDonation(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM donations WHERE id = ?`, id)
	return err
}

// ListDonations returns the donations received within [from, to), newest
// first, with the tied event's title when there is one.
func ListDonations(conn *sql.DB, from, to time.Time) ([]Donation, error) {
	rows, err := conn.Query(
		`SELECT d.id, d.event_id, d.amount_cents, d.source, d.note, d.received_at,
		        COALESCE(e.title, '')
		 FROM donations d
		 LEFT JOIN events e ON e.id = d.event_id
		 WHERE d.received_at >= ? AND d.received_at < ?
		 ORDER BY d.received_at DESC, d.id DESC`,
		from.Format(dateLayout), to.Format(dateLayout),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Donation
	for rows.Next() {
		var d Donation
		var eventID sql.NullInt64
		var received string
		if err := rows.Scan(&d.ID, &eventID, &d.AmountCents, &d.Source, &d.Note, &received, &d.EventTitle); err != nil {
			return nil, err
		}
		if eventID.Valid {
			d.EventID = &eventID.Int64
		}
		t, err := time.Parse(dateLayout, received)
		if err != nil {
			return nil, err
		}
		d.ReceivedAt = t
		out = append(out, d)
	}
	return out, rows.Err()
}

// TypeStat is the per-event-type slice of a report.
type TypeStat struct {
	Type       string
	Events     int
	Attendance int
}

// Stats is a whole report over a date range: how much happened, how many
// came, and how much money came in. Door take is attributed by the event's
// date; donations are attributed by the date they were received, so both a
// show's jar and an online gift land in the quarter they belong to.
type Stats struct {
	From, To             time.Time
	Events               int
	AttendanceEvents     int // events with an attendance figure recorded
	Attendance           int
	ReportedEvents       int // events with any numbers recorded
	DoorCents            int64
	DonationCents        int64
	DonationCount        int
	VolunteerRolesFilled int
	ByType               []TypeStat
}

func (s Stats) DoorMoney() string     { return Dollars(s.DoorCents) }
func (s Stats) DonationMoney() string { return Dollars(s.DonationCents) }
func (s Stats) TotalMoney() string    { return Dollars(s.DoorCents + s.DonationCents) }

// AvgAttendance is the mean over the events that actually have a figure, so
// unrecorded events don't drag the average toward zero.
func (s Stats) AvgAttendance() string {
	if s.AttendanceEvents == 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f", float64(s.Attendance)/float64(s.AttendanceEvents))
}

// RangeStats builds a report over [from, to). Events are filtered in Go
// against real instants, matching how the rest of the calendar avoids
// SQLite's string-time comparisons across daylight saving.
func RangeStats(conn *sql.DB, from, to time.Time) (Stats, error) {
	s := Stats{From: from, To: to}

	all, err := ListAllEvents(conn)
	if err != nil {
		return s, err
	}
	inRange := map[int64]string{} // event id -> type
	byType := map[string]*TypeStat{}
	var typeOrder []string
	for _, e := range all {
		if e.StartsAt.Before(from) || !e.StartsAt.Before(to) {
			continue
		}
		s.Events++
		inRange[e.ID] = e.EventType
		if _, ok := byType[e.EventType]; !ok {
			byType[e.EventType] = &TypeStat{Type: e.EventType}
			typeOrder = append(typeOrder, e.EventType)
		}
		byType[e.EventType].Events++
	}

	// Per-event numbers: attendance and door take, matched to the events
	// that fall in range.
	reportRows, err := conn.Query(`SELECT event_id, attendance, door_cents FROM event_reports`)
	if err != nil {
		return s, err
	}
	defer reportRows.Close()
	for reportRows.Next() {
		var id int64
		var attendance, door sql.NullInt64
		if err := reportRows.Scan(&id, &attendance, &door); err != nil {
			return s, err
		}
		if _, ok := inRange[id]; !ok {
			continue
		}
		s.ReportedEvents++
		if attendance.Valid {
			s.AttendanceEvents++
			s.Attendance += int(attendance.Int64)
			byType[inRange[id]].Attendance += int(attendance.Int64)
		}
		if door.Valid {
			s.DoorCents += door.Int64
		}
	}
	if err := reportRows.Err(); err != nil {
		return s, err
	}

	// Filled volunteer roles for in-range events.
	volRows, err := conn.Query(`SELECT event_id FROM event_volunteers WHERE volunteer_name <> ''`)
	if err != nil {
		return s, err
	}
	defer volRows.Close()
	for volRows.Next() {
		var id int64
		if err := volRows.Scan(&id); err != nil {
			return s, err
		}
		if _, ok := inRange[id]; ok {
			s.VolunteerRolesFilled++
		}
	}
	if err := volRows.Err(); err != nil {
		return s, err
	}

	// Donations, ranged by the date received so standalone gifts count too.
	if err := conn.QueryRow(
		`SELECT COALESCE(SUM(amount_cents), 0), COUNT(*) FROM donations
		 WHERE received_at >= ? AND received_at < ?`,
		from.Format(dateLayout), to.Format(dateLayout),
	).Scan(&s.DonationCents, &s.DonationCount); err != nil {
		return s, err
	}

	for _, t := range typeOrder {
		s.ByType = append(s.ByType, *byType[t])
	}
	return s, nil
}
