# Decay Main

Site for DECAY, a Brooklyn studio/venue/nonprofit. Built with Go and [Echo](https://echo.labstack.com/).

## Run

```bash
go run main.go
```

Then visit http://localhost:8080

## Structure

- `main.go` — server, routes, event/merch data
- `templates/index.html` — page template
- `static/css`, `static/js`, `static/img` — assets, embedded into the binary at build time
