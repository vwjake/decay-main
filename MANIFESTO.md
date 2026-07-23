# The DECAY stack manifesto

DECAY is a volunteer-run nonprofit. The site that runs it should be boring
to operate, cheap to host, and legible to whoever picks it up next —
which is why it's Go, all the way down.

## Principles

**One binary.** Templates, CSS, JS, and images are `go:embed`-ed into the
compiled binary. Deploying is copying one file and restarting it — no
`node_modules`, no build pipeline, no PHP + Apache + MySQL stack to keep
patched.

**One database file.** SQLite, not a managed Postgres instance. Backups
are `cp data.db data.db.bak`. There's no ORM between us and the schema —
plain SQL, because the queries here are simple and an ORM would just be
another thing to learn.

**No JS framework.** htmx handles interactivity by asking the server for
HTML fragments instead of shipping a client-side app. There's no build
step, no bundler, no npm supply chain to audit.

**No CMS lock-in.** We're not adopting a Ponzu or a Wagtail. The admin
panel is Echo routes and templ forms, tailored to exactly what DECAY
needs (events, posts, photos, products) and nothing it doesn't.

**Payments are the one exception.** Checkout, PCI compliance, and fraud
handling are not worth reinventing for a venue site. That piece is
someone else's problem, on purpose.

## The stack

| Concern | Tool | Why |
|---|---|---|
| Routing / middleware | [Echo](https://echo.labstack.com/) | Already in place, thin, unopinionated |
| Templates | [templ](https://templ.guide) | Type-safe, compiles to Go, no runtime parsing |
| Interactivity | [htmx](https://htmx.org) | Server-rendered fragments, no JS framework |
| Storage | SQLite (`modernc.org/sqlite`) | Pure Go, no cgo, one file |
| Blog content | [goldmark](https://github.com/yuin/goldmark) | Markdown posts, no CMS needed |
| Photos | [disintegration/imaging](https://github.com/disintegration/imaging) | Pure Go thumbnailing |
| Calendar | [rrule-go](https://github.com/teambition/rrule-go) + [golang-ical](https://github.com/arran4/golang-ical) | Recurring events, `.ics` subscription feed |
| Shop / checkout | Stripe Checkout or Snipcart | Embedded widget + a webhook; not hand-rolled |

## What this replaces

The old PHP site (`decay`) does the right things — flat data files,
a hand-rolled admin panel, no framework bloat — just in PHP. This is
the same philosophy, ported to a language and toolchain we'd rather
maintain.
