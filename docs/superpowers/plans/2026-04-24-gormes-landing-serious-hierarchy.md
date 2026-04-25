# Gormes Landing Serious Hierarchy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply the approved serious-infra landing reset to `www.gormes.ai`.

**Architecture:** The landing remains Go-rendered with embedded templates and static CSS. Content changes live in `internal/site/content.go`; structure changes live in `templates/index.tmpl` and partials; visual hierarchy lives in `static/site.css`; render/export/Playwright tests protect the behavior.

**Tech Stack:** Go `html/template`, embedded assets, vanilla CSS, Playwright.

---

### Task 1: Lock Expected Landing Behavior

**Files:**
- Modify: `www.gormes.ai/internal/site/render_test.go`
- Modify: `www.gormes.ai/internal/site/static_export_test.go`
- Modify: `www.gormes.ai/tests/home.spec.mjs`

- [ ] **Step 1: Update render/export tests first**

Assert the trimmed nav, serious hero text, no hero image, install command set, feature pain block, roadmap focus block, and stale-copy rejects.

- [ ] **Step 2: Run tests and verify red**

Run:

```bash
cd www.gormes.ai && go test ./internal/site -run 'TestRenderIndex_RendersRedesignedLanding|TestExportDir_WritesStaticSite' -count=1
```

Expected: FAIL because the current page still has the gopher hero, old typography assumptions, and no pain/roadmap summary blocks.

### Task 2: Update Content And Templates

**Files:**
- Modify: `www.gormes.ai/internal/site/content.go`
- Modify: `www.gormes.ai/internal/site/templates/layout.tmpl`
- Modify: `www.gormes.ai/internal/site/templates/index.tmpl`
- Modify: `www.gormes.ai/internal/site/templates/partials/roadmap_phase.tmpl`

- [ ] **Step 1: Remove hero image data and trim nav**

Drop `HeroImage`, set nav to `Install`, `Roadmap`, `GitHub`, add hero note, feature pain bullets, install source note, docs footer link, and roadmap summary fields.

- [ ] **Step 2: Rebuild section structure**

Render hero as a single-column editorial block, add the hero note, install header/source callout, pain block before cards, and roadmap summary before generated phase groups.

- [ ] **Step 3: Collapse roadmap phase details on mobile**

Use native `<details>` for phase groups so small screens can scan status/title without absorbing every item.

### Task 3: Rewrite CSS Hierarchy

**Files:**
- Modify: `www.gormes.ai/internal/site/static/site.css`

- [ ] **Step 1: Apply the typography system**

Limit display serif to `.hero-title`; use DM Sans for section/card/roadmap text; keep JetBrains Mono for command/code/copy affordances.

- [ ] **Step 2: Tighten hierarchy and mobile layout**

Remove hero image layout, shrink paragraph line length, make `Install` dominant, make feature cards sharper, space install steps consistently, and collapse roadmap details under mobile widths.

### Task 4: Verify

**Files:**
- Test only

- [ ] **Step 1: Run Go tests**

```bash
cd www.gormes.ai && go test ./...
```

- [ ] **Step 2: Run Playwright tests**

```bash
cd www.gormes.ai && npm run test:e2e
```

- [ ] **Step 3: Check git diff**

```bash
git diff --stat
git status --short
```
