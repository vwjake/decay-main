package main

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	netmail "net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	// Event times are resolved against the venue's timezone, and Windows
	// has no system timezone database to read it from.
	_ "time/tzdata"

	"decay-main/admin"
	"decay-main/bookingmail"
	"decay-main/db"
	"decay-main/ics"
	"decay-main/mail"
	"decay-main/shop"
	"decay-main/views"

	stripe "github.com/stripe/stripe-go/v79"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed static
var staticFS embed.FS

const uploadsDir = "uploads"

func main() {
	// Loads .env into the process environment if the file exists; a
	// missing .env (e.g. in production, where vars are set directly) is
	// not an error.
	_ = godotenv.Load()

	conn, err := db.Open("decay.db")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	if err := db.Seed(conn); err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		log.Fatal(err)
	}

	// Accounts live in the database. ADMIN_USERNAME/ADMIN_PASSWORD only
	// seed the very first one — after that, accounts are managed at
	// /admin/users and these are ignored.
	if err := bootstrapAdmin(conn); err != nil {
		log.Fatal(err)
	}

	// The calendar grid is laid out in the venue's own timezone, so a
	// late show lands on the day it started rather than the UTC one.
	venue, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Fatal(err)
	}

	// Calendar subscribers keep absolute links, so the feed needs to know
	// where the site actually lives.
	siteURL := os.Getenv("SITE_URL")
	if siteURL == "" {
		siteURL = "http://localhost:8080"
	}

	// Notification mail for the contact form. Disabled (a no-op) unless SMTP
	// is configured; messages are saved to the admin queue regardless.
	mailer := mail.FromEnv()
	if mailer.Enabled() {
		log.Printf("contact-form notifications will be emailed to %s", mailer.To())
	} else {
		log.Println("SMTP not configured — contact messages are saved to /admin/messages only, no email sent. Set SMTP_HOST to enable notifications.")
	}

	// Reads and replies to the booking mailbox for the admin's booking
	// detail page. Disabled (the panel just hides) unless BOOKING_IMAP_HOST
	// is set — this is a real mailbox, not a notification address, so it
	// stays off by default rather than half-configured.
	bookingMailer := bookingmail.New(bookingmail.FromEnv())
	if bookingMailer.Enabled() {
		log.Printf("booking mailbox connected: %s", bookingMailer.Address())
	} else {
		log.Println("BOOKING_IMAP_HOST not set — the booking email history panel is hidden.")
	}

	// Stripe integration for shop checkout. Leave STRIPE_SECRET_KEY unset to disable.
	stripeSecretKey := os.Getenv("STRIPE_SECRET_KEY")
	stripeWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if stripeSecretKey != "" {
		stripe.Key = stripeSecretKey
		log.Println("Stripe Checkout integration enabled")
	} else {
		log.Println("STRIPE_SECRET_KEY not set — Stripe Checkout disabled, shop links out to shop.decay.events")
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// A not-found URL gets a branded page instead of Echo's plain text;
	// every other error falls through to Echo's default handling.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		code := http.StatusInternalServerError
		var he *echo.HTTPError
		if errors.As(err, &he) {
			code = he.Code
		}
		if code == http.StatusNotFound && !c.Response().Committed {
			c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
			c.Response().Status = http.StatusNotFound
			if renderErr := views.NotFound().Render(c.Request().Context(), c.Response()); renderErr == nil {
				return
			}
		}
		e.DefaultHTTPErrorHandler(err, c)
	}

	static, _ := fs.Sub(staticFS, "static")
	e.StaticFS("/static", static)
	e.Static("/uploads", uploadsDir)

	// Admin session cookies are marked Secure whenever the site is served
	// over HTTPS (i.e. in production behind the TLS proxy), so the cookie
	// never leaves over plain HTTP. Local dev on http://localhost keeps it
	// off, or the browser would drop the cookie and login would never stick.
	secureCookies := strings.HasPrefix(siteURL, "https://")

	// The internal staff calendar is read from a separate Nextcloud
	// share; an unset URL simply leaves that admin page dormant.
	admin.Register(e, conn, sessionSecret(), uploadsDir, venue, os.Getenv("STAFF_ICS_URL"), bookingMailer, secureCookies)

	e.GET("/", func(c echo.Context) error {
		events, err := db.ListUpcomingEvents(conn, 4)
		if err != nil {
			return err
		}
		// The home preview shows only what can be bought; the full shop
		// page carries the sold-out items too.
		products, err := db.AvailableProducts(conn)
		if err != nil {
			return err
		}
		videos, err := db.ListVideos(conn)
		if err != nil {
			return err
		}
		return views.Home(events, products, videos).Render(c.Request().Context(), c.Response())
	})

	e.GET("/about", func(c echo.Context) error {
		people, err := db.ListPeople(conn)
		if err != nil {
			return err
		}
		return views.About(people).Render(c.Request().Context(), c.Response())
	})

	e.GET("/support", func(c echo.Context) error {
		return views.Support().Render(c.Request().Context(), c.Response())
	})

	e.GET("/policies", func(c echo.Context) error {
		return views.Policies().Render(c.Request().Context(), c.Response())
	})

	e.GET("/events", func(c echo.Context) error {
		events, err := db.UpcomingEvents(conn)
		if err != nil {
			return err
		}
		events, page := db.Paginate(events, db.PageNumber(c.QueryParam("page")), db.PerPagePublic)
		page.Path = "/events"
		if wantsJSON(c) {
			return c.JSON(http.StatusOK, db.EventsResponse(siteURL, events))
		}
		return views.Events(db.GroupByMonth(events), page).Render(c.Request().Context(), c.Response())
	})

	// The subscribable feed carries the whole calendar, past included, so
	// there's no window rule for subscribers to be surprised by.
	e.GET("/events.ics", func(c echo.Context) error {
		events, err := db.ListAllEvents(conn)
		if err != nil {
			return err
		}
		return c.Blob(http.StatusOK, "text/calendar; charset=utf-8", ics.Calendar("DECAY", siteURL, events))
	})

	e.GET("/calendar", func(c echo.Context) error {
		month := db.ParseMonth(c.QueryParam("month"), venue)
		events, err := db.EventsInMonth(conn, month)
		if err != nil {
			return err
		}
		return views.Calendar(db.BuildCalendar(events, month, venue)).Render(c.Request().Context(), c.Response())
	})

	e.GET("/events/archive", func(c echo.Context) error {
		events, err := db.PastEvents(conn)
		if err != nil {
			return err
		}
		events, page := db.Paginate(events, db.PageNumber(c.QueryParam("page")), db.PerPagePublic)
		page.Path = "/events/archive"
		return views.EventArchive(db.GroupByMonth(events), page).Render(c.Request().Context(), c.Response())
	})

	e.GET("/events/:slug", func(c echo.Context) error {
		ev, err := db.EventBySlug(conn, c.Param("slug"))
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}
		volunteers, err := db.VolunteersFor(conn, ev.ID)
		if err != nil {
			return err
		}
		openRoles := db.OpenRoles(volunteers)
		if wantsJSON(c) {
			return c.JSON(http.StatusOK, ev.Response(siteURL, openRoles))
		}
		signup := views.SignupBox{
			// Offer the signup form only for upcoming events that the admin
			// has actually flagged as needing volunteers.
			Show: ev.StartsAt.After(time.Now()) && len(volunteers) > 0,
			Done: c.QueryParam("signed") != "",
		}
		return views.EventDetail(ev, openRoles, signup, views.EventMeta(ev, siteURL)).Render(c.Request().Context(), c.Response())
	})

	e.POST("/events/:slug/volunteer", func(c echo.Context) error {
		return submitSignup(c, conn, siteURL)
	})

	e.GET("/book", func(c echo.Context) error {
		return views.BookingForm(db.BookingRequest{}, c.QueryParam("sent") != "", "").Render(c.Request().Context(), c.Response())
	})

	e.POST("/book", func(c echo.Context) error {
		return submitBooking(c, conn)
	})

	e.GET("/contact", func(c echo.Context) error {
		return views.ContactForm(db.ContactMessage{}, c.QueryParam("sent") != "", "").Render(c.Request().Context(), c.Response())
	})

	e.POST("/contact", func(c echo.Context) error {
		return submitContact(c, conn, mailer)
	})

	e.GET("/bios", func(c echo.Context) error {
		bios, err := db.PublicCommunityBios(conn)
		if err != nil {
			return err
		}
		return views.Bios(bios).Render(c.Request().Context(), c.Response())
	})

	e.GET("/get-involved", func(c echo.Context) error {
		forms, err := db.EnabledExternalForms(conn)
		if err != nil {
			return err
		}
		return views.GetInvolved(forms).Render(c.Request().Context(), c.Response())
	})

	e.GET("/shop", func(c echo.Context) error {
		products, err := db.ListProducts(conn)
		if err != nil {
			return err
		}
		return views.Shop(products).Render(c.Request().Context(), c.Response())
	})

	// Stripe Checkout routes (only active if STRIPE_SECRET_KEY is set)
	if stripeSecretKey != "" {
		e.POST("/shop/checkout", func(c echo.Context) error {
			return handleShopCheckout(c, conn, siteURL)
		})

		e.GET("/order/confirm", func(c echo.Context) error {
			return handleOrderConfirm(c, conn)
		})

		e.GET("/api/order-status", func(c echo.Context) error {
			return handleOrderStatus(c, conn)
		})

		e.POST("/webhooks/stripe", func(c echo.Context) error {
			return handleStripeWebhook(c, conn, stripeWebhookSecret)
		})
	}

	e.GET("/groups", func(c echo.Context) error {
		groups, err := db.EnabledGroups(conn)
		if err != nil {
			return err
		}
		return views.Groups(groups).Render(c.Request().Context(), c.Response())
	})

	e.GET("/groups/:slug", func(c echo.Context) error {
		group, err := db.GroupBySlug(conn, c.Param("slug"))
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}
		upcoming, err := db.UpcomingForGroup(conn, group, 6)
		if err != nil {
			return err
		}
		photos, err := db.PhotosForGroup(conn, group.ID)
		if err != nil {
			return err
		}
		return views.GroupPage(group, upcoming, photos).Render(c.Request().Context(), c.Response())
	})

	e.GET("/blog", func(c echo.Context) error {
		posts, err := db.ListPosts(conn)
		if err != nil {
			return err
		}
		if wantsJSON(c) {
			return c.JSON(http.StatusOK, db.PostsResponse(siteURL, posts))
		}
		return views.Blog(posts).Render(c.Request().Context(), c.Response())
	})

	e.GET("/blog/:slug", func(c echo.Context) error {
		post, err := db.PostBySlug(conn, c.Param("slug"))
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return err
		}
		if wantsJSON(c) {
			return c.JSON(http.StatusOK, post.Response(siteURL))
		}
		return views.PostPage(post, views.PostMeta(post, siteURL)).Render(c.Request().Context(), c.Response())
	})

	e.GET("/photos", func(c echo.Context) error {
		photos, err := db.ListPhotos(conn)
		if err != nil {
			return err
		}
		return views.Photos(photos).Render(c.Request().Context(), c.Response())
	})

	// PORT lets the host pick the listen port; it defaults to 8080 for
	// local development.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	e.Logger.Fatal(e.Start(":" + port))
}

// wantsJSON reports whether the client asked for a JSON representation of a
// page that can serve both. The rule is explicit opt-in: only an Accept header
// naming application/json gets JSON. Ordinary browser navigation
// (Accept: text/html,...), htmx requests (Accept: */*), and link-preview
// scrapers all fall through to the HTML page, so previews and progressive
// enhancement keep working. Admin endpoints are JSON-only and don't use this.
func wantsJSON(c echo.Context) bool {
	return strings.Contains(c.Request().Header.Get(echo.HeaderAccept), "application/json")
}

// submitBooking validates and stores a public booking request. A filled
// honeypot field is treated as a bot and silently dropped.
func submitBooking(c echo.Context, conn *sql.DB) error {
	if strings.TrimSpace(c.FormValue("website")) != "" {
		return c.Redirect(http.StatusSeeOther, "/book?sent=1")
	}
	b := db.BookingRequest{
		Name:               strings.TrimSpace(c.FormValue("name")),
		Email:              strings.TrimSpace(c.FormValue("email")),
		Phone:              strings.TrimSpace(c.FormValue("phone")),
		EventName:          strings.TrimSpace(c.FormValue("event_name")),
		Description:        strings.TrimSpace(c.FormValue("description")),
		PreferredDate:      strings.TrimSpace(c.FormValue("preferred_date")),
		ExpectedAttendance: strings.TrimSpace(c.FormValue("expected_attendance")),
	}
	render := func(msg string) error {
		return views.BookingForm(b, false, msg).Render(c.Request().Context(), c.Response())
	}
	if b.Name == "" || b.Description == "" {
		return render("Please add your name and a bit about your event.")
	}
	if b.Email == "" && b.Phone == "" {
		return render("Please leave an email or phone so we can reach you.")
	}
	if err := db.CreateBookingRequest(conn, b); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/book?sent=1")
}

// submitContact validates and stores a public contact message, then fires a
// best-effort email notification. A filled honeypot field is treated as a bot
// and silently dropped. The database row is the record; a mail failure is
// logged but never shown to the sender, whose message is already saved.
func submitContact(c echo.Context, conn *sql.DB, mailer *mail.Mailer) error {
	if strings.TrimSpace(c.FormValue("website")) != "" {
		return c.Redirect(http.StatusSeeOther, "/contact?sent=1")
	}
	m := db.ContactMessage{
		Name:    strings.TrimSpace(c.FormValue("name")),
		Email:   strings.TrimSpace(c.FormValue("email")),
		Subject: strings.TrimSpace(c.FormValue("subject")),
		Message: strings.TrimSpace(c.FormValue("message")),
	}
	render := func(msg string) error {
		return views.ContactForm(m, false, msg).Render(c.Request().Context(), c.Response())
	}
	if m.Name == "" || m.Message == "" {
		return render("Please add your name and a message.")
	}
	if !validEmail(m.Email) {
		return render("Please enter a valid email so we can reply.")
	}
	if err := db.CreateContactMessage(conn, m); err != nil {
		return err
	}
	notifyContact(mailer, m)
	return c.Redirect(http.StatusSeeOther, "/contact?sent=1")
}

// notifyContact emails the saved message to the DECAY inbox off the request's
// path, so a slow or failing mail server never delays the sender's thank-you
// page. The message is already saved, so a dropped email loses nothing.
func notifyContact(mailer *mail.Mailer, m db.ContactMessage) {
	if !mailer.Enabled() {
		return
	}
	subject := "Contact form: " + m.SubjectOr()
	body := fmt.Sprintf("Name: %s\nEmail: %s\n\n%s", m.Name, m.Email, m.Message)
	go func() {
		if err := mailer.Notify(subject, body, m.Email); err != nil {
			log.Printf("contact-form email to %s failed (message is saved in /admin/messages): %v", mailer.To(), err)
		}
	}()
}

// validEmail reports whether addr parses as a single email address.
func validEmail(addr string) bool {
	if addr == "" {
		return false
	}
	_, err := netmail.ParseAddress(addr)
	return err == nil
}

// submitSignup validates and stores a public volunteer offer for an event.
// A filled honeypot field is treated as a bot and silently dropped.
func submitSignup(c echo.Context, conn *sql.DB, siteURL string) error {
	ev, err := db.EventBySlug(conn, c.Param("slug"))
	if errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.FormValue("website")) != "" {
		return c.Redirect(http.StatusSeeOther, ev.Path()+"?signed=1")
	}

	volunteers, err := db.VolunteersFor(conn, ev.ID)
	if err != nil {
		return err
	}
	openRoles := db.OpenRoles(volunteers)

	name := strings.TrimSpace(c.FormValue("name"))
	contact := strings.TrimSpace(c.FormValue("contact"))
	if name == "" || contact == "" {
		signup := views.SignupBox{Show: true, Error: "Please leave your name and a way to reach you."}
		return views.EventDetail(ev, openRoles, signup, views.EventMeta(ev, siteURL)).Render(c.Request().Context(), c.Response())
	}

	// Only accept a role the event actually has open; anything else falls
	// back to "wherever needed".
	role := c.FormValue("role")
	if role != "" && !roleIsOpen(openRoles, role) {
		role = ""
	}

	if err := db.CreateVolunteerSignup(conn, db.VolunteerSignup{
		EventID: ev.ID,
		Role:    role,
		Name:    name,
		Contact: contact,
		Note:    strings.TrimSpace(c.FormValue("note")),
	}); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, ev.Path()+"?signed=1")
}

func roleIsOpen(openRoles []db.EventVolunteer, role string) bool {
	for _, r := range openRoles {
		if r.Role == role {
			return true
		}
	}
	return false
}

// bootstrapAdmin creates the first master account from the environment
// when the database has none. Once any account exists this does nothing,
// so ADMIN_PASSWORD stops being a live credential the moment the panel is
// set up — it can't be used to log in, only to create that first account.
func bootstrapAdmin(conn *sql.DB) error {
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		return errors.New("no accounts exist yet — set ADMIN_PASSWORD to create the first one (see .env.example)")
	}
	username := os.Getenv("ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}

	created, err := db.EnsureFirstUser(conn, username, password)
	if err != nil {
		return fmt.Errorf("creating the first account: %w", err)
	}
	if created {
		log.Printf("created the first admin account %q from ADMIN_USERNAME/ADMIN_PASSWORD — "+
			"manage accounts at /admin/users from now on", username)
		if len([]rune(password)) < db.MinPasswordLength {
			log.Printf("WARNING: that password is under %d characters. Change it at /admin/users; "+
				"ADMIN_PASSWORD is ignored from here on.", db.MinPasswordLength)
		}
	}
	return nil
}

func sessionSecret() []byte {
	if s := os.Getenv("SESSION_SECRET"); s != "" {
		return []byte(s)
	}
	log.Println("SESSION_SECRET not set — generating an ephemeral one; admin sessions won't survive a restart. Set SESSION_SECRET in production.")
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatal(err)
	}
	return secret
}

// handleShopCheckout creates a Stripe Checkout Session for the requested products.
func handleShopCheckout(c echo.Context, conn *sql.DB, siteURL string) error {
	// Parse product IDs and quantities from form
	// For MVP, we'll accept ?product_id=ID&quantity=QTY format
	productID := c.FormValue("product_id")
	quantity := c.FormValue("quantity")
	email := c.FormValue("email")

	if productID == "" || quantity == "" || email == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing product_id, quantity, or email"})
	}

	id, err := strconv.ParseInt(productID, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid product_id"})
	}

	qty, err := strconv.Atoi(quantity)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quantity"})
	}

	// Create Checkout Session
	successURL := siteURL + "/order/confirm?token={CHECKOUT_SESSION_ID}"
	cancelURL := siteURL + "/shop"

	sessionID, orderToken, err := shop.CreateCheckoutSession(conn, shop.CreateCheckoutSessionParams{
		ProductIDs: []int64{id},
		Quantities: []int{qty},
		Email:      email,
		SuccessURL: successURL,
		CancelURL:  cancelURL,
	})
	if err != nil {
		log.Printf("error creating checkout session: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create checkout session"})
	}

	// Redirect to Stripe Checkout
	return c.JSON(http.StatusOK, map[string]string{
		"sessionId":  sessionID,
		"orderToken": orderToken,
	})
}

// handleOrderConfirm shows the order confirmation page with polling for payment confirmation.
func handleOrderConfirm(c echo.Context, conn *sql.DB) error {
	// For MVP, just show a placeholder confirmation page
	// In a full implementation, this would display the order details and redeem code
	token := c.QueryParam("token")
	if token == "" {
		return c.String(http.StatusBadRequest, "Missing order token")
	}

	return c.String(http.StatusOK, fmt.Sprintf("Order confirmation page for token: %s (placeholder)", token))
}

// handleOrderStatus returns the status of an order for client-side polling.
func handleOrderStatus(c echo.Context, conn *sql.DB) error {
	token := c.QueryParam("token")
	if token == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing token"})
	}

	order, err := db.OrderByToken(conn, token)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "order not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch order"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":      order.Status,
		"redeem_code": order.RedeemCode,
	})
}

// handleStripeWebhook processes incoming Stripe webhook events.
func handleStripeWebhook(c echo.Context, conn *sql.DB, webhookSecret string) error {
	payload, err := io.ReadAll(c.Request().Body)
	if err != nil {
		log.Printf("error reading webhook payload: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "error reading request body"})
	}

	signature := c.Request().Header.Get("Stripe-Signature")
	if signature == "" {
		log.Println("webhook missing Stripe-Signature header")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing Stripe-Signature header"})
	}

	if err := shop.HandleStripeWebhook(conn, payload, signature, webhookSecret); err != nil {
		log.Printf("error processing webhook: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("webhook error: %v", err)})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "received"})
}
