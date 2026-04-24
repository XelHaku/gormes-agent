# Gormes Operator Console Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign `gormes/www.gormes.ai` into an aggressive operator-console landing page that converts existing Hermes users into trying Gormes locally now, stays truthful about shipped scope, and works cleanly on mobile.

**Architecture:** Keep the site static-export friendly and Go-owned. Move the redesign through the existing `internal/site` content + template pipeline, preserve the shared server/export render path, and use TDD to lock new copy, structure, mobile behavior, and stale-claim removal before rewriting templates and CSS.

**Tech Stack:** Go, `html/template`, embedded static assets, plain CSS, Playwright, `go test`, `npm run test:e2e`.

---

## Prerequisites

- Work from `<repo>/gormes/www.gormes.ai` unless a step explicitly targets the repo root.
- Keep edits inside `gormes/` only.
- Re-verify binary-size claims from a fresh local build before hardcoding any number into page copy.
- Treat `7.9 MB` as stale and forbidden unless a fresh build somehow returns to that value.

## File Structure Map

```
gormes/
├── docs/superpowers/
│   ├── specs/
│   │   └── 2026-04-20-gormes-operator-console-redesign-design.md   # SPEC REFERENCE
│   └── plans/
│       └── 2026-04-20-gormes-operator-console-redesign.md          # THIS FILE
└── www.gormes.ai/
    ├── internal/site/
    │   ├── content.go                                              # MODIFY — truthful operator-console data model + copy
    │   ├── render_test.go                                          # MODIFY — render truth + template coverage
    │   ├── static_export_test.go                                   # MODIFY — exported HTML truth coverage
    │   ├── static/site.css                                         # MODIFY — operator-console visual system + mobile layout
    │   └── templates/
    │       ├── layout.tmpl                                         # MODIFY — control-surface shell
    │       ├── index.tmpl                                          # MODIFY — hero/proof/run-now/ops/shipping/source flow
    │       └── partials/
    │           ├── command_step.tmpl                               # NEW — activation sequence block
    │           ├── ops_module.tmpl                                 # NEW — systems-advantage module
    │           ├── proof_stat.tmpl                                 # NEW — proof/status tile
    │           ├── ship_state.tmpl                                 # NEW — shipping-ledger row
    │           ├── code_block.tmpl                                 # DELETE — replaced by command_step
    │           ├── feature_card.tmpl                               # DELETE — replaced by ops_module
    │           └── phase_item.tmpl                                 # DELETE — replaced by ship_state
    └── tests/
        └── home.spec.mjs                                           # MODIFY — desktop + mobile smoke coverage
```

---

### Task 1: Lock current-truth operator copy before redesigning layout

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/content.go`
- Modify: `gormes/www.gormes.ai/internal/site/render_test.go`
- Test: `gormes/www.gormes.ai/internal/site/render_test.go`

- [ ] **Step 1: Write the failing render test for new hero truth**

Replace the current render test body with:

```go
func TestRenderIndex_RendersOperatorConsoleTruth(t *testing.T) {
	body, err := RenderIndex()
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}

	text := string(body)
	wants := []string{
		"Run Hermes Through a Go Operator Console.",
		"Boot Gormes",
		"Current boundary: the Go shell ships now. Transcript memory stays on the later cutover path.",
		"Go Shell Shipping Now",
	}
	rejects := []string{
		"7.9 MB Static Binary",
		"7.9 MB zero-CGO TUI",
	}

	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered page missing %q\nbody:\n%s", want, text)
		}
	}

	for _, reject := range rejects {
		if strings.Contains(text, reject) {
			t.Fatalf("rendered page still contains stale claim %q\nbody:\n%s", reject, text)
		}
	}
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run:

```bash
cd <repo>/gormes/www.gormes.ai
go test ./internal/site -run TestRenderIndex_RendersOperatorConsoleTruth
```

Expected:

```text
... rendered page missing "Run Hermes Through a Go Operator Console."
... rendered page still contains stale claim "7.9 MB Static Binary"
FAIL
```

- [ ] **Step 3: Write the minimal truthful content update**

First refresh the binary size reference:

```bash
cd <repo>/gormes
make build
ls -lh bin
```

Use the fresh result to update `gormes/www.gormes.ai/internal/site/content.go`. The minimal code change should be:

```go
Title:       "Gormes.ai | Run Hermes Through a Go Operator Console",
Description: "Gormes is the Go operator shell for Hermes users: zero-CGO, Go-native tools, split Telegram edge, and honest shipping state.",
Nav: []NavLink{
	{Label: "Run Now", Href: "#quickstart"},
	{Label: "Shipping State", Href: "#roadmap"},
	{Label: "Source", Href: "#contribute"},
	{Label: "GitHub", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
},
HeroBadge:    "Open Source • MIT License • Zero-CGO • Go Shell Shipping Now",
HeroHeadline: "Run Hermes Through a Go Operator Console.",
HeroCopy: []string{
	"Stop waiting for the clean-room rewrite. Gormes already ships a Go shell, a Go-native tool loop, Route-B resilience, and a split Telegram edge.",
	"Boot it locally. Judge the surface yourself. Keep the promises honest.",
},
PrimaryCTA:   Link{Label: "Boot Gormes", Href: "#quickstart"},
SecondaryCTA: Link{Label: "See Shipping State", Href: "#roadmap"},
TertiaryCTA:  Link{Label: "Inspect Source", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
PhaseNote:    "Current boundary: the Go shell ships now. Transcript memory stays on the later cutover path.",
```

- [ ] **Step 4: Run the focused test to verify it passes**

Run:

```bash
cd <repo>/gormes/www.gormes.ai
go test ./internal/site -run TestRenderIndex_RendersOperatorConsoleTruth
```

Expected:

```text
ok  	github.com/TrebuchetDynamics/gormes-agent/gormes/www.gormes.ai/internal/site
```

- [ ] **Step 5: Commit**

Run:

```bash
git add gormes/www.gormes.ai/internal/site/content.go gormes/www.gormes.ai/internal/site/render_test.go
git commit -m "copy(gormes/www): remove stale landing page claims"
```

---

### Task 2: Replace the stacked marketing layout with operator-console structure

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/content.go`
- Modify: `gormes/www.gormes.ai/internal/site/render_test.go`
- Modify: `gormes/www.gormes.ai/internal/site/static_export_test.go`
- Modify: `gormes/www.gormes.ai/internal/site/templates/layout.tmpl`
- Modify: `gormes/www.gormes.ai/internal/site/templates/index.tmpl`
- Add: `gormes/www.gormes.ai/internal/site/templates/partials/command_step.tmpl`
- Add: `gormes/www.gormes.ai/internal/site/templates/partials/ops_module.tmpl`
- Add: `gormes/www.gormes.ai/internal/site/templates/partials/proof_stat.tmpl`
- Add: `gormes/www.gormes.ai/internal/site/templates/partials/ship_state.tmpl`
- Delete: `gormes/www.gormes.ai/internal/site/templates/partials/code_block.tmpl`
- Delete: `gormes/www.gormes.ai/internal/site/templates/partials/feature_card.tmpl`
- Delete: `gormes/www.gormes.ai/internal/site/templates/partials/phase_item.tmpl`
- Test: `gormes/www.gormes.ai/internal/site/render_test.go`
- Test: `gormes/www.gormes.ai/internal/site/static_export_test.go`

- [ ] **Step 1: Write the failing structure tests**

Update `gormes/www.gormes.ai/internal/site/render_test.go` so the main render test asserts the new operator-console sections:

```go
wants := []string{
	"Run Hermes Through a Go Operator Console.",
	"Run the shell. Judge it yourself.",
	"Why Hermes users switch",
	"Shipping State, Not Wishcasting",
	"Inspect the Machine",
	"8.2M shell",
	"15M telegram edge",
}
```

Update the embedded-template test to require:

```go
files := []string{
	"templates/layout.tmpl",
	"templates/index.tmpl",
	"templates/partials/command_step.tmpl",
	"templates/partials/ops_module.tmpl",
	"templates/partials/proof_stat.tmpl",
	"templates/partials/ship_state.tmpl",
}

for _, want := range []string{"layout", "index", "command_step", "ops_module", "proof_stat", "ship_state"} {
	if templates.Lookup(want) == nil {
		t.Fatalf("parsed templates missing %q", want)
	}
}
```

Update `gormes/www.gormes.ai/internal/site/static_export_test.go` so the exported page also asserts:

```go
if !strings.Contains(string(body), "Shipping State, Not Wishcasting") {
	t.Fatalf("index.html missing shipping-ledger heading\n%s", string(body))
}
if strings.Contains(string(body), "7.9 MB") {
	t.Fatalf("index.html still contains stale size claim\n%s", string(body))
}
```

- [ ] **Step 2: Run the site tests to verify they fail**

Run:

```bash
cd <repo>/gormes/www.gormes.ai
go test ./internal/site
```

Expected:

```text
... embedded template "templates/partials/proof_stat.tmpl" missing
... rendered page missing "Shipping State, Not Wishcasting"
FAIL
```

- [ ] **Step 3: Implement the new data model and templates**

Expand `gormes/www.gormes.ai/internal/site/content.go` with explicit operator-console structures:

```go
type ProofStat struct {
	Label string
	Value string
	Tone  string
}

type CommandStep struct {
	Label string
	Note  string
	Lines []string
}

type OpsModule struct {
	Label string
	Title string
	Body  string
}

type ShipState struct {
	State string
	Name  string
	Body  string
}

type LandingPage struct {
	Title            string
	Description      string
	Nav              []NavLink
	HeroKicker       string
	HeroHeadline     string
	HeroCopy         []string
	HeroPanelTitle   string
	HeroPanelLines   []string
	PrimaryCTA       Link
	SecondaryCTA     Link
	TertiaryCTA      Link
	ScopeNote        string
	ProofStats       []ProofStat
	ActivationTitle  string
	ActivationIntro  string
	ActivationSteps  []CommandStep
	OpsTitle         string
	OpsIntro         string
	OpsModules       []OpsModule
	RoadmapTitle     string
	RoadmapIntro     string
	ShipStates       []ShipState
	ContributorTitle string
	ContributorBody  string
	ContributorLinks []Link
	FooterLinks      []Link
	FooterLine       string
}
```

Populate `DefaultPage()` with operator-console copy. Use the fresh measured sizes from Task 1 in these slots:

```go
HeroKicker:     "HERMES / GO OPERATOR SHELL",
HeroPanelTitle: "Boot Sequence",
HeroPanelLines: []string{
	"[ok] shell compiled",
	"[ok] tool loop armed",
	"[ok] route-b ready",
	"[warn] transcript memory still on later cutover path",
},
ProofStats: []ProofStat{
	{Label: "gormes shell", Value: "8.2M", Tone: "live"},
	{Label: "telegram edge", Value: "15M", Tone: "cold"},
	{Label: "deployment", Value: "Zero-CGO", Tone: "live"},
	{Label: "tool surface", Value: "Go-native", Tone: "cold"},
	{Label: "phase", Value: "2 ships now", Tone: "warn"},
},
ActivationTitle: "Run the shell. Judge it yourself.",
ActivationIntro: "If you already trust Hermes, this is the shortest honest path to feel the Go layer.",
ActivationSteps: []CommandStep{
	{
		Label: "01 / BACKEND",
		Note:  "Bring up the existing Hermes backend first.",
		Lines: []string{"API_SERVER_ENABLED=true hermes gateway start"},
	},
	{
		Label: "02 / BUILD",
		Note:  "Compile the Go surfaces with the current local toolchain.",
		Lines: []string{"cd gormes", "make build"},
	},
	{
		Label: "03 / VERIFY",
		Note:  "Run doctor, launch the shell, and probe the Telegram edge if needed.",
		Lines: []string{"./bin/gormes doctor --offline", "./bin/gormes", "GORMES_TELEGRAM_TOKEN=... GORMES_TELEGRAM_CHAT_ID=123456789 ./bin/gormes-telegram"},
	},
},
OpsTitle: "Why Hermes users switch",
OpsIntro: "Gormes is not a reskin. It is the hardened shell around the workflows you already trust.",
OpsModules: []OpsModule{
	{Label: "RESPONSIVENESS", Title: "Cut startup tax", Body: "Use the Go shell that boots like a tool, not a ceremony."},
	{Label: "TOOLS", Title: "Keep the loop typed", Body: "Run the Go-native tool surface in-process and verify it before you spend more tokens."},
	{Label: "ISOLATION", Title: "Split the blast radius", Body: "Keep Telegram and the shell in separate binaries so dependencies and failures stay local."},
	{Label: "HONESTY", Title: "Ship the boundary you have", Body: "The shell is real now. Transcript memory and the brain cutover are still later work."},
},
RoadmapTitle: "Shipping State, Not Wishcasting",
ShipStates: []ShipState{
	{State: "SHIPPED", Name: "Phase 1 — Dashboard", Body: "The Bubble Tea shell and operator surface are already real."},
	{State: "SHIPPED", Name: "Phase 2 — Gateway", Body: "Tool registry, Telegram scout, and thin session resume already live on trunk."},
	{State: "NEXT", Name: "Phase 3 — Memory", Body: "SQLite + FTS5 transcript memory still marks the real handoff line."},
	{State: "LATER", Name: "Phase 4 — Brain", Body: "Prompt building and native agent orchestration move after memory is real."},
},
ContributorTitle: "Inspect the Machine",
ContributorBody:  "Read the architecture, inspect the source, and verify the claims like an operator, not a spectator.",
FooterLine:       "Gormes already ships the operator shell. The memory lattice and brain cutover come later.",
```

Replace `gormes/www.gormes.ai/internal/site/templates/index.tmpl` with:

```gotemplate
{{define "index"}}
<section class="hero-deck" aria-labelledby="hero-title">
  <div class="hero-copy-stack">
    <p class="eyebrow">{{.HeroKicker}}</p>
    <h1 id="hero-title">{{.HeroHeadline}}</h1>
    {{range .HeroCopy}}
    <p class="hero-copy">{{.}}</p>
    {{end}}
    <div class="hero-actions">
      <a class="hero-cta hero-cta-primary" href="{{.PrimaryCTA.Href}}">{{.PrimaryCTA.Label}}</a>
      <a class="hero-cta hero-cta-secondary" href="{{.SecondaryCTA.Href}}">{{.SecondaryCTA.Label}}</a>
      <a class="hero-cta hero-cta-secondary" href="{{.TertiaryCTA.Href}}">{{.TertiaryCTA.Label}}</a>
    </div>
    <p class="scope-note">{{.ScopeNote}}</p>
  </div>
  <aside class="hero-panel" aria-label="{{.HeroPanelTitle}}">
    <p class="panel-label">{{.HeroPanelTitle}}</p>
    <pre class="panel-pre"><code>{{range .HeroPanelLines}}{{.}}
{{end}}</code></pre>
  </aside>
</section>

<section id="proof" class="proof-strip" aria-labelledby="proof-title">
  <h2 id="proof-title">Proof Rail</h2>
  {{range .ProofStats}}
    {{template "proof_stat" .}}
  {{end}}
</section>

<section id="quickstart" class="activation-grid" aria-labelledby="activation-title">
  <div class="activation-copy">
    <h2 id="activation-title">{{.ActivationTitle}}</h2>
    <p>{{.ActivationIntro}}</p>
  </div>
  <div class="activation-steps">
    {{range .ActivationSteps}}
      {{template "command_step" .}}
    {{end}}
  </div>
</section>

<section class="ops-section" aria-labelledby="ops-title">
  <h2 id="ops-title">{{.OpsTitle}}</h2>
  <p>{{.OpsIntro}}</p>
  <div class="ops-grid">
    {{range .OpsModules}}
      {{template "ops_module" .}}
    {{end}}
  </div>
</section>

<section id="roadmap" class="shipping-ledger" aria-labelledby="roadmap-title">
  <h2 id="roadmap-title">{{.RoadmapTitle}}</h2>
  <p>{{.RoadmapIntro}}</p>
  <div class="ship-state-list">
    {{range .ShipStates}}
      {{template "ship_state" .}}
    {{end}}
  </div>
</section>

<section id="contribute" class="source-block" aria-labelledby="contribute-title">
  <h2 id="contribute-title">{{.ContributorTitle}}</h2>
  <p>{{.ContributorBody}}</p>
  <ul>
    {{range .ContributorLinks}}
    <li><a href="{{.Href}}">{{.Label}}</a></li>
    {{end}}
  </ul>
</section>
{{end}}
```

Add the new partials exactly as follows:

`gormes/www.gormes.ai/internal/site/templates/partials/proof_stat.tmpl`

```gotemplate
{{define "proof_stat"}}
<article class="proof-stat proof-stat-{{.Tone}}">
  <p class="proof-label">{{.Label}}</p>
  <h3 class="proof-value">{{.Value}}</h3>
</article>
{{end}}
```

`gormes/www.gormes.ai/internal/site/templates/partials/command_step.tmpl`

```gotemplate
{{define "command_step"}}
<article class="command-step">
  <div class="command-step-header">
    <p class="eyebrow">{{.Label}}</p>
    <p class="command-note">{{.Note}}</p>
  </div>
  <pre class="command-pre"><code>{{range .Lines}}{{.}}
{{end}}</code></pre>
</article>
{{end}}
```

`gormes/www.gormes.ai/internal/site/templates/partials/ops_module.tmpl`

```gotemplate
{{define "ops_module"}}
<article class="ops-module">
  <p class="module-label">{{.Label}}</p>
  <h3>{{.Title}}</h3>
  <p>{{.Body}}</p>
</article>
{{end}}
```

`gormes/www.gormes.ai/internal/site/templates/partials/ship_state.tmpl`

```gotemplate
{{define "ship_state"}}
<article class="ship-state ship-state-{{.State}}">
  <p class="state-label">{{.State}}</p>
  <div>
    <h3>{{.Name}}</h3>
    <p>{{.Body}}</p>
  </div>
</article>
{{end}}
```

Adjust `gormes/www.gormes.ai/internal/site/templates/layout.tmpl` so the nav and footer labels match the new operator-console structure:

```gotemplate
<body>
  <div class="page-shell">
    <header id="top" class="site-header">
      <nav class="nav" aria-label="Primary">
        <a class="brand" href="#top">Gormes</a>
        <p class="nav-tag">operator console / hermes go shell</p>
        <div class="nav-links">
          {{range .Nav}}
          <a href="{{.Href}}">{{.Label}}</a>
          {{end}}
        </div>
      </nav>
    </header>
```

- [ ] **Step 4: Run the site tests to verify they pass**

Run:

```bash
cd <repo>/gormes/www.gormes.ai
go test ./internal/site
```

Expected:

```text
ok  	github.com/TrebuchetDynamics/gormes-agent/gormes/www.gormes.ai/internal/site
```

- [ ] **Step 5: Commit**

Run:

```bash
git add gormes/www.gormes.ai/internal/site/content.go \
        gormes/www.gormes.ai/internal/site/render_test.go \
        gormes/www.gormes.ai/internal/site/static_export_test.go \
        gormes/www.gormes.ai/internal/site/templates/layout.tmpl \
        gormes/www.gormes.ai/internal/site/templates/index.tmpl \
        gormes/www.gormes.ai/internal/site/templates/partials
git commit -m "feat(gormes/www): rebuild landing page as operator console"
```

---

### Task 3: Rewrite CSS for operator-console presentation and mobile survival

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/static/site.css`
- Modify: `gormes/www.gormes.ai/tests/home.spec.mjs`
- Test: `gormes/www.gormes.ai/tests/home.spec.mjs`

- [ ] **Step 1: Write the failing desktop + mobile browser smoke tests**

Replace `gormes/www.gormes.ai/tests/home.spec.mjs` with:

```javascript
import { test, expect } from '@playwright/test';

test('homepage renders the operator-console story', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('Gormes.ai | Run Hermes Through a Go Operator Console');
  await expect(page.getByRole('heading', { name: 'Run Hermes Through a Go Operator Console.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Run the shell. Judge it yourself.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Shipping State, Not Wishcasting' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Boot Gormes' })).toBeVisible();
  await expect(page.getByText('7.9 MB Static Binary')).toHaveCount(0);
  await expect(page.locator('link[href="/static/site.css"]')).toHaveCount(1);
  await expect(page.locator('script')).toHaveCount(0);
});

test('mobile keeps the run-now flow readable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');

  await expect(page.getByRole('link', { name: 'Boot Gormes' })).toBeVisible();
  await expect(page.getByText('8.2M')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Run the shell. Judge it yourself.' })).toBeVisible();

  const hasOverflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  expect(hasOverflow).toBeFalsy();
});
```

- [ ] **Step 2: Run the browser smoke test to verify it fails**

Run:

```bash
cd <repo>/gormes/www.gormes.ai
npm run test:e2e
```

Expected:

```text
... mobile keeps the run-now flow readable
... expected false received true
FAIL
```

- [ ] **Step 3: Write the minimal operator-console CSS**

Replace the existing soft-card CSS with an operator-console system. The core stylesheet should include these sections:

```css
:root {
  --bg-0: #06080b;
  --bg-1: #0d1218;
  --bg-2: #141b23;
  --steel: #1a232d;
  --line: rgba(163, 188, 214, 0.16);
  --line-strong: rgba(255, 191, 71, 0.4);
  --text: #edf3f8;
  --muted: #98a7b8;
  --amber: #ffbf47;
  --cyan: #6fd3ff;
  --green: #89f7a1;
  --danger: #ff7b72;
  --shadow: 0 24px 80px rgba(0, 0, 0, 0.42);
  --radius: 20px;
  --font-body: "IBM Plex Sans", "Segoe UI", sans-serif;
  --font-display: "Bahnschrift", "DIN Alternate", "Arial Narrow", sans-serif;
  --font-mono: "IBM Plex Mono", "SFMono-Regular", monospace;
}

body {
  margin: 0;
  color: var(--text);
  font-family: var(--font-body);
  background:
    radial-gradient(circle at top right, rgba(111, 211, 255, 0.12), transparent 28rem),
    radial-gradient(circle at top left, rgba(255, 191, 71, 0.1), transparent 24rem),
    linear-gradient(180deg, var(--bg-1) 0%, var(--bg-0) 100%);
}

.page-shell {
  width: min(1220px, calc(100% - 32px));
  margin: 0 auto;
  padding: 18px 0 42px;
  position: relative;
}

.page-shell::before {
  content: "";
  position: fixed;
  inset: 0;
  pointer-events: none;
  background-image:
    linear-gradient(rgba(255,255,255,0.02) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255,255,255,0.02) 1px, transparent 1px);
  background-size: 24px 24px;
  mask-image: linear-gradient(180deg, rgba(0,0,0,0.35), transparent 75%);
}

.nav,
.hero-deck,
.proof-strip,
.activation-grid,
.ops-section,
.shipping-ledger,
.source-block,
.site-footer {
  border: 1px solid var(--line);
  background: linear-gradient(180deg, rgba(255,255,255,0.03), rgba(255,255,255,0.01));
  box-shadow: var(--shadow);
}

.hero-deck {
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) minmax(320px, 0.8fr);
  gap: 18px;
  padding: 24px;
}

.proof-strip {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
  padding: 18px;
}

.activation-grid {
  display: grid;
  grid-template-columns: minmax(260px, 0.75fr) minmax(0, 1.25fr);
  gap: 18px;
  padding: 22px;
}

.ops-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.ship-state-list {
  display: grid;
  gap: 12px;
}

.ship-state {
  display: grid;
  grid-template-columns: 120px 1fr;
  gap: 14px;
  padding: 16px;
  border: 1px solid var(--line);
  background: rgba(7, 11, 16, 0.58);
}

.hero-cta-primary {
  background: var(--amber);
  color: #0d1117;
  font-weight: 700;
}

.proof-value,
.state-label,
.eyebrow,
.module-label,
.proof-label,
.nav-tag,
.command-note {
  font-family: var(--font-mono);
}

.panel-pre,
.command-pre {
  overflow-x: auto;
  white-space: pre;
}

@media (max-width: 900px) {
  .page-shell {
    width: min(100% - 18px, 1220px);
    padding-top: 12px;
  }

  .nav,
  .hero-deck,
  .proof-strip,
  .activation-grid,
  .ops-grid,
  .ship-state,
  .footer-links {
    grid-template-columns: 1fr;
  }

  .hero-actions {
    display: grid;
    gap: 10px;
  }

  .nav-links {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }
}

@media (max-width: 480px) {
  h1 {
    font-size: clamp(2.2rem, 13vw, 3.4rem);
  }

  .proof-strip,
  .activation-grid,
  .ops-section,
  .shipping-ledger,
  .source-block,
  .site-footer {
    padding: 16px;
  }

  .command-pre,
  .panel-pre {
    font-size: 13px;
  }
}
```

Keep the existing no-script and reduced-motion rules.

- [ ] **Step 4: Run the browser smoke test to verify it passes**

Run:

```bash
cd <repo>/gormes/www.gormes.ai
npm run test:e2e
```

Expected:

```text
2 passed
```

- [ ] **Step 5: Commit**

Run:

```bash
git add gormes/www.gormes.ai/internal/site/static/site.css gormes/www.gormes.ai/tests/home.spec.mjs
git commit -m "feat(gormes/www): ship operator console styling"
```

---

### Task 4: Full verification and export proof

**Files:**
- Verify only

- [ ] **Step 1: Run the full Go site test suite**

Run:

```bash
cd <repo>/gormes/www.gormes.ai
go test ./...
```

Expected:

```text
ok  	github.com/TrebuchetDynamics/gormes-agent/gormes/www.gormes.ai/internal/site
```

- [ ] **Step 2: Run the browser smoke suite**

Run:

```bash
cd <repo>/gormes/www.gormes.ai
npm run test:e2e
```

Expected:

```text
2 passed
```

- [ ] **Step 3: Verify the static export output**

Run:

```bash
cd <repo>/gormes/www.gormes.ai
rm -rf dist
go run ./cmd/www-gormes-export
test -f dist/index.html
test -f dist/static/site.css
rg -n "7\\.9 MB|gormes\\.io" dist/index.html
```

Expected:

```text
2026/... INFO exported www.gormes.ai out=dist
dist/index.html and dist/static/site.css exist
rg prints no matches
```

- [ ] **Step 4: Verify the current Pages settings still match**

Use:

```text
Root directory: gormes/www.gormes.ai
Build command: go run ./cmd/www-gormes-export
Build output directory: dist
```
