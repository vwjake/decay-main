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
    placeholder TEXT NOT NULL DEFAULT '',
    stripe_url TEXT NOT NULL DEFAULT ''
);
