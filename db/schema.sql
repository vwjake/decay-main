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

CREATE TABLE IF NOT EXISTS photos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    filename TEXT NOT NULL,
    caption TEXT NOT NULL DEFAULT '',
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
    variants TEXT NOT NULL DEFAULT ''
);
