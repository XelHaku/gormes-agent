# Gormes.io Landing Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:executing-plans` or `superpowers:subagent-driven-development` to work this plan task-by-task.

**Goal:** Ship and document the Phase 1.5 public landing page for `www.gormes.io` using the already-implemented Go server in `www.gormes.io/internal/site`.

**Scope:** Docs only for this task. The page itself is rendered by Go templates and embedded static assets under `www.gormes.io/internal/site`, not by a separate JavaScript frontend.

**Reality check:** The landing page is server-rendered HTML, with templates and CSS embedded via `//go:embed`. The content is driven by `internal/site/content.go`, the HTTP surface is `GET /` plus `GET /static/*`, and the smoke test path is Playwright against the running Go server.

## Architecture

- `www.gormes.io/internal/site/assets.go` owns the embedded filesystem.
- `www.gormes.io/internal/site/templates/*.tmpl` and `www.gormes.io/internal/site/static/*` are part of the binary.
- `www.gormes.io/internal/site/content.go` carries the landing-page copy and links.
- `www.gormes.io/internal/site/server.go` wires the route surface.
- `www.gormes.io/tests/home.spec.mjs` provides a browser smoke test for the public page.

## Work Items

- [ ] Keep the landing-page design spec and this plan doc in `gormes/docs` so Goldmark validation covers both.
- [ ] Document the actual Go asset layout in `www.gormes.io/README.md`, including the `www.gormes.io/internal/site` location and `//go:embed` usage.
- [ ] Document the local verification flow in `www.gormes.io/README.md`:
  - `go test ./...`
  - `npm run test:e2e`
- [ ] Keep the page free of client-side framework requirements and note that the smoke test expects no runtime JavaScript.
- [ ] Preserve portable Markdown in this plan and the design doc so Goldmark and Hugo can render them without special extensions.

## Verification

- `cd gormes && go test ./docs -run 'TestTargetsIncludeLandingPageDocs|TestMarkdownRendersCleanViaGoldmark|TestMarkdownAvoidsPortabilityHazards'`
- `cd www.gormes.io && go test ./...`
- `cd www.gormes.io && npm run test:e2e`

## Notes

- This plan intentionally reflects the current implementation instead of the earlier placeholder layout.
- The static assets now live under `www.gormes.io/internal/site` specifically so the binary can embed them cleanly.
- Playwright smoke testing is part of the documented workflow, not an optional future addition.
