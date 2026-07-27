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

The admin panel lives at `/admin/login` and manages events, shop
products, blog posts, and photos — all stored in `decay.db` (SQLite,
created and seeded automatically on first run).

## Accounts

Everyone signs in with their own account. Accounts live in the `users`
table with bcrypt-hashed passwords and are managed at `/admin/users`.

`ADMIN_USERNAME` and `ADMIN_PASSWORD` create the **first** account and
nothing more. Once any account exists they're ignored — changing them
won't change a password or let anyone in, so the real credential lives in
the database from that point on. (Startup deliberately accepts a short
`ADMIN_PASSWORD` for that first account and warns instead of refusing:
the admin panel is the only place to change it, so refusing to boot would
lock everyone out of fixing it.)

Access is by permission, not by role name — `events`, `posts`, `shop`,
`photos`, and `users`. Roles map to sets of those in `db/roles.go`, and
handlers check the permission, so adding a narrower role later means
adding an entry there and changing no handlers. **`master` is the only
role so far** and grants everything.

Two things the panel refuses, because both would lock everyone out
permanently: deleting the account you're signed in as, and removing the
last account that can manage accounts.

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

## Shop

The merch listing is a **catalogue, not a store** — names, prices, photos,
and available sizes/colours, each linking out to shop.decay.events, which
is where orders are actually taken. Nothing here handles carts, stock, or
payment.

`db/products.json` is generated from a shop.decay.events export:

```bash
go run ./cmd/importshop -export ../shop-decay-events-export-2026-07-24-002012
```

Two quirks of that export are handled in the importer. Its point-of-sale
prices **exclude** sales tax — a $30 shirt is listed at 27.32 — so the
tax rate from the same file is applied back, giving the price people
actually pay. And it has no image column at all, so products are matched
to photos by a table in `cmd/importshop/main.go`; a new product needs a
line there. Only the `Merch` category is imported: concessions (popcorn,
coffee) are sold at the door and donations aren't merchandise.

Photos land in `uploads/products/` with web-sized copies alongside, the
same as flyers, and can be replaced per item at `/admin/products/<id>`.

## Calendar

Events are shown three ways: `/calendar` is a month grid like the old
site's, `/events` is a paginated list of what's coming up, and
`/events/archive` is everything past. The grid is laid out in the venue's
timezone, so a 9pm show stays on the night it started rather than sliding
into the next day via UTC.

SQLite is the record for events. They're edited at `/admin/events`, and
the site publishes them as an iCalendar feed at `/events.ics` that
Nextcloud, Apple Calendar, and Google Calendar can all subscribe to — so
the org calendar and everyone's phone follow the site rather than the
other way round. Subscribing is read-only and needs no credentials:

- **Nextcloud** — Calendar → New calendar → *Add subscription*, paste the
  feed URL. Nextcloud re-fetches it on its own schedule.
- **Apple / Google Calendar** — "Subscribe to calendar" / "From URL".

The feed carries the whole calendar, past events included, so there's no
window rule to be surprised by — unlike the pages, it isn't paginated,
since a calendar client wants the lot in one fetch. Each event's `uid` column is its
iCalendar identity: it has to stay stable for the life of an event, or
subscribers get a duplicate every time they refresh. Events imported from
the old site reuse the `caldav_uid` it already pushed to Nextcloud, so
anything already subscribed there won't see a second copy.

Writing *back* — creating an event inside Nextcloud and having it appear
on the site — would need a real two-way CalDAV sync with conflict rules,
and iCalendar can't carry the fields a DECAY event needs. That's
deliberately not built.

### Internal meetings

The public feed above is the site publishing *out* to Nextcloud. The
`/admin/meetings` page does the reverse for DECAY's own business: it
subscribes *in* to a separate, internal Nextcloud calendar (board and
organising meetings) and shows it on a month grid with an upcoming list.
It's the same arrangement — one-way, no stored credentials — just
inverted: point `MEETINGS_ICS_URL` at that calendar's read-only `.ics`
share link and it's read live (cached a few minutes) on each view.
Nothing is ever written back, and leaving the variable unset simply hides
the page. Meetings stay edited in Nextcloud; this is only a window onto
them, gated on a `meetings` permission.

## Pages

`/about`, `/support`, and `/policies` are static copy carried over from
the old site — mission and board, how to give and who funds us, and the
safer space policy. They're in `views/` rather than the database because
nobody edits them week to week; when that changes they should move behind
the admin panel.

Outbound links are only to accounts DECAY actually uses: YouTube
(`@no_tape`), Discord, Patreon, Givebutter, Instagram, and the beehiiv
newsletter. There's no Bandcamp and the Twitch account is unused, so
neither is linked.

## Photos

`/photos` is the gallery, uploaded at `/admin/photos`. Files live under
`uploads/photos/` with web-sized copies in `uploads/photos/web/`, the
same arrangement as flyers and product shots — the page shows the copy
and links the original behind it. Tiles are cropped square so a grid of
mixed phone aspect ratios still reads as a grid; the uncropped original
is one click away.

Captions are optional and double as the image's alt text. Without one the
alt is deliberately empty, which marks the image as decorative rather
than reading a generated filename out to a screen reader.

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
- **The importer carries no names at all** — only which roles an event
  needed. `db/events.json` is committed to git, and the old records hold
  real community members' names, emails, and phone numbers. Who
  volunteered is recorded through the admin panel, into `decay.db`, which
  isn't committed.

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
- `cmd/importshop/` — one-way converter from the shop.decay.events export
- `ics/` — renders events as a subscribable iCalendar feed
- `meetings/` — reads DECAY's internal Nextcloud calendar for `/admin/meetings`
- `markdown/` — renders blog post Markdown to HTML
- `images/` — makes web-sized copies of uploaded flyers
- `views/` — templ page templates (`.templ` source + generated `_templ.go`)
- `admin/` — session auth, permission checks, and CRUD handlers for `/admin/*`
- `db/roles.go` — the permission set each role grants
- `static/css`, `static/js`, `static/img` — assets, embedded into the binary at build time
- `uploads/` — photo and flyer uploads written at runtime (not embedded, not committed)

Editing a `.templ` file requires regenerating its Go code:

```bash
go install github.com/a-h/templ/cmd/templ@latest
templ generate
```
