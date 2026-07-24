package db

import (
	"database/sql"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
)

// eventColumns is the select list every event query shares, so scanEvents
// stays in step with it.
const eventColumns = `id, title, event_type, starts_at, ends_at, location, description, link, uid, flyer, slug`

const timeLayout = "2006-01-02T15:04:05-07:00"

type Event struct {
	ID          int64
	Title       string
	EventType   string
	StartsAt    time.Time
	EndsAt      *time.Time
	Location    string
	Description string
	Link        string
	// UID is the event's iCalendar identity, carried over from the old
	// site where it exists so calendars already subscribed to it don't
	// see a second copy.
	UID string
	// Flyer is a filename under uploads/flyers/, empty if there isn't one.
	Flyer string
	Slug  string
}

// Slug builds an event's URL segment from its date and title, e.g.
// "2026-07-25-free-mask-distro". The date leads so that a recurring title
// like "Open Draw" stays unique and the URL says when it happened.
func Slug(startsAt time.Time, title string) string {
	var b strings.Builder
	b.WriteString(startsAt.Format("2006-01-02"))
	b.WriteByte('-')

	lastDash := true
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// VolunteerRoles are the jobs an event can need covered, in the order
// they're shown. The old site used exactly these four.
var VolunteerRoles = []string{"door", "sound", "cleanup", "promote"}

// RoleLabel renders a role slug for display.
func RoleLabel(role string) string {
	switch role {
	case "door":
		return "Door"
	case "sound":
		return "Sound"
	case "cleanup":
		return "Cleanup"
	case "promote":
		return "Promote"
	}
	return role
}

// EventVolunteer is one role an event needs covered. Name is empty while
// the slot is still open.
type EventVolunteer struct {
	ID      int64
	EventID int64
	Role    string
	Name    string
}

func (v EventVolunteer) Label() string { return RoleLabel(v.Role) }
func (v EventVolunteer) Filled() bool  { return v.Name != "" }

func (e Event) HasFlyer() bool { return e.Flyer != "" }

// FlyerPath is the URL of the original, full-resolution flyer.
func (e Event) FlyerPath() string { return "/uploads/flyers/" + e.Flyer }

// FlyerWebPath is the URL of the web-sized copy, which is what pages
// should display — originals run to several megabytes.
func (e Event) FlyerWebPath() string {
	return "/uploads/flyers/web/" + strings.TrimSuffix(e.Flyer, path.Ext(e.Flyer)) + ".jpg"
}

// Paragraphs splits a description into blocks on blank lines so a template
// can render it without trusting the source with raw HTML.
func (e Event) Paragraphs() []string {
	var out []string
	for _, block := range strings.Split(strings.ReplaceAll(e.Description, "\r\n", "\n"), "\n\n") {
		if block = strings.TrimSpace(block); block != "" {
			out = append(out, block)
		}
	}
	return out
}

// LongDate renders the full date for a detail page, e.g.
// "Saturday, July 25, 2026".
func (e Event) LongDate() string { return e.StartsAt.Format("Monday, January 2, 2006") }

// Path is the event's public URL. Events imported before slugs existed
// fall back to their id so nothing 404s.
func (e Event) Path() string {
	if e.Slug != "" {
		return "/events/" + e.Slug
	}
	return "/events/" + strconv.FormatInt(e.ID, 10)
}

// HasLink reports whether the event points somewhere worth clicking.
// Events carry "#" when the old site had no link for them.
func (e Event) HasLink() bool { return e.Link != "" && e.Link != "#" }

func (e Event) Day() string   { return e.StartsAt.Format("02") }
func (e Event) Month() string { return strings.ToUpper(e.StartsAt.Format("Jan")) }

func (e Event) TimeRange() string {
	start := formatClock(e.StartsAt)
	if e.EndsAt != nil {
		return start + " – " + formatClock(*e.EndsAt)
	}
	return start
}

func formatClock(t time.Time) string {
	if t.Minute() == 0 {
		return t.Format("3 PM")
	}
	return t.Format("3:04 PM")
}

type Product struct {
	ID          int64
	Name        string
	PriceCents  int
	Placeholder string
	StripeURL   string
	Image       string
	Variants    string
}

// HasImage reports whether there's a photo to show instead of the
// placeholder text.
func (p Product) HasImage() bool { return p.Image != "" }

// ImagePath is the web-sized copy, which is what pages should display.
func (p Product) ImagePath() string {
	return "/uploads/products/web/" + strings.TrimSuffix(p.Image, path.Ext(p.Image)) + ".jpg"
}

func (p Product) Price() string {
	if p.PriceCents%100 == 0 {
		return fmt.Sprintf("$%d", p.PriceCents/100)
	}
	return fmt.Sprintf("$%.2f", float64(p.PriceCents)/100)
}

type Post struct {
	ID          int64
	Slug        string
	Title       string
	Body        string
	PublishedAt *time.Time
	CreatedAt   time.Time
}

func (p Post) Published() bool { return p.PublishedAt != nil }

// Date renders a post's publication date for display.
func (p Post) Date() string {
	if p.PublishedAt == nil {
		return ""
	}
	return p.PublishedAt.Format("January 2, 2006")
}

// Paragraphs splits a post body into blocks on blank lines, so a template
// can render it without handing raw HTML to the browser.
func (p Post) Paragraphs() []string {
	var out []string
	for _, block := range strings.Split(strings.ReplaceAll(p.Body, "\r\n", "\n"), "\n\n") {
		if block = strings.TrimSpace(block); block != "" {
			out = append(out, block)
		}
	}
	return out
}

// PostBySlug fetches a single published post. Drafts stay unreachable
// from the public site, so an unpublished slug is reported as missing.
func PostBySlug(conn *sql.DB, slug string) (Post, error) {
	rows, err := conn.Query(
		`SELECT id, slug, title, body_markdown, published_at, created_at FROM posts WHERE slug = ? AND published_at IS NOT NULL`,
		slug,
	)
	if err != nil {
		return Post{}, err
	}
	defer rows.Close()

	posts, err := scanPosts(rows)
	if err != nil {
		return Post{}, err
	}
	if len(posts) == 0 {
		return Post{}, sql.ErrNoRows
	}
	return posts[0], nil
}

type Photo struct {
	ID       int64
	Filename string
	Caption  string
}

func scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var ev Event
		var startsAt string
		var endsAt sql.NullString
		if err := rows.Scan(&ev.ID, &ev.Title, &ev.EventType, &startsAt, &endsAt, &ev.Location, &ev.Description, &ev.Link, &ev.UID, &ev.Flyer, &ev.Slug); err != nil {
			return nil, err
		}
		var err error
		ev.StartsAt, err = time.Parse(timeLayout, startsAt)
		if err != nil {
			return nil, err
		}
		if endsAt.Valid {
			end, err := time.Parse(timeLayout, endsAt.String)
			if err != nil {
				return nil, err
			}
			ev.EndsAt = &end
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// ListUpcomingEvents returns up to limit events starting now or later,
// soonest first.
func ListUpcomingEvents(conn *sql.DB, limit int) ([]Event, error) {
	events, err := UpcomingEvents(conn)
	if err != nil {
		return nil, err
	}
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

// UpcomingEvents returns every event starting now or later, soonest
// first. Filtering happens in Go rather than SQL so the comparison is
// against real time.Time instants, not SQLite's UTC clock.
func UpcomingEvents(conn *sql.DB) ([]Event, error) {
	all, err := ListAllEvents(conn)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var events []Event
	// ListAllEvents is newest first; walking it backwards yields soonest first.
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].StartsAt.Before(now) {
			continue
		}
		events = append(events, all[i])
	}
	return events, nil
}

// PastEvents returns every event that has already started, most recent
// first — the public archive of what has happened at the space.
func PastEvents(conn *sql.DB) ([]Event, error) {
	all, err := ListAllEvents(conn)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var events []Event
	for _, ev := range all {
		if ev.StartsAt.Before(now) {
			events = append(events, ev)
		}
	}
	return events, nil
}

// EventBySlug looks up a single event for its detail page. It also
// accepts a bare numeric id, so events predating slugs stay reachable.
// Returns sql.ErrNoRows when nothing matches.
func EventBySlug(conn *sql.DB, slug string) (Event, error) {
	rows, err := conn.Query(`SELECT `+eventColumns+` FROM events WHERE slug = ? OR (slug = '' AND id = ?) LIMIT 1`, slug, slug)
	if err != nil {
		return Event{}, err
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return Event{}, err
	}
	if len(events) == 0 {
		return Event{}, sql.ErrNoRows
	}
	return events[0], nil
}

// EventByID looks up one event for the admin panel.
func EventByID(conn *sql.DB, id int64) (Event, error) {
	rows, err := conn.Query(`SELECT `+eventColumns+` FROM events WHERE id = ?`, id)
	if err != nil {
		return Event{}, err
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return Event{}, err
	}
	if len(events) == 0 {
		return Event{}, sql.ErrNoRows
	}
	return events[0], nil
}

// ProductByID fetches one shop item for editing.
func ProductByID(conn *sql.DB, id int64) (Product, error) {
	var p Product
	err := conn.QueryRow(
		`SELECT `+productColumns+` FROM products WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.PriceCents, &p.Placeholder, &p.StripeURL, &p.Image, &p.Variants)
	return p, err
}

// PostByID fetches one post for editing, drafts included.
func PostByID(conn *sql.DB, id int64) (Post, error) {
	rows, err := conn.Query(
		`SELECT id, slug, title, body_markdown, published_at, created_at FROM posts WHERE id = ?`, id,
	)
	if err != nil {
		return Post{}, err
	}
	defer rows.Close()

	posts, err := scanPosts(rows)
	if err != nil {
		return Post{}, err
	}
	if len(posts) == 0 {
		return Post{}, sql.ErrNoRows
	}
	return posts[0], nil
}

// Counts is the at-a-glance summary the admin dashboard opens with.
type Counts struct {
	UpcomingEvents int
	PastEvents     int
	OpenRoles      int
	MissingFlyers  int
	Drafts         int
	PublishedPosts int
	Products       int
	Photos         int
	NextEvent      *Event
}

// Summary gathers the dashboard counts in one pass.
func Summary(conn *sql.DB) (Counts, error) {
	var c Counts

	upcoming, err := UpcomingEvents(conn)
	if err != nil {
		return c, err
	}
	c.UpcomingEvents = len(upcoming)
	if len(upcoming) > 0 {
		c.NextEvent = &upcoming[0]
	}
	// Only upcoming events are worth chasing a flyer for.
	for _, ev := range upcoming {
		if !ev.HasFlyer() {
			c.MissingFlyers++
		}
	}

	all, err := ListAllEvents(conn)
	if err != nil {
		return c, err
	}
	c.PastEvents = len(all) - c.UpcomingEvents

	// Open roles only count for events that haven't happened yet.
	if err := conn.QueryRow(`
		SELECT count(*) FROM event_volunteers v
		JOIN events e ON e.id = v.event_id
		WHERE v.volunteer_name = '' AND e.starts_at > ?`,
		time.Now().Format(timeLayout),
	).Scan(&c.OpenRoles); err != nil {
		return c, err
	}

	for _, q := range []struct {
		dest  *int
		query string
	}{
		{&c.Drafts, `SELECT count(*) FROM posts WHERE published_at IS NULL`},
		{&c.PublishedPosts, `SELECT count(*) FROM posts WHERE published_at IS NOT NULL`},
		{&c.Products, `SELECT count(*) FROM products`},
		{&c.Photos, `SELECT count(*) FROM photos`},
	} {
		if err := conn.QueryRow(q.query).Scan(q.dest); err != nil {
			return c, err
		}
	}
	return c, nil
}

// FlyerInUse reports whether any event still references a flyer file.
// Recurring events share one, so a replaced flyer isn't always safe to
// delete from disk.
func FlyerInUse(conn *sql.DB, filename string) (bool, error) {
	var count int
	err := conn.QueryRow(`SELECT count(*) FROM events WHERE flyer = ?`, filename).Scan(&count)
	return count > 0, err
}

// VolunteersFor returns the roles an event needs covered, in the order
// given by VolunteerRoles.
func VolunteersFor(conn *sql.DB, eventID int64) ([]EventVolunteer, error) {
	rows, err := conn.Query(`SELECT id, event_id, role, volunteer_name FROM event_volunteers WHERE event_id = ?`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byRole := map[string]EventVolunteer{}
	for rows.Next() {
		var v EventVolunteer
		if err := rows.Scan(&v.ID, &v.EventID, &v.Role, &v.Name); err != nil {
			return nil, err
		}
		byRole[v.Role] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var volunteers []EventVolunteer
	for _, role := range VolunteerRoles {
		if v, ok := byRole[role]; ok {
			volunteers = append(volunteers, v)
			delete(byRole, role)
		}
	}
	// Anything the old site recorded under a role we don't know about
	// still belongs to the event.
	for _, v := range byRole {
		volunteers = append(volunteers, v)
	}
	return volunteers, nil
}

// OpenRoles returns just the roles still needing someone.
func OpenRoles(volunteers []EventVolunteer) []EventVolunteer {
	var open []EventVolunteer
	for _, v := range volunteers {
		if !v.Filled() {
			open = append(open, v)
		}
	}
	return open
}

// EventMonth is a run of events sharing a calendar month, used to break
// a long list into headed sections.
type EventMonth struct {
	Label  string
	Events []Event
}

// GroupByMonth splits an already-sorted event list into month sections,
// preserving the order it was given.
func GroupByMonth(events []Event) []EventMonth {
	var months []EventMonth
	for _, ev := range events {
		label := ev.StartsAt.Format("January 2006")
		if n := len(months); n > 0 && months[n-1].Label == label {
			months[n-1].Events = append(months[n-1].Events, ev)
			continue
		}
		months = append(months, EventMonth{Label: label, Events: []Event{ev}})
	}
	return months
}

// ListAllEvents returns every event, most recent start time first, for
// admin management (unlike ListUpcomingEvents it includes past events).
func ListAllEvents(conn *sql.DB) ([]Event, error) {
	rows, err := conn.Query(`SELECT ` + eventColumns + ` FROM events ORDER BY starts_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

const productColumns = `id, name, price_cents, placeholder, stripe_url, image, variants`

func ListProducts(conn *sql.DB) ([]Product, error) {
	rows, err := conn.Query(`SELECT ` + productColumns + ` FROM products ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.PriceCents, &p.Placeholder, &p.StripeURL, &p.Image, &p.Variants); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func scanPosts(rows *sql.Rows) ([]Post, error) {
	var posts []Post
	for rows.Next() {
		var p Post
		var publishedAt sql.NullString
		var createdAt string
		if err := rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &publishedAt, &createdAt); err != nil {
			return nil, err
		}
		if publishedAt.Valid {
			t, err := time.Parse(timeLayout, publishedAt.String)
			if err != nil {
				return nil, err
			}
			p.PublishedAt = &t
		}
		created, err := time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return nil, err
		}
		p.CreatedAt = created
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// ListPosts returns published posts only, newest first, for the public blog.
func ListPosts(conn *sql.DB) ([]Post, error) {
	rows, err := conn.Query(`SELECT id, slug, title, body_markdown, published_at, created_at FROM posts WHERE published_at IS NOT NULL ORDER BY published_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPosts(rows)
}

// ListAllPosts returns every post including drafts, newest first, for admin management.
func ListAllPosts(conn *sql.DB) ([]Post, error) {
	rows, err := conn.Query(`SELECT id, slug, title, body_markdown, published_at, created_at FROM posts ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPosts(rows)
}

func ListPhotos(conn *sql.DB) ([]Photo, error) {
	rows, err := conn.Query(`SELECT id, filename, caption FROM photos ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.Filename, &p.Caption); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}
