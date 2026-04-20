# Gormes.ai Landing Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:executing-plans` or `superpowers:subagent-driven-development` to work this plan task-by-task.

**Goal:** Ship and document the Phase 1.5 public landing page for `www.gormes.ai` using the already-implemented Go server in `gormes/www.gormes.ai/internal/site`.

**Scope:** Docs only for this task. The page itself is rendered by Go templates and embedded static assets under `gormes/www.gormes.ai/internal/site`, not by a separate JavaScript frontend.

**Reality check:** The landing page is server-rendered HTML, with templates and CSS embedded via `//go:embed`. The content is driven by `internal/site/content.go`, the HTTP surface is `GET /` plus `GET /static/*`, and the smoke test path is Playwright against the running Go server.

## Architecture

- `gormes/www.gormes.ai/internal/site/assets.go` owns the embedded filesystem.
- `gormes/www.gormes.ai/internal/site/templates/*.tmpl` and `gormes/www.gormes.ai/internal/site/static/*` are part of the binary.
- `gormes/www.gormes.ai/internal/site/content.go` carries the landing-page copy and links.
- `gormes/www.gormes.ai/internal/site/server.go` wires the route surface.
- `gormes/www.gormes.ai/tests/home.spec.mjs` provides a browser smoke test for the public page.

## Work Items

- [ ] Keep the landing-page design spec and this plan doc in `gormes/docs` so Goldmark validation covers both.
- [ ] Document the actual Go asset layout in `gormes/www.gormes.ai/README.md`, including the `gormes/www.gormes.ai/internal/site` location and `//go:embed` usage.
- [ ] Document the local verification flow in `gormes/www.gormes.ai/README.md`:
  - `go test ./...`
  - `npm run test:e2e`
- [ ] Keep the page free of client-side framework requirements and note that the smoke test expects no runtime JavaScript.
- [ ] Preserve portable Markdown in this plan and the design doc so Goldmark and Hugo can render them without special extensions.

## Verification

- `cd gormes && go test ./docs`
- `cd gormes/www.gormes.ai && go test ./...`
- `cd gormes/www.gormes.ai && npm run test:e2e`

## Notes

- This plan intentionally reflects the current implementation instead of the earlier placeholder layout.
- The static assets now live under `gormes/www.gormes.ai/internal/site` specifically so the binary can embed them cleanly.
- Playwright smoke testing is part of the documented workflow, not an optional future addition.
