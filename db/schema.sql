CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    event_type TEXT NOT NULL,
    starts_at TEXT NOT NULL,
    ends_at TEXT,
    location TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    link TEXT NOT NULL DEFAULT '#',
    -- iCalendar UID. Subscribers key on this, so it has to stay stable
    -- for the life of an event or clients duplicate it on every refresh.
    uid TEXT NOT NULL DEFAULT '',
    -- Filename under uploads/flyers/. Empty when an event has no flyer.
    flyer TEXT NOT NULL DEFAULT '',
    -- Public URL segment. Also goes out in the calendar feed, so it
    -- shouldn't change once an event has been published.
    slug TEXT NOT NULL DEFAULT '',
    -- Who's covering the space for this event. Free text rather than a
    -- link to users.id: the admin form suggests existing accounts but
    -- also accepts anyone without one, and a name should stay attached
    -- to the event even if that account is later removed.
    keyholder TEXT NOT NULL DEFAULT '',
    -- The organizer's contact info, carried over automatically when an
    -- event is converted from a booking request. Lets the admin event page
    -- show the same email correspondence lookup the booking page has.
    contact_name TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    -- Groups events stamped out together by the Repeat tool so their shared
    -- details (title, type, description, location, link, flyer) can be kept
    -- in sync from one place. 0 means the event isn't part of a series; a
    -- non-zero value is the id of the series' first event, whether or not
    -- that row still exists.
    series_id INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS events_slug ON events(slug) WHERE slug <> '';

-- One row per volunteer role an event needs. The row existing is what
-- marks the role as needed; volunteer_name empty means it's still open.
-- Deliberately name-only: the old site also stored volunteers' email and
-- phone, and that contact information is not worth holding here.
CREATE TABLE IF NOT EXISTS event_volunteers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    volunteer_name TEXT NOT NULL DEFAULT '',
    UNIQUE (event_id, role)
);

-- Admin accounts. Roles are defined in Go (db/roles.go) rather than in a
-- table, so the set of permissions a role grants is reviewable in code and
-- can't drift per-database: 'master', 'manager', and 'keyholder'.
-- photo and blurb are the account's own profile, edited at /admin/account.
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'master',
    -- Filename under uploads/avatars/. Empty when there's no photo.
    photo TEXT NOT NULL DEFAULT '',
    -- Free text about yourself, capped at 250 characters and stored
    -- stripped of markup (db.SanitizeBlurb).
    blurb TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_login_at TEXT
);

-- Usernames are compared case-insensitively so "Jake" and "jake" can't
-- both exist and confuse who is who.
CREATE UNIQUE INDEX IF NOT EXISTS users_username ON users(lower(username));

-- Signup links a master or manager sends out so someone can create their
-- own account rather than being handed a password. role and email are set
-- when the link is issued; display_name is just a suggestion the signup
-- form prefills, left blank if there isn't one yet. used_at marks the link
-- spent — a token is good for one account, once, before it expires.
CREATE TABLE IF NOT EXISTS invites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token TEXT NOT NULL,
    role TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    invited_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL,
    used_at TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS invites_token ON invites(token);

CREATE TABLE IF NOT EXISTS posts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT UNIQUE NOT NULL,
    title TEXT NOT NULL,
    body_markdown TEXT NOT NULL,
    published_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Images uploaded for a blog post. The author drops them into the post's
-- Markdown body by URL; this table just tracks the files so the edit page
-- can list them and so they're removed when the post is deleted. Files live
-- under uploads/blog/ with a web-sized copy under uploads/blog/web/.
CREATE TABLE IF NOT EXISTS post_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    filename TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Recurring meetups that call DECAY home (Open Draw, No Tape, etc.). Each
-- has a public detail page. body holds the titled sections in a small
-- line-based markup — "# Heading" starts a section, "- item" is a bullet,
-- other lines are paragraphs — so admins edit it in one textarea. enabled
-- hides a group from the public list without deleting it; position orders
-- both the list and any nav.
CREATE TABLE IF NOT EXISTS groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    pills TEXT NOT NULL DEFAULT '',
    hero_image TEXT NOT NULL DEFAULT '',
    hero_alt TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    -- Terms (one per line) that tie events to this group. A term matches
    -- when it appears in an event's title or type, so the group page can
    -- show its own upcoming schedule off the shared calendar.
    match_terms TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS groups_slug ON groups(slug);

-- Board and staff profiles shown on the About page. position orders them
-- (board first, then staff, however they're arranged); role is the title
-- line under the name, and photo is a filename under uploads/people/.
CREATE TABLE IF NOT EXISTS people (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    pronouns TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    photo TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Public requests to book the space for an event, worked through in the
-- admin queue: read it, maybe convert it to an event, delete it once
-- handled. status only ever holds 'new' now — there's no reviewed/archived
-- workflow — kept as a column rather than dropped so it doesn't need a
-- migration to remove.
CREATE TABLE IF NOT EXISTS booking_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    event_name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    preferred_date TEXT NOT NULL DEFAULT '',
    expected_attendance TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'new',
    -- Private admin notes — never shown to the requester.
    notes TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Messages from the public contact form, worked through in the admin queue
-- the same way booking requests are. status moves new -> reviewed; archived
-- hides it. The message is always saved here; email notification (if SMTP is
-- configured) is a best-effort extra on top, so nothing is lost when mail is
-- down or unconfigured.
CREATE TABLE IF NOT EXISTS contact_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    subject TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'new',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Public offers to volunteer for an event, with contact details so
-- organizers can follow up. Distinct from event_volunteers, which is the
-- admin's record of who is actually covering a role.
CREATE TABLE IF NOT EXISTS volunteer_signups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    contact TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS photos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    filename TEXT NOT NULL,
    caption TEXT NOT NULL DEFAULT '',
    -- Optional group tag; a tagged photo shows on that group's page.
    -- SET NULL rather than cascade so deleting a group leaves the photo.
    group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Post-event operational numbers, one row per event. Filled in after the
-- event from the reports screen; no row simply means the numbers haven't
-- been recorded yet. attendance and door_cents are nullable so "recorded
-- as zero" reads differently from "never entered".
CREATE TABLE IF NOT EXISTS event_reports (
    event_id INTEGER PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
    attendance INTEGER,
    door_cents INTEGER,
    notes TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Donations, in cents. event_id ties a donation to an event (the jar at a
-- show); NULL is a standalone donation — online, a mailed check — that
-- still belongs in the quarter's totals. received_at is the date the money
-- came in, which is what reports range on.
CREATE TABLE IF NOT EXISTS donations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER REFERENCES events(id) ON DELETE SET NULL,
    amount_cents INTEGER NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    received_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Embedded videos shown in the home page's media section. Only the
-- YouTube id is stored; the embed URL is built from it. position orders
-- the list.
CREATE TABLE IF NOT EXISTS media_videos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    youtube_id TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Community member bios shown on the public /bios page. Entered by admins —
-- there's no public submission form — and broader than the board/staff People
-- table (DJs, artists, regulars). public=0 keeps a bio on file (e.g. collected
-- for a grant application) without showing it on the site. position orders the
-- list; no photo, unlike People.
CREATE TABLE IF NOT EXISTS community_bios (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    pronouns TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    public INTEGER NOT NULL DEFAULT 1,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Links to off-site forms (Nextcloud Forms surveys and the like) shown on the
-- public Get Involved page and managed at /admin/forms. The site doesn't host
-- these — it points at them, the same way it links out to the shop and the
-- newsletter. position orders them; enabled hides one without deleting.
CREATE TABLE IF NOT EXISTS external_forms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    price_cents INTEGER NOT NULL,
    -- Text shown in place of a photo when there isn't one yet.
    placeholder TEXT NOT NULL DEFAULT '',
    stripe_url TEXT NOT NULL DEFAULT '',
    -- Stripe is the catalogue's source of truth: /admin/products syncs name,
    -- price, and description down from it. The photo is deliberately not —
    -- it stays local, in uploads/products/, keyed by the image column below.
    --
    -- Rows are matched on stripe_product_id, which survives a price change;
    -- stripe_price_id is whatever that product's current one-time price is
    -- and gets rewritten on each sync, since Stripe prices are immutable and
    -- editing an amount mints a new id. A row with no stripe_product_id is
    -- local-only and sync never touches it.
    stripe_product_id TEXT NOT NULL DEFAULT '',
    stripe_price_id TEXT NOT NULL DEFAULT '',
    -- Filename under uploads/products/. Empty falls back to placeholder.
    image TEXT NOT NULL DEFAULT '',
    -- Sizes, colours, prints — shown as text only; Checkout has no way to
    -- capture which one a buyer picked.
    variants TEXT NOT NULL DEFAULT '',
    -- Optional blurb shown under the item on the shop page.
    description TEXT NOT NULL DEFAULT '',
    -- Marks an item unavailable; it stays listed with a badge.
    sold_out INTEGER NOT NULL DEFAULT 0,
    -- Orders the catalogue; lower comes first.
    position INTEGER NOT NULL DEFAULT 0
);

-- Shop orders from Stripe Checkout Sessions. status moves pending -> paid
-- on the checkout.session.completed webhook.
CREATE TABLE IF NOT EXISTS orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    -- Unguessable id for the confirmation URL, so an order page can't be
    -- walked by incrementing a number.
    secure_token TEXT NOT NULL UNIQUE,
    customer_name TEXT NOT NULL,
    customer_email TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    -- Short human-readable reference a customer quotes when they get in
    -- touch. Assigned once the order is paid; nothing is redeemed with it.
    redeem_code TEXT UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Items within an order
CREATE TABLE IF NOT EXISTS order_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id INTEGER NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL,
    price_at_purchase INTEGER NOT NULL
);
