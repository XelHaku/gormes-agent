# Gormes.ai

Server-rendered landing page for the Gormes Phase 1 Go port.

The site is built in Go and serves the public homepage at `/` plus embedded static assets at `/static/*`. The implementation lives under `www.gormes.ai/internal/site` so the templates and CSS can be embedded with `//go:embed`.

## Layout

- `cmd/www-gormes` - HTTP entrypoint.
- `internal/site/content.go` - landing-page copy and link data.
- `internal/site/server.go` - route wiring and template execution.
- `internal/site/templates/*.tmpl` - HTML templates.
- `internal/site/static/*` - embedded CSS and other static assets.
- `tests/home.spec.mjs` - Playwright smoke test for the homepage.

## Local Development

```bash
cd www.gormes.ai
make build
./bin/www-gormes -listen :8080
```

Or run the server directly:

```bash
go run ./cmd/www-gormes -listen :8080
```

`make run` uses the same command.

## Verification

Run the Go test suite:

```bash
go test ./...
```

Run the browser smoke test:

```bash
npm install
npm run test:e2e
```

The Playwright config launches the Go server with `go run ./cmd/www-gormes -listen :8080`, so no separate app process is needed for the smoke test.

## Content Updates

- Edit `internal/site/content.go` to change copy, CTAs, or roadmap text.
- Edit `internal/site/templates/*.tmpl` to change structure.
- Edit `internal/site/static/site.css` to change presentation.

The page intentionally avoids client-side JavaScript. The homepage should remain readable and useful with scripts disabled.
