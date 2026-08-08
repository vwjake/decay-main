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

The admin panel lives at `/admin/login`. It manages events, shop
products, blog posts, photos, board/staff people, groups, media, and
site forms, and it collects booking requests, volunteer sign-ups, and
contact messages — all stored in `decay.db` (SQLite, created and seeded
automatically on first run).

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
`photos`, `people`, `groups`, `media`, `bookings`, `messages`, `forms`,
`reports`, `staff`, and `users`. Roles map to sets of those in
`db/roles.go`, and handlers check the permission, so adding a narrower
role later means adding an entry there and changing no handlers. There
are three:

- **Keyholder** — running the space: events, bookings, messages, photos,
  reports, and the staff calendar. Not the shop, the site's copy, or
  accounts.
- **Manager** — everything, including creating accounts and handing out
  access.
- **Master** — everything a manager has, and hidden. It's the owner
  account, so `/admin/users` is the one place the difference shows.

**A master is invisible to anyone who isn't one.** Managers don't see
master accounts in the list, aren't offered the role on any form, and get
a 404 — not a 403 — from a master's edit, password, or delete URL, since
"forbidden" would confirm the account is there. The role only appears to
a master. `ADMIN_USERNAME`/`ADMIN_PASSWORD` still creates the first
account as a master, so a fresh install has one.

Two things the panel refuses, because both would lock everyone out
permanently: deleting the account you're signed in as, and removing the
last account that can manage accounts.

### Your account

`/admin/account` is everyone's own page, behind no permission at all — a
keyholder who can reach nothing else still has one. It holds the display
name shown around the panel, a photo, a blurb of up to 250 characters,
and a password change (which asks for the current password, so a session
left open isn't enough to lock its owner out).

The blurb is stored as plain text, whatever gets posted: `db.SanitizeBlurb`
strips tags, drops `script` and `style` bodies whole, and removes control
characters before it ever reaches the column. The templates escape what
they render, so that isn't what stands between the site and an injection
— it's so nothing executable is in the database to begin with, for
whatever reads it later. An account manager can clear someone's blurb or
photo from `/admin/users/<id>`; nobody but the owner can set them.

Photos land in `uploads/avatars/` with a web-sized copy alongside, the
same as flyers and product shots.

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

**Stripe is the catalogue's source of truth.** *Sync from Stripe* at
`/admin/products` pulls names, prices, and descriptions down. There's no
add-product form: an item created here that Stripe doesn't know about is
only a way to get the two out of sync.

Rows match on the Stripe *product* id rather than the price id. Stripe
prices are immutable, so editing an amount mints a new one, and matching
on that would duplicate the item on every price change. **Photos are
deliberately not synced** — they stay local under `uploads/products/`,
set per item at `/admin/products/<id>`, and the upsert names the columns
it writes so a sync can't blank an image an admin uploaded or lose the
catalogue's ordering. An item Stripe stops listing is marked sold out
rather than deleted, so its photo and position survive a relisting. A row
with no Stripe product id is local-only and sync never touches it; the
list marks which of the two each row is.

### Checkout

With `STRIPE_SECRET_KEY` set, the shop takes payment through Stripe
Checkout. Unset, all of it stays dormant and the shop is a catalogue
linking out to shop.decay.events, which is what it was before.

Stripe returns the buyer to `/order/confirm`, showing what they bought
and their **order code** — a short reference to quote if they get in
touch, not something redeemed at a door. The link carries the order's own
secure token, not Stripe's checkout session id, since the token is what
identifies an order here. A token matching nothing is a 404 rather than a
403, on the same reasoning as hidden master accounts: "forbidden" would
confirm the order exists.

That page is routinely reached *before* the payment is confirmed — the
browser is redirected the instant Stripe takes the money, and the webhook
confirming it is a separate call landing a moment later. An unconfirmed
order therefore renders as pending and polls `/api/order-status` until it
changes, then reloads to draw the paid state, so only one piece of code
knows what a paid order looks like. Without JavaScript the page still
shows the order and says it's pending; a refresh does the same job.

The webhook marks the order paid, assigns the code, and emails the buyer
a receipt carrying it, using the same SMTP settings as contact
notifications. Unconfigured mail just means no receipt — the confirmation
page has the same details either way. Two things it's careful about:
Stripe redelivers events it didn't get a clean response to, so an order
already paid and coded is left alone rather than issued a second code the
buyer has never seen; and a failed email never fails the webhook, since
the money has already moved and an error would have Stripe retry a
completed sale.

### Legacy import

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

### Staff calendar

The public feed above is the site publishing *out* to Nextcloud. The
`/admin/staff` page does the reverse for DECAY's own business: it
subscribes *in* to a separate, internal Nextcloud calendar (board and
organising meetings) and shows it on a month grid with an upcoming list.
It's the same arrangement — one-way, no stored credentials — just
inverted: point `STAFF_ICS_URL` at that calendar's read-only `.ics`
share link and it's read live (cached a few minutes) on each view.
Nothing is ever written back, and leaving the variable unset simply hides
the page. Meetings stay edited in Nextcloud; this is only a window onto
them, gated on a `staff` permission.

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

## Media

`/media` is videos and photographs together. It was `/photos` until it
grew videos; that URL now redirects, since old links and printed material
still point at it.

**Photos** are uploaded at `/admin/photos`. Files live under
`uploads/photos/` with web-sized copies in `uploads/photos/web/`, the
same arrangement as flyers and product shots — the page shows the copy
and links the original behind it. Tiles are cropped square so a grid of
mixed phone aspect ratios still reads as a grid; the uncropped original
is one click away.

Captions are optional and double as the image's alt text. Without one the
alt is deliberately empty, which marks the image as decorative rather
than reading a generated filename out to a screen reader.

**Videos** come from two places. *Featured* are the ones entered at
`/admin/media` — the same handful the home page carries — and they get
real embedded players. *Recent uploads* are read live from the channel's
public Atom feed, and are thumbnails linking out to YouTube rather than
players: a dozen embeds would load a YouTube frame each before anyone
asked to watch anything. Anything already featured is dropped from the
recent list so it isn't shown twice.

The feed needs no API key and no credentials, the same read-only
arrangement as the staff calendar. `YOUTUBE_CHANNEL` takes either a
handle (`@no_tape`) or a channel id (`UC…`) and defaults to DECAY's own
channel, so it works unconfigured; set it to empty to drop the section
entirely. A handle costs one extra fetch of the channel page to resolve,
which happens once per process.

Two things worth knowing if this ever misbehaves. Resolving a handle
reads the channel id out of the page's canonical `/channel/UC…` link
specifically — a channel page is full of other id-shaped strings, and
taking the first one resolves to a stranger's channel whose feed then
404s. And the endpoint intermittently answers 404 or 500 to a perfectly
good request, so a fetch is retried once and the results are cached for
an hour; a failed refresh keeps serving the last good list. The section
is a bonus on top of the database, so YouTube being down costs the page
that section, never the page.

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

Upcoming event pages carry a public **sign-up form** — name, a way to
reach them, an optional role, and a note. Offers land in a separate
`volunteer_signups` table and surface on the admin event page, never on
the public one. This is the one place volunteer contact details are
collected on purpose, and they're kept apart from `event_volunteers`,
which still holds names only; a honeypot field drops bots.

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
- `staff/` — reads DECAY's internal Nextcloud calendar for `/admin/staff`
- `youtube/` — reads recent uploads off a channel's public feed for `/media`
- `markdown/` — renders blog post Markdown to HTML
- `embed/` — resolves YouTube/Bandcamp links in posts into players
- `mail/` — best-effort SMTP notification for contact-form messages
- `images/` — makes web-sized copies of uploaded flyers
- `views/` — templ page templates (`.templ` source + generated `_templ.go`)
- `admin/` — session auth, permission checks, and CRUD handlers for `/admin/*`
- `db/roles.go` — the permission set each role grants
- `static/css`, `static/js`, `static/img` — assets, embedded into the binary at build time
- `uploads/` — photo, flyer, and avatar uploads written at runtime (not embedded, not committed)

Editing a `.templ` file requires regenerating its Go code:

```bash
go install github.com/a-h/templ/cmd/templ@latest
templ generate
```
