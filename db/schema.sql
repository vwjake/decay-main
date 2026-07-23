CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    event_type TEXT NOT NULL,
    starts_at TEXT NOT NULL,
    ends_at TEXT,
    location TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    link TEXT NOT NULL DEFAULT '#'
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
