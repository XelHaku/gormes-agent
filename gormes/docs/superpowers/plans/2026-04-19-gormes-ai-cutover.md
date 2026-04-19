# Gormes.ai Hard Cutover Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the public website module and its active validation surfaces from `www.gormes.io` to `www.gormes.ai`, while preserving the Go-rendered landing page and keeping Node/Playwright isolated to the website module.

**Architecture:** Execute the rename in a dedicated worktree with TDD discipline. First, update the active Go/docs/Playwright tests so they demand the `.ai` identity and fail against the old `.io` implementation. Then rename the website module directory and module path, repair public strings and the site-local README, and finally update the active landing-page docs/test surface so the renamed implementation and validation docs agree.

**Tech Stack:** Go 1.22+, stdlib `net/http`, `html/template`, `embed`, Playwright via `@playwright/test` inside the website module only, Goldmark docs tests in `gormes/docs`, plain CSS, shell `mv`.

---

## File Structure

Work from a clean dedicated worktree. The current main worktree already has unrelated edits in `README.md`, `gormes/README.md`, `gormes/docs/ARCH_PLAN.md`, and `gormes/docs/THEORETICAL_ADVANTAGES_GORMES_HERMES.md`; do not fold those into this rename.

- Rename: `www.gormes.io/` → `www.gormes.ai/`
- Modify after rename: `www.gormes.ai/go.mod`
- Modify after rename: `www.gormes.ai/cmd/www-gormes/main.go`
- Modify after rename: `www.gormes.ai/README.md`
- Modify after rename: `www.gormes.ai/package.json`
- Modify after rename: `www.gormes.ai/internal/site/content.go`
- Modify after rename: `www.gormes.ai/internal/site/render_test.go`
- Modify after rename: `www.gormes.ai/tests/home.spec.mjs`
- Modify: `gormes/docs/docs_test.go`
- Modify: `gormes/docs/landing_page_docs_test.go`
- Modify: `gormes/docs/superpowers/specs/2026-04-19-gormes-landing-page-design.md`
- Modify: `gormes/docs/superpowers/plans/2026-04-19-gormes-landing-page.md`
- Reference only: `gormes/docs/superpowers/specs/2026-04-19-gormes-ai-cutover-design.md`
- Reference only: `gormes/docs/superpowers/plans/2026-04-19-gormes-ai-cutover.md`

The rename stays scoped to the website module and the active landing-page validation docs. Do not do a repository-wide `.io` rewrite of older historical documents.

### Task 1: Make the Active Tests Demand `www.gormes.ai`

**Files:**
- Modify: `www.gormes.io/internal/site/render_test.go`
- Modify: `www.gormes.io/tests/home.spec.mjs`
- Modify: `gormes/docs/landing_page_docs_test.go`

- [ ] **Step 1: Write the failing Go/site and docs tests**

Update `www.gormes.io/internal/site/render_test.go` so the rendered HTML must contain the `.ai` title:

```go
func TestServer_RendersApprovedPhase1Story(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	wants := []string{
		"Gormes.ai | The Agent That GOes With You.",
		"The Agent That GOes With You.",
		"Open Source • MIT License • Phase 1 Go Port",
		"API_SERVER_ENABLED=true hermes gateway start",
		"./bin/gormes",
		"Phase 1 uses your existing Hermes backend.",
		"The Port Is Already Moving",
		"Help Finish the Port",
		"Same agent. Same memory. Same workflows.",
	}

	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page missing %q\nbody:\n%s", want, body)
		}
	}
}
```

Update `www.gormes.io/tests/home.spec.mjs` so the browser smoke test expects the `.ai` title:

```js
import { test, expect } from '@playwright/test';

test('homepage renders the phase-1 story', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('Gormes.ai | The Agent That GOes With You.');
  await expect(page.getByRole('heading', { name: 'The Agent That GOes With You.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Quick Start' })).toBeVisible();
  await expect(page.getByText('Phase 1 uses your existing Hermes backend.')).toBeVisible();
  await expect(page.locator('.hero')).toHaveCount(1);
  await expect(page.locator('.hero-cta-row')).toHaveCount(1);
  await expect(page.locator('link[href="/static/site.css"]')).toHaveCount(1);
  await expect(page.locator('script')).toHaveCount(0);
});
```

Update `gormes/docs/landing_page_docs_test.go` so the active docs surface expects the `.ai` directory/module identity and the new cutover spec/plan to be present:

```go
func TestTargetsIncludeAICutoverDocs(t *testing.T) {
	want := map[string]bool{
		"superpowers/specs/2026-04-19-gormes-ai-cutover-design.md": false,
		"superpowers/plans/2026-04-19-gormes-ai-cutover.md":        false,
	}

	for _, target := range targets {
		if _, ok := want[target]; ok {
			want[target] = true
		}
	}

	for rel, seen := range want {
		if !seen {
			t.Fatalf("docs target missing %s", rel)
		}
	}
}

func TestLandingPagePlanDocReferencesRealImplementationFilesAndCommands(t *testing.T) {
	raw := readDoc(t, "superpowers/plans/2026-04-19-gormes-landing-page.md")
	wants := []string{
		"www.gormes.ai/internal/site/assets.go",
		"www.gormes.ai/internal/site/content.go",
		"www.gormes.ai/internal/site/server.go",
		"www.gormes.ai/internal/site/templates/*.tmpl",
		"www.gormes.ai/internal/site/static/*",
		"www.gormes.ai/tests/home.spec.mjs",
		"cd gormes && go test ./docs",
		"npm run test:e2e",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("plan doc is missing %q", want)
		}
	}

	for _, rel := range []string{
		"../../www.gormes.ai/internal/site/assets.go",
		"../../www.gormes.ai/internal/site/content.go",
		"../../www.gormes.ai/internal/site/server.go",
		"../../www.gormes.ai/internal/site/templates/index.tmpl",
		"../../www.gormes.ai/internal/site/templates/layout.tmpl",
		"../../www.gormes.ai/internal/site/templates/partials/code_block.tmpl",
		"../../www.gormes.ai/internal/site/templates/partials/feature_card.tmpl",
		"../../www.gormes.ai/internal/site/templates/partials/phase_item.tmpl",
		"../../www.gormes.ai/internal/site/static/site.css",
		"../../www.gormes.ai/tests/home.spec.mjs",
	} {
		if _, err := os.Stat(filepath.Join(".", rel)); err != nil {
			t.Fatalf("expected implementation file %s to exist: %v", rel, err)
		}
	}

	pkgJSON := readDoc(t, "../../www.gormes.ai/package.json")
	if !strings.Contains(pkgJSON, `"test:e2e": "playwright test --project=chromium"`) {
		t.Fatalf("www.gormes.ai package.json does not define the documented test:e2e script")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail on the old `.io` identity**

Run:

```bash
cd www.gormes.io && go test ./internal/site -run TestServer_RendersApprovedPhase1Story
cd www.gormes.io && npm install
cd www.gormes.io && npm run test:e2e
cd ../gormes && go test ./docs -run 'TestTargetsIncludeAICutoverDocs|TestLandingPagePlanDocReferencesRealImplementationFilesAndCommands'
```

Expected:

- the Go site test fails because the page still renders `Gormes.io | The Agent That GOes With You.`
- the Playwright smoke fails on the title assertion for `.ai`
- the docs test fails because `targets` does not yet include the cutover spec/plan and `../../www.gormes.ai/...` does not exist

- [ ] **Step 3: Commit the failing-test baseline**

```bash
git add www.gormes.io/internal/site/render_test.go www.gormes.io/tests/home.spec.mjs gormes/docs/landing_page_docs_test.go
git commit -m "test(gormes-www): require gormes.ai identity"
```

### Task 2: Rename the Website Module and Make the Site Tests Pass

**Files:**
- Rename: `www.gormes.io/` → `www.gormes.ai/`
- Modify: `www.gormes.ai/go.mod`
- Modify: `www.gormes.ai/cmd/www-gormes/main.go`
- Modify: `www.gormes.ai/internal/site/content.go`
- Modify: `www.gormes.ai/README.md`
- Modify: `www.gormes.ai/package.json`

- [ ] **Step 1: Rename the website module directory**

Run:

```bash
mv www.gormes.io www.gormes.ai
```

Verify:

```bash
test -d www.gormes.ai && test ! -e www.gormes.io
```

Expected: exit code `0`

- [ ] **Step 2: Update the Go module path**

Replace `www.gormes.ai/go.mod` with:

```go
module github.com/XelHaku/golang-hermes-agent/www.gormes.ai

go 1.22

toolchain go1.26.1
```

- [ ] **Step 3: Update the entrypoint import path and log line**

Replace `www.gormes.ai/cmd/www-gormes/main.go` with:

```go
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/XelHaku/golang-hermes-agent/www.gormes.ai/internal/site"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	handler, err := site.NewServer()
	if err != nil {
		slog.Error("build server", "err", err)
		os.Exit(1)
	}

	slog.Info("www.gormes.ai listening", "addr", *listen)
	if err := http.ListenAndServe(*listen, handler); err != nil {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Update the landing-page title string**

In `www.gormes.ai/internal/site/content.go`, replace the `Title` field value with:

```go
Title: "Gormes.ai | The Agent That GOes With You.",
```

Leave the rest of the landing-page copy intact unless it explicitly references `.io`.

- [ ] **Step 5: Update the site-local README and package identity**

Replace `www.gormes.ai/README.md` with:

````markdown
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
````

In `www.gormes.ai/package.json`, change the package name to:

```json
{
  "name": "www-gormes-ai",
  "private": true,
  "type": "module",
  "scripts": {
    "test:e2e": "playwright test --project=chromium"
  },
  "devDependencies": {
    "@playwright/test": "1.58.2"
  }
}
```

- [ ] **Step 6: Run the renamed module tests to verify they pass**

Run:

```bash
cd www.gormes.ai && npm install
cd www.gormes.ai && go test ./...
cd www.gormes.ai && npm run test:e2e
```

Expected: PASS

- [ ] **Step 7: Commit the website rename**

```bash
git add -A www.gormes.io www.gormes.ai
git commit -m "refactor(gormes-www): rename website module to gormes.ai"
```

### Task 3: Repair the Active Landing-Page Docs Surface for `.ai`

**Files:**
- Modify: `gormes/docs/docs_test.go`
- Modify: `gormes/docs/landing_page_docs_test.go`
- Modify: `gormes/docs/superpowers/specs/2026-04-19-gormes-landing-page-design.md`
- Modify: `gormes/docs/superpowers/plans/2026-04-19-gormes-landing-page.md`

- [ ] **Step 1: Add the cutover spec and plan to Goldmark coverage**

Replace the `targets` slice in `gormes/docs/docs_test.go` with:

```go
var targets = []string{
	"ARCH_PLAN.md",
	"THEORETICAL_ADVANTAGES_GORMES_HERMES.md",
	"superpowers/specs/2026-04-18-gormes-frontend-adapter-design.md",
	"superpowers/plans/2026-04-18-gormes-phase1-frontend-adapter.md",
	"superpowers/specs/2026-04-19-gormes-landing-page-design.md",
	"superpowers/plans/2026-04-19-gormes-landing-page.md",
	"superpowers/specs/2026-04-19-gormes-ai-cutover-design.md",
	"superpowers/plans/2026-04-19-gormes-ai-cutover.md",
}
```

- [ ] **Step 2: Update the landing-page docs tests to the renamed module**

Keep the `landing_page_docs_test.go` changes from Task 1 and add a second target check for the cutover docs if you did not already:

```go
func TestTargetsIncludeLandingPageDocs(t *testing.T) {
	want := map[string]bool{
		"superpowers/specs/2026-04-19-gormes-landing-page-design.md": false,
		"superpowers/plans/2026-04-19-gormes-landing-page.md":        false,
	}

	for _, target := range targets {
		if _, ok := want[target]; ok {
			want[target] = true
		}
	}

	for rel, seen := range want {
		if !seen {
			t.Fatalf("docs target missing %s", rel)
		}
	}
}
```

Do not weaken `TestLandingPagePlanDocReferencesRealImplementationFilesAndCommands`; it should continue checking the renamed filesystem paths and the `test:e2e` script in `../../www.gormes.ai/package.json`.

- [ ] **Step 3: Update the active landing-page design and plan docs**

In `gormes/docs/superpowers/specs/2026-04-19-gormes-landing-page-design.md`, replace the old `.io` scope/purpose references with `.ai`:

```md
# Gormes.ai Landing Page Design Spec

**Scope:** A simple, high-performance public landing page for `www.gormes.ai`, built in Go and targeted at current Hermes users evaluating the Phase 1 Go port.
```

And:

```md
`www.gormes.ai` must explain Gormes in one screenful to the right audience:
```

In `gormes/docs/superpowers/plans/2026-04-19-gormes-landing-page.md`, replace the website-module references with `.ai`:

```md
# Gormes.ai Landing Page Implementation Plan

**Goal:** Ship and document the Phase 1.5 public landing page for `www.gormes.ai` using the already-implemented Go server in `www.gormes.ai/internal/site`.
```

```md
**Scope:** Docs only for this task. The page itself is rendered by Go templates and embedded static assets under `www.gormes.ai/internal/site`, not by a separate JavaScript frontend.
```

And in the architecture/work-item/verification bullets, update:

- `www.gormes.io/internal/site/...` -> `www.gormes.ai/internal/site/...`
- `www.gormes.io/tests/home.spec.mjs` -> `www.gormes.ai/tests/home.spec.mjs`
- `www.gormes.io/README.md` -> `www.gormes.ai/README.md`
- `cd www.gormes.io ...` -> `cd www.gormes.ai ...`

- [ ] **Step 4: Run the docs validation to verify it passes**

Run:

```bash
cd gormes && go test ./docs
```

Expected: PASS

- [ ] **Step 5: Commit the docs cutover**

```bash
git add gormes/docs/docs_test.go gormes/docs/landing_page_docs_test.go gormes/docs/superpowers/specs/2026-04-19-gormes-landing-page-design.md gormes/docs/superpowers/plans/2026-04-19-gormes-landing-page.md
git commit -m "docs(gormes): rename landing page docs to gormes.ai"
```

## Final Verification

Run the complete verification sequence from the repository root:

```bash
cd www.gormes.ai && go test ./...
cd ../gormes && go test ./docs
cd ../www.gormes.ai && make build
cd ../www.gormes.ai && npm run test:e2e
```

Then run the stale-reference audit on the active rename surface:

```bash
rg -n "www\\.gormes\\.io|gormes\\.io" \
  www.gormes.ai \
  gormes/docs/landing_page_docs_test.go \
  gormes/docs/docs_test.go \
  gormes/docs/superpowers/specs/2026-04-19-gormes-landing-page-design.md \
  gormes/docs/superpowers/specs/2026-04-19-gormes-ai-cutover-design.md \
  gormes/docs/superpowers/plans/2026-04-19-gormes-landing-page.md \
  gormes/docs/superpowers/plans/2026-04-19-gormes-ai-cutover.md
```

Expected:

- verification commands all pass
- `rg` returns no stale `.io` references inside the active website module or the active landing-page validation docs

## Spec Coverage Checklist

- Hard cutover: Task 2 renames the folder and module path, and Task 3 updates the active docs/tests to the new identity.
- Canonical identity: Task 2 changes visible site strings and package identity to `.ai`.
- README requirement: Task 2 replaces the site-local README with explicit build/run/test instructions.
- Dependency boundary: Task 2 keeps Node tooling inside `www.gormes.ai` only, and no task touches `gormes/` runtime code for Node integration.
- Active docs surface: Task 3 updates the landing-page design/plan docs plus Goldmark coverage for the new `.ai` cutover spec and plan.
- TDD: Task 1 makes the tests fail on `.ai` expectations before the rename work is implemented.
