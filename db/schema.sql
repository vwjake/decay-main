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
    slug TEXT NOT NULL DEFAULT ''
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
-- can't drift per-database. Only 'master' exists so far.
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'master',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_login_at TEXT
);

-- Usernames are compared case-insensitively so "Jake" and "jake" can't
-- both exist and confuse who is who.
CREATE UNIQUE INDEX IF NOT EXISTS users_username ON users(lower(username));

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

-- Public requests to book the space for an event, reviewed by organizers
-- in the admin queue. status moves new -> reviewed; archived hides it.
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
    -- Filename under uploads/products/. Empty falls back to placeholder.
    image TEXT NOT NULL DEFAULT '',
    -- Sizes, colours, prints — shown as text, since the site doesn't
    -- take orders. shop.decay.events still handles the actual selling.
    variants TEXT NOT NULL DEFAULT '',
    -- Optional blurb shown under the item on the shop page.
    description TEXT NOT NULL DEFAULT '',
    -- Marks an item unavailable; it stays listed with a badge.
    sold_out INTEGER NOT NULL DEFAULT 0,
    -- Orders the catalogue; lower comes first.
    position INTEGER NOT NULL DEFAULT 0
);
