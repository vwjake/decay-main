# Decay Main

Site for DECAY, a community arts and technology space in Olympia, WA.
Built with Go, Echo, SQLite, [templ](https://templ.guide), and htmx —
see [MANIFESTO.md](MANIFESTO.md) for the stack philosophy.

## Run

```bash
ADMIN_PASSWORD=change-me go run main.go
```

Then visit http://localhost:8080. Copy `.env.example` to `.env` (or export
the vars yourself) for real use — `ADMIN_PASSWORD` is required, and
`SESSION_SECRET` should be set so admin logins survive a restart.

The admin panel lives at `/admin/login` (username `admin` unless
`ADMIN_USERNAME` overrides it) and manages events, shop products, blog
posts, and photos — all stored in `decay.db` (SQLite, created and seeded
automatically on first run).

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
