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

## Structure

- `main.go` — server startup, routes, env config
- `db/` — SQLite schema, queries, and seed data
- `views/` — templ page templates (`.templ` source + generated `_templ.go`)
- `admin/` — session auth and CRUD handlers for `/admin/*`
- `static/css`, `static/js`, `static/img` — assets, embedded into the binary at build time
- `uploads/` — photo uploads written at runtime (not embedded, not committed)

Editing a `.templ` file requires regenerating its Go code:

```bash
go install github.com/a-h/templ/cmd/templ@latest
templ generate
```
