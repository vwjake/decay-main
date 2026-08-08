package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CreateEvent inserts an event, filling in a UID and slug when the caller
// hasn't supplied them, and returns its new id.
func CreateEvent(conn *sql.DB, e Event) (int64, error) {
	var endsAt any
	if e.EndsAt != nil {
		endsAt = e.EndsAt.Format(timeLayout)
	}
	if e.UID == "" {
		uid, err := NewUID()
		if err != nil {
			return 0, err
		}
		e.UID = uid
	}
	if e.Slug == "" {
		slug, err := uniqueSlug(conn, Slug(e.StartsAt, e.Title))
		if err != nil {
			return 0, err
		}
		e.Slug = slug
	}
	res, err := conn.Exec(
		`INSERT INTO events (title, event_type, starts_at, ends_at, location, description, link, uid, flyer, slug) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Title, e.EventType, e.StartsAt.Format(timeLayout), endsAt, e.Location, e.Description, e.Link, e.UID, e.Flyer, e.Slug,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateEvent saves edits to an event's details. It deliberately leaves
// uid alone: subscribers key on it, so changing it would make everyone's
// calendar drop the event and re-add it as a new one. Slug is editable
// but defaults to unchanged for the same reason — it's published in the
// feed and in any link people have shared.
func UpdateEvent(conn *sql.DB, e Event) error {
	var endsAt any
	if e.EndsAt != nil {
		endsAt = e.EndsAt.Format(timeLayout)
	}
	_, err := conn.Exec(
		`UPDATE events SET title = ?, event_type = ?, starts_at = ?, ends_at = ?, location = ?, description = ?, link = ?, slug = ? WHERE id = ?`,
		e.Title, e.EventType, e.StartsAt.Format(timeLayout), endsAt, e.Location, e.Description, e.Link, e.Slug, e.ID,
	)
	return err
}

// SlugTaken reports whether a slug is already used by a different event.
func SlugTaken(conn *sql.DB, slug string, exceptID int64) (bool, error) {
	var count int
	err := conn.QueryRow(`SELECT count(*) FROM events WHERE slug = ? AND id <> ?`, slug, exceptID).Scan(&count)
	return count > 0, err
}

// SetProductImage points a product at an uploaded photo, returning the
// filename it replaced so the caller can delete it.
func SetProductImage(conn *sql.DB, id int64, filename string) (string, error) {
	var previous string
	if err := conn.QueryRow(`SELECT image FROM products WHERE id = ?`, id).Scan(&previous); err != nil {
		return "", err
	}
	if _, err := conn.Exec(`UPDATE products SET image = ? WHERE id = ?`, filename, id); err != nil {
		return "", err
	}
	return previous, nil
}

// UpdateProduct saves edits to a shop item.
func UpdateProduct(conn *sql.DB, p Product) error {
	_, err := conn.Exec(
		`UPDATE products SET name = ?, price_cents = ?, placeholder = ?, stripe_url = ?, stripe_price_id = ?, variants = ?, description = ?, sold_out = ?, position = ? WHERE id = ?`,
		p.Name, p.PriceCents, p.Placeholder, p.StripeURL, p.StripePriceID, p.Variants, p.Description, p.SoldOut, p.Position, p.ID,
	)
	return err
}

// StripeProduct is one item as Stripe describes it — the fields the shop
// takes from there, with everything else (photo, ordering) staying local.
type StripeProduct struct {
	ProductID   string
	PriceID     string
	Name        string
	Description string
	PriceCents  int
}

// UpsertStripeProduct writes one synced item, matching on stripe_product_id.
// It deliberately lists the columns it touches rather than replacing the
// row: image, variants, placeholder, and position are the site's to own, and
// a sync must not blank the photo an admin uploaded here. Returns whether
// the row was newly created, for the sync summary.
func UpsertStripeProduct(conn *sql.DB, sp StripeProduct) (created bool, err error) {
	res, err := conn.Exec(
		`UPDATE products SET name = ?, price_cents = ?, description = ?, stripe_price_id = ?, sold_out = 0
		 WHERE stripe_product_id = ?`,
		sp.Name, sp.PriceCents, sp.Description, sp.PriceID, sp.ProductID,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return false, nil
	}

	// New to the site. An admin adds the photo afterwards, so it starts on
	// the placeholder and sorts to the end of the catalogue.
	_, err = conn.Exec(
		`INSERT INTO products (name, price_cents, placeholder, description, stripe_product_id, stripe_price_id, position)
		 VALUES (?, ?, 'product photo', ?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM products))`,
		sp.Name, sp.PriceCents, sp.Description, sp.ProductID, sp.PriceID,
	)
	return true, err
}

// RetireMissingStripeProducts marks synced items that Stripe no longer lists
// as sold out. They're kept rather than deleted so the photo and the item's
// ordering survive it being relisted. Local-only rows (no stripe_product_id)
// are never touched. Returns how many were retired.
func RetireMissingStripeProducts(conn *sql.DB, seen []string) (int, error) {
	query := `UPDATE products SET sold_out = 1 WHERE stripe_product_id <> '' AND sold_out = 0`
	args := make([]any, 0, len(seen))
	if len(seen) > 0 {
		placeholders := strings.Repeat(",?", len(seen))[1:]
		query += ` AND stripe_product_id NOT IN (` + placeholders + `)`
		for _, id := range seen {
			args = append(args, id)
		}
	}
	res, err := conn.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// LinkProductToStripe attaches an existing local row to a Stripe product, so
// merch that predates the sync joins up instead of being duplicated by it.
func LinkProductToStripe(conn *sql.DB, id int64, stripeProductID string) error {
	_, err := conn.Exec(`UPDATE products SET stripe_product_id = ? WHERE id = ?`, stripeProductID, id)
	return err
}

// UpdatePost saves edits to a post's slug, title, and body. Its published
// state is changed separately, through PublishPost and UnpublishPost.
func UpdatePost(conn *sql.DB, id int64, slug, title, body string) error {
	_, err := conn.Exec(
		`UPDATE posts SET slug = ?, title = ?, body_markdown = ? WHERE id = ?`,
		slug, title, body, id,
	)
	return err
}

// UnpublishPost returns a post to draft, taking it off the public blog.
func UnpublishPost(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`UPDATE posts SET published_at = NULL WHERE id = ?`, id)
	return err
}

// UpdatePhoto saves a gallery photo's caption and group tag. A nil groupID
// clears the tag.
func UpdatePhoto(conn *sql.DB, id int64, caption string, groupID *int64) error {
	_, err := conn.Exec(`UPDATE photos SET caption = ?, group_id = ? WHERE id = ?`, caption, nullInt(groupID), id)
	return err
}

// SetEventFlyer points an event at an uploaded flyer file, returning the
// filename it replaced so the caller can delete it.
func SetEventFlyer(conn *sql.DB, id int64, filename string) (string, error) {
	var previous string
	if err := conn.QueryRow(`SELECT flyer FROM events WHERE id = ?`, id).Scan(&previous); err != nil {
		return "", err
	}
	if _, err := conn.Exec(`UPDATE events SET flyer = ? WHERE id = ?`, filename, id); err != nil {
		return "", err
	}
	return previous, nil
}

// SetVolunteerRoles replaces an event's volunteer roles with the given
// set. Roles already recorded keep whoever is signed up for them.
func SetVolunteerRoles(conn *sql.DB, eventID int64, roles []string) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	keep := map[string]bool{}
	for _, role := range roles {
		keep[role] = true
		if _, err := tx.Exec(
			`INSERT INTO event_volunteers (event_id, role) VALUES (?, ?) ON CONFLICT (event_id, role) DO NOTHING`,
			eventID, role,
		); err != nil {
			return err
		}
	}

	rows, err := tx.Query(`SELECT role FROM event_volunteers WHERE event_id = ?`, eventID)
	if err != nil {
		return err
	}
	var existing []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			rows.Close()
			return err
		}
		existing = append(existing, role)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, role := range existing {
		if keep[role] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM event_volunteers WHERE event_id = ? AND role = ?`, eventID, role); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AssignVolunteer records who is covering a role, or clears it when name
// is empty.
func AssignVolunteer(conn *sql.DB, eventID int64, role, name string) error {
	_, err := conn.Exec(
		`INSERT INTO event_volunteers (event_id, role, volunteer_name) VALUES (?, ?, ?)
		 ON CONFLICT (event_id, role) DO UPDATE SET volunteer_name = excluded.volunteer_name`,
		eventID, role, name,
	)
	return err
}

// AddVolunteer creates the role/name pair used when seeding imported data.
func AddVolunteer(conn *sql.DB, eventID int64, role, name string) error {
	_, err := conn.Exec(
		`INSERT INTO event_volunteers (event_id, role, volunteer_name) VALUES (?, ?, ?)
		 ON CONFLICT (event_id, role) DO NOTHING`,
		eventID, role, name,
	)
	return err
}

// uniqueSlug appends a counter until the slug is free, so two events with
// the same title on the same day both get a usable URL.
func uniqueSlug(conn *sql.DB, base string) (string, error) {
	if base == "" {
		return "", nil
	}
	candidate := base
	for i := 2; ; i++ {
		var count int
		if err := conn.QueryRow(`SELECT count(*) FROM events WHERE slug = ?`, candidate).Scan(&count); err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

// NewUID returns a random RFC 4122 version 4 UUID, matching the shape of
// the UIDs the old site generated.
func NewUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func DeleteEvent(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM events WHERE id = ?`, id)
	return err
}

func CreateProduct(conn *sql.DB, p Product) error {
	_, err := conn.Exec(
		`INSERT INTO products (name, price_cents, placeholder, stripe_url, description, position) VALUES (?, ?, ?, ?, ?, ?)`,
		p.Name, p.PriceCents, p.Placeholder, p.StripeURL, p.Description, p.Position,
	)
	return err
}

func DeleteProduct(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM products WHERE id = ?`, id)
	return err
}

func CreatePost(conn *sql.DB, slug, title, body string, publish bool) error {
	var publishedAt any
	if publish {
		publishedAt = time.Now().UTC().Format(timeLayout)
	}
	_, err := conn.Exec(
		`INSERT INTO posts (slug, title, body_markdown, published_at) VALUES (?, ?, ?, ?)`,
		slug, title, body, publishedAt,
	)
	return err
}

func PublishPost(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`UPDATE posts SET published_at = ? WHERE id = ?`, time.Now().UTC().Format(timeLayout), id)
	return err
}

func DeletePost(conn *sql.DB, id int64) error {
	_, err := conn.Exec(`DELETE FROM posts WHERE id = ?`, id)
	return err
}

func CreatePhoto(conn *sql.DB, filename, caption string, groupID *int64) error {
	_, err := conn.Exec(`INSERT INTO photos (filename, caption, group_id) VALUES (?, ?, ?)`, filename, caption, nullInt(groupID))
	return err
}

// DeletePhoto removes the DB row and returns its filename so the caller
// can also remove the file from the uploads directory.
func DeletePhoto(conn *sql.DB, id int64) (string, error) {
	var filename string
	if err := conn.QueryRow(`SELECT filename FROM photos WHERE id = ?`, id).Scan(&filename); err != nil {
		return "", err
	}
	if _, err := conn.Exec(`DELETE FROM photos WHERE id = ?`, id); err != nil {
		return "", err
	}
	return filename, nil
}

// CreateOrder inserts a new pending order and returns its ID.
func CreateOrder(conn *sql.DB, o Order) (int64, error) {
	res, err := conn.Exec(
		`INSERT INTO orders (secure_token, customer_name, customer_email, status, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		o.SecureToken, o.CustomerName, o.CustomerEmail, o.Status,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateOrderStatus updates an order's status and updated_at timestamp.
func UpdateOrderStatus(conn *sql.DB, id int64, status string) error {
	_, err := conn.Exec(
		`UPDATE orders SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		status, id,
	)
	return err
}

// SetOrderRedeemCode sets the redeem code for an order.
func SetOrderRedeemCode(conn *sql.DB, id int64, code string) error {
	_, err := conn.Exec(
		`UPDATE orders SET redeem_code = ?, updated_at = datetime('now') WHERE id = ?`,
		code, id,
	)
	return err
}

// OrderByToken fetches an order by its secure token.
func OrderByToken(conn *sql.DB, token string) (Order, error) {
	var o Order
	var createdAt, updatedAt string
	err := conn.QueryRow(
		`SELECT id, secure_token, customer_name, customer_email, status, redeem_code, created_at, updated_at FROM orders WHERE secure_token = ?`,
		token,
	).Scan(&o.ID, &o.SecureToken, &o.CustomerName, &o.CustomerEmail, &o.Status, &o.RedeemCode, &createdAt, &updatedAt)
	if err != nil {
		return o, err
	}
	var errCreatedAt, errUpdatedAt error
	o.CreatedAt, errCreatedAt = time.Parse("2006-01-02 15:04:05", createdAt)
	o.UpdatedAt, errUpdatedAt = time.Parse("2006-01-02 15:04:05", updatedAt)
	if errCreatedAt != nil || errUpdatedAt != nil {
		return o, fmt.Errorf("error parsing timestamps: %v, %v", errCreatedAt, errUpdatedAt)
	}
	return o, nil
}

// AddOrderItem inserts an item into an order.
func AddOrderItem(conn *sql.DB, oi OrderItem) error {
	_, err := conn.Exec(
		`INSERT INTO order_items (order_id, product_id, quantity, price_at_purchase) VALUES (?, ?, ?, ?)`,
		oi.OrderID, oi.ProductID, oi.Quantity, oi.PriceAtPurchase,
	)
	return err
}

// ItemsForOrder fetches all items in an order, joined with product details.
type OrderItemDetail struct {
	OrderItem
	ProductName string
}

func ItemsForOrder(conn *sql.DB, orderID int64) ([]OrderItemDetail, error) {
	rows, err := conn.Query(
		`SELECT oi.id, oi.order_id, oi.product_id, oi.quantity, oi.price_at_purchase, p.name FROM order_items oi
		 JOIN products p ON p.id = oi.product_id WHERE oi.order_id = ?`,
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OrderItemDetail
	for rows.Next() {
		var item OrderItemDetail
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.Quantity, &item.PriceAtPurchase, &item.ProductName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
