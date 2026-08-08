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
	// StripeProductID is the sync key; empty means the item is local-only
	// and a sync leaves it alone. StripePriceID is the current one-time
	// price, rewritten on each sync.
	StripeProductID string
	StripePriceID   string
	Image           string
	Variants        string
	// Description is optional copy shown under the item on the shop page.
	Description string
	// SoldOut marks an item as unavailable — it stays on the page with a
	// badge instead of disappearing.
	SoldOut bool
	// Position orders the catalogue; lower comes first.
	Position int
}

// HasImage reports whether there's a photo to show instead of the
// placeholder text.
func (p Product) HasImage() bool { return p.Image != "" }

// HasDescription reports whether there's blurb worth rendering.
func (p Product) HasDescription() bool { return p.Description != "" }

// BuyURL is where a listing sends someone to actually buy it. The site
// displays the catalogue only; shop.decay.events takes the orders.
func (p Product) BuyURL() string {
	if p.StripeURL != "" {
		return p.StripeURL
	}
	return "https://shop.decay.events"
}

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

type Order struct {
	ID            int64
	SecureToken   string
	CustomerName  string
	CustomerEmail string
	Status        string
	RedeemCode    *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type OrderItem struct {
	ID              int64
	OrderID         int64
	ProductID       int64
	Quantity        int
	PriceAtPurchase int
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

// Path is the post's public URL.
func (p Post) Path() string { return "/blog/" + p.Slug }

// Date renders a post's publication date for display.
func (p Post) Date() string {
	if p.PublishedAt == nil {
		return ""
	}
	return p.PublishedAt.Format("January 2, 2006")
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

// PhotosSubdir keeps gallery shots apart from flyers and product photos
// inside the uploads directory.
const PhotosSubdir = "photos"

type Photo struct {
	ID       int64
	Filename string
	Caption  string
	// GroupID optionally tags a photo to a group, so it shows on that
	// group's page. Nil means untagged.
	GroupID *int64
}

// InGroup reports whether the photo is tagged to the given group, for
// pre-selecting the admin dropdown.
func (p Photo) InGroup(id int64) bool { return p.GroupID != nil && *p.GroupID == id }

// Path is the original, full-resolution upload.
func (p Photo) Path() string { return "/uploads/" + PhotosSubdir + "/" + p.Filename }

// WebPath is the web-sized copy, which is what the gallery displays.
func (p Photo) WebPath() string {
	return "/uploads/" + PhotosSubdir + "/web/" + strings.TrimSuffix(p.Filename, path.Ext(p.Filename)) + ".jpg"
}

// Alt is the image's alternative text. A caption doubles as the
// description; without one there's nothing meaningful to say, and an
// empty alt correctly marks the image as decorative rather than
// repeating a filename to a screen reader.
func (p Photo) Alt() string { return p.Caption }

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
	err := scanProduct(conn.QueryRow(`SELECT `+productColumns+` FROM products WHERE id = ?`, id), &p)
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
	Drafts         int
	PublishedPosts int
	Products       int
	Photos         int
	NewBookings    int
	NewMessages    int
	NewSignups     int
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
		{&c.NewBookings, `SELECT count(*) FROM booking_requests WHERE status = 'new'`},
		{&c.NewMessages, `SELECT count(*) FROM contact_messages WHERE status = 'new'`},
	} {
		if err := conn.QueryRow(q.query).Scan(q.dest); err != nil {
			return c, err
		}
	}

	signups, err := CountSignupsForUpcoming(conn)
	if err != nil {
		return c, err
	}
	c.NewSignups = signups
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

const productColumns = `id, name, price_cents, placeholder, stripe_url, stripe_product_id, stripe_price_id, image, variants, description, sold_out, position`

func scanProduct(s interface {
	Scan(...any) error
}, p *Product) error {
	return s.Scan(&p.ID, &p.Name, &p.PriceCents, &p.Placeholder, &p.StripeURL, &p.StripeProductID, &p.StripePriceID, &p.Image, &p.Variants, &p.Description, &p.SoldOut, &p.Position)
}

// SyncedFromStripe reports whether a row is tied to a Stripe product, and
// so has its name, price, and description managed there rather than here.
func (p Product) SyncedFromStripe() bool { return p.StripeProductID != "" }

func ListProducts(conn *sql.DB) ([]Product, error) {
	rows, err := conn.Query(`SELECT ` + productColumns + ` FROM products ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := scanProduct(rows, &p); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

// AvailableProducts is the catalogue with sold-out items dropped, for
// places that only want what can actually be bought.
func AvailableProducts(conn *sql.DB) ([]Product, error) {
	all, err := ListProducts(conn)
	if err != nil {
		return nil, err
	}
	var out []Product
	for _, p := range all {
		if !p.SoldOut {
			out = append(out, p)
		}
	}
	return out, nil
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

func scanPhotos(rows *sql.Rows) ([]Photo, error) {
	var photos []Photo
	for rows.Next() {
		var p Photo
		var groupID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Filename, &p.Caption, &groupID); err != nil {
			return nil, err
		}
		if groupID.Valid {
			p.GroupID = &groupID.Int64
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}

func ListPhotos(conn *sql.DB) ([]Photo, error) {
	rows, err := conn.Query(`SELECT id, filename, caption, group_id FROM photos ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPhotos(rows)
}

// PhotosForGroup returns the photos tagged to a group, newest first.
func PhotosForGroup(conn *sql.DB, groupID int64) ([]Photo, error) {
	rows, err := conn.Query(`SELECT id, filename, caption, group_id FROM photos WHERE group_id = ? ORDER BY id DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPhotos(rows)
}
