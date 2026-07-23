# Decay Main

Site for DECAY, a community arts and technology space in Olympia, WA.
Built with Go, Echo, SQLite, [templ](https://templ.guide), and htmx —
see [MANIFESTO.md](MANIFESTO.md) for the stack philosophy.

## Run

Copy `.env.example` to `.env` and set a real `ADMIN_PASSWORD` (and
`SESSION_SECRET`, so admin logins survive a restart):

```bash
cp .env.example .env
go run main.go
```

Then visit http://localhost:8080.

The admin panel lives at `/admin/login` (username `admin` unless
`ADMIN_USERNAME` overrides it) and manages events, shop products, blog
posts, and photos — all stored in `decay.db` (SQLite, created and seeded
automatically on first run).

### Live reload

Go doesn't hot-reload — `go run` compiles once, so every edit needs a
manual stop/rebuild/restart, and the static assets are `go:embed`-ed
into the binary too, so even a CSS tweak needs a rebuild. [air](https://github.com/air-verse/air)
automates that loop:

```bash
go install github.com/air-verse/air@latest
air
```

`.air.toml` runs `templ generate` before every build and watches `.go`,
`.templ`, `.css`, and `.js` files, rebuilding and restarting the server
on save. `decay.db` and `uploads/` aren't touched by a rebuild — they're
runtime data, not compiled in.

## Calendar

SQLite is the record for events. They're edited at `/admin/events`, and
the site publishes them as an iCalendar feed at `/events.ics` that
Nextcloud, Apple Calendar, and Google Calendar can all subscribe to — so
the org calendar and everyone's phone follow the site rather than the
other way round. Subscribing is read-only and needs no credentials:

- **Nextcloud** — Calendar → New calendar → *Add subscription*, paste the
  feed URL. Nextcloud re-fetches it on its own schedule.
- **Apple / Google Calendar** — "Subscribe to calendar" / "From URL".

The feed carries the whole calendar, past events included, so there's no
window rule to be surprised by. Each event's `uid` column is its
iCalendar identity: it has to stay stable for the life of an event, or
subscribers get a duplicate every time they refresh. Events imported from
the old site reuse the `caldav_uid` it already pushed to Nextcloud, so
anything already subscribed there won't see a second copy.

Writing *back* — creating an event inside Nextcloud and having it appear
on the site — would need a real two-way CalDAV sync with conflict rules,
and iCalendar can't carry the fields a DECAY event needs. That's
deliberately not built.

## Flyers and volunteers

These are the fields iCalendar can't express, so they live on the site
and each event has its own page at `/events/<slug>` to hold them.

**Flyers** are images under `uploads/flyers/`, uploaded per event at
`/admin/events/<id>`. They're runtime data, not committed — the originals
alone are ~340 MB. Pages serve a web-sized copy from
`uploads/flyers/web/`, capped at 1200px wide and re-encoded as JPEG,
which takes that set to ~54 MB; the original is one click behind the
image. Both the importer and the admin upload generate the web copy, so
it always exists. The feed links to a flyer with `ATTACH` rather than
embedding it, so calendar clients that support it can show the image
without the feed carrying the bytes.

**Volunteer roles** are rows in `event_volunteers` — door, sound,
cleanup, promote. A row existing means the job is needed; an empty
`volunteer_name` means it's still open. Two deliberate choices here:

- The public event page lists only the roles **still open**. Who has
  signed up is shown in the admin panel and nowhere else.
- Only a volunteer's **name** is stored. The old site also kept their
  email and phone; that contact information is not carried over, and the
  importer drops it.

There's no public sign-up form yet — the page tells people to ask on
Discord or at the door. Adding one means deciding how to handle contact
details and spam, which is worth doing on purpose rather than by
default.

## Event data

The event archive in `db/events.json` is DECAY's real calendar — 676
events back to March 2025 — converted from the old PHP site's flat JSON
files. The old site keeps the current quarter in `data/events/data/` and
rolls finished quarters into `data/archive/<year>/Q<n>/`, so the importer
reads both:

```bash
go run ./cmd/importevents -src ../decay/data/events/data -archive ../decay/data/archive
```

It also copies each referenced flyer image into `uploads/flyers/`
(`-flyers-out ""` skips that).

It only rewrites `db/events.json`. Because seeding runs once, on an
empty `events` table, an existing `decay.db` won't pick up the new file
— delete `decay.db` (or clear the table) to seed again. Anything entered
through `/admin/events` lives only in `decay.db` and would be lost, so
once events are managed there, stop re-importing.

## Structure

- `main.go` — server startup, routes, env config
- `db/` — SQLite schema, queries, and seed data (`events.json`)
- `cmd/importevents/` — one-way converter from the old site's event JSON
- `ics/` — renders events as a subscribable iCalendar feed
- `images/` — makes web-sized copies of uploaded flyers
- `views/` — templ page templates (`.templ` source + generated `_templ.go`)
- `admin/` — session auth and CRUD handlers for `/admin/*`
- `static/css`, `static/js`, `static/img` — assets, embedded into the binary at build time
- `uploads/` — photo and flyer uploads written at runtime (not embedded, not committed)

Editing a `.templ` file requires regenerating its Go code:

```bash
go install github.com/a-h/templ/cmd/templ@latest
templ generate
```
