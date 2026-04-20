# Gormes.ai

Server-rendered landing page for current Gormes trunk.

The site should reflect the shipped moat layers truthfully: the 7.9 MB zero-CGO TUI, the Go-native tool registry, the split-binary Telegram Scout, Route-B resilience, and Phase 2.C's thin bbolt resume layer. It should not regress into a Phase-1-only story.

The site is built in Go and serves the public homepage at `/` plus embedded static assets at `/static/*`. In this monorepo, the implementation lives under `gormes/www.gormes.ai/internal/site` so the templates and CSS can be embedded with `//go:embed`.

## Layout

- `cmd/www-gormes` - HTTP entrypoint.
- `internal/site/content.go` - landing-page copy and link data.
- `internal/site/server.go` - route wiring and template execution.
- `internal/site/templates/*.tmpl` - HTML templates.
- `internal/site/static/*` - embedded CSS and other static assets.
- `tests/home.spec.mjs` - Playwright smoke test for the homepage.

## Local Development

```bash
cd gormes/www.gormes.ai
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

Install the browser-test dependency once per checkout:

```bash
npm install
```

Run the browser smoke test:

```bash
npm run test:e2e
```

The Playwright config launches the Go server with `go run ./cmd/www-gormes -listen :8080`, so no separate app process is needed for the smoke test.

## Content Updates

- Edit `internal/site/content.go` to change copy, CTAs, or roadmap text.
- Edit `internal/site/templates/*.tmpl` to change structure.
- Edit `internal/site/static/site.css` to change presentation.

The page intentionally avoids client-side JavaScript. The homepage should remain readable and useful with scripts disabled.
