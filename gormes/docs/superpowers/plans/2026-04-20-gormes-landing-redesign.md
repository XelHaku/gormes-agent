# Gormes Landing Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current `gormes.ai` landing page with a Hermes-inspired layout: kicker hero + 2-step install + 4-card features + compact shipping ledger + footer. Dark theme, single accent color, no client-side JS.

**Architecture:** The page renders from `gormes/www.gormes.ai/internal/site/`. Reuse the existing Go-templated, embed-served architecture. Trim the `LandingPage` struct, replace `index.tmpl`, swap obsolete partials for two new ones, rewrite `site.css`. Acceptance criteria are the verbatim copy strings locked in the spec.

**Tech Stack:** Go 1.25, `html/template`, `embed`, plain CSS, Playwright (smoke).

**Spec:** `gormes/docs/superpowers/specs/2026-04-20-gormes-landing-redesign-design.md`

---

## File Structure

| Action | Path | Purpose |
|---|---|---|
| Modify | `gormes/www.gormes.ai/internal/site/render_test.go` | Assert new hero/install/features/ledger copy renders; reject old strings. |
| Modify | `gormes/www.gormes.ai/internal/site/static_export_test.go` | Same assertions against `dist/index.html`. |
| Modify | `gormes/www.gormes.ai/tests/home.spec.mjs` | Playwright smoke against new headings + install command + mobile overflow. |
| Modify | `gormes/www.gormes.ai/internal/site/content.go` | Rewrite `LandingPage` struct + `DefaultPage()`. Drop obsolete fields. Add InstallStep, FeatureCard. |
| Modify | `gormes/www.gormes.ai/internal/site/templates/layout.tmpl` | Slim header/footer, new nav links, footer with `FooterLeft`/`FooterRight`. |
| Modify | `gormes/www.gormes.ai/internal/site/templates/index.tmpl` | New section composition: hero → install → features → ledger. |
| Modify | `gormes/www.gormes.ai/internal/site/templates/partials/ship_state.tmpl` | New pill style using `Tone` field for CSS class. |
| Create | `gormes/www.gormes.ai/internal/site/templates/partials/install_step.tmpl` | Renders one numbered install step (label + command). |
| Create | `gormes/www.gormes.ai/internal/site/templates/partials/feature_card.tmpl` | Renders one feature card (title + body). |
| Delete | `gormes/www.gormes.ai/internal/site/templates/partials/command_step.tmpl` | Obsolete (replaced by install_step). |
| Delete | `gormes/www.gormes.ai/internal/site/templates/partials/proof_stat.tmpl` | Obsolete (Proof Rail dropped). |
| Delete | `gormes/www.gormes.ai/internal/site/templates/partials/ops_module.tmpl` | Obsolete (Ops modules dropped). |
| Modify | `gormes/www.gormes.ai/internal/site/static/site.css` | Full rewrite. New variables, hero/install/features/ledger sections, mobile media query. |

The `assets.go`, `server.go`, `cmd/www-gormes/`, and `cmd/www-gormes-export/` files don't change.

---

## Task 1: Lock failing tests to the new copy

This task commits a red main (tests fail because templates still render old copy). Subsequent tasks turn it green. The failing tests pin the acceptance criteria so the next task can't drift.

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/render_test.go`
- Modify: `gormes/www.gormes.ai/internal/site/static_export_test.go`

- [ ] **Step 1: Replace `render_test.go` with the new assertions**

Overwrite `gormes/www.gormes.ai/internal/site/render_test.go` with:

```go
package site

import (
	"io/fs"
	"strings"
	"testing"
)

func TestRenderIndex_RendersRedesignedLanding(t *testing.T) {
	body, err := RenderIndex()
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}

	text := string(body)
	wants := []string{
		// Hero
		"OPEN SOURCE · MIT LICENSE",
		"Hermes, In a Single Static Binary.",
		"Zero-CGO. No Python runtime on the host. One file you scp anywhere",
		// Install
		"1. INSTALL",
		"curl -fsSL https://gormes.ai/install.sh | sh",
		"2. RUN",
		"Requires Hermes backend at localhost:8642.",
		"Install Hermes →",
		// Features
		"FEATURES",
		"Why a Go layer matters.",
		"Single Static Binary",
		"Boots Like a Tool",
		"In-Process Tool Loop",
		"Survives Dropped Streams",
		"Route-B reconnect treats SSE drops",
		// Shipping ledger
		"SHIPPING STATE",
		"What ships now, what doesn&#39;t.",
		"Phase 1 — Bubble Tea TUI shell.",
		"Phase 2 — Tool registry + Telegram adapter + session resume.",
		"Phase 3 — SQLite + FTS5 transcript memory.",
		"Phase 4 — Native prompt building + agent orchestration.",
		// Footer
		"Gormes v0.1.0 · TrebuchetDynamics",
		"MIT License · 2026",
		// CSS link
		`href="/static/site.css"`,
	}
	rejects := []string{
		"Run Hermes Through a Go Operator Console.",
		"Boot Sequence",
		"Proof Rail",
		"01 / INSTALL HERMES",
		"Why Hermes users switch",
		"Inspect the Machine",
		"<script",
	}

	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered page missing %q\nbody:\n%s", want, text)
		}
	}
	for _, reject := range rejects {
		if strings.Contains(text, reject) {
			t.Fatalf("rendered page still contains stale token %q", reject)
		}
	}
}

func TestEmbeddedTemplates_ArePresentAndParse(t *testing.T) {
	files := []string{
		"templates/layout.tmpl",
		"templates/index.tmpl",
		"templates/partials/install_step.tmpl",
		"templates/partials/feature_card.tmpl",
		"templates/partials/ship_state.tmpl",
	}

	for _, name := range files {
		body, err := fs.ReadFile(templateFS, name)
		if err != nil {
			t.Fatalf("embedded template %q missing: %v", name, err)
		}
		if len(body) == 0 {
			t.Fatalf("embedded template %q is empty", name)
		}
	}

	templates, err := parseTemplates()
	if err != nil {
		t.Fatalf("parseTemplates: %v", err)
	}

	for _, want := range []string{"layout", "index", "install_step", "feature_card", "ship_state"} {
		if templates.Lookup(want) == nil {
			t.Fatalf("parsed templates missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Replace `static_export_test.go` with the new assertions**

Overwrite `gormes/www.gormes.ai/internal/site/static_export_test.go` with:

```go
package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportDir_WritesStaticSite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "dist")

	if err := ExportDir(root); err != nil {
		t.Fatalf("ExportDir: %v", err)
	}

	indexBody, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	text := string(indexBody)
	wants := []string{
		"Hermes, In a Single Static Binary.",
		"curl -fsSL https://gormes.ai/install.sh | sh",
		"Why a Go layer matters.",
		"What ships now, what doesn&#39;t.",
		"Phase 4 — Native prompt building + agent orchestration.",
	}
	rejects := []string{
		"Run Hermes Through a Go Operator Console.",
		"Boot Sequence",
		"Proof Rail",
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("dist/index.html missing %q", want)
		}
	}
	for _, reject := range rejects {
		if strings.Contains(text, reject) {
			t.Fatalf("dist/index.html still contains stale token %q", reject)
		}
	}

	cssPath := filepath.Join(root, "static", "site.css")
	css, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("read %s: %v", cssPath, err)
	}
	if !strings.Contains(string(css), "--bg-0") {
		t.Fatalf("site.css missing --bg-0 design token")
	}

	installBody, err := os.ReadFile(filepath.Join(root, "install.sh"))
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	if !strings.Contains(string(installBody), "github.com/TrebuchetDynamics/gormes-agent/gormes/cmd/gormes") {
		t.Fatalf("install.sh missing TrebuchetDynamics module path")
	}
}

func TestExportDir_RecreatesDist(t *testing.T) {
	root := filepath.Join(t.TempDir(), "dist")
	stalePath := filepath.Join(root, "stale.txt")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := ExportDir(root); err != nil {
		t.Fatalf("ExportDir: %v", err)
	}

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale file still present after export: err=%v", err)
	}
}
```

- [ ] **Step 3: Run the tests and confirm they fail**

Run from `gormes/www.gormes.ai/`:

```bash
go test ./internal/site/ -run "TestRenderIndex_RendersRedesignedLanding|TestEmbeddedTemplates_ArePresentAndParse|TestExportDir_WritesStaticSite" 2>&1 | tail -10
```

Expected: `FAIL` on all three — most likely `rendered page missing "OPEN SOURCE · MIT LICENSE"` or template parse errors. The red is the point.

- [ ] **Step 4: Commit (red tests pinned)**

```bash
cd <repo> && \
git add gormes/www.gormes.ai/internal/site/render_test.go \
        gormes/www.gormes.ai/internal/site/static_export_test.go && \
git commit -m "test(gormes/www): pin acceptance for landing redesign

Rewrite render_test and static_export_test against the locked copy
in 2026-04-20-gormes-landing-redesign-design.md. Tests fail until
content.go + templates + CSS land in the next task.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Rewrite `content.go` + templates → green

Lands the data layer (`content.go`) and template files together. They're coupled — `html/template` errors at execution if the template references a struct field that no longer exists. Run as one atomic change.

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/content.go`
- Modify: `gormes/www.gormes.ai/internal/site/templates/layout.tmpl`
- Modify: `gormes/www.gormes.ai/internal/site/templates/index.tmpl`
- Modify: `gormes/www.gormes.ai/internal/site/templates/partials/ship_state.tmpl`
- Create: `gormes/www.gormes.ai/internal/site/templates/partials/install_step.tmpl`
- Create: `gormes/www.gormes.ai/internal/site/templates/partials/feature_card.tmpl`
- Delete: `gormes/www.gormes.ai/internal/site/templates/partials/command_step.tmpl`
- Delete: `gormes/www.gormes.ai/internal/site/templates/partials/proof_stat.tmpl`
- Delete: `gormes/www.gormes.ai/internal/site/templates/partials/ops_module.tmpl`

- [ ] **Step 1: Replace `content.go` entirely**

Overwrite `gormes/www.gormes.ai/internal/site/content.go` with:

```go
package site

type NavLink struct {
	Label string
	Href  string
}

type Link struct {
	Label string
	Href  string
}

type InstallStep struct {
	Label   string
	Command string
}

type FeatureCard struct {
	Title string
	Body  string
}

// ShipState renders one row of the shipping ledger.
// State is the display label ("SHIPPED", "NEXT", "LATER").
// Tone is the lowercase CSS-class suffix used by .status-<tone>.
type ShipState struct {
	State string
	Tone  string
	Name  string
}

type LandingPage struct {
	Title               string
	Description         string
	Nav                 []NavLink
	HeroKicker          string
	HeroHeadline        string
	HeroSubhead         string
	PrimaryCTA          Link
	SecondaryCTA        Link
	InstallSteps        []InstallStep
	InstallFootnote     string
	InstallFootnoteLink string
	InstallFootnoteHref string
	FeaturesLabel       string
	FeaturesHeadline    string
	FeatureCards        []FeatureCard
	LedgerLabel         string
	LedgerHeadline      string
	ShippingStates      []ShipState
	FooterLeft          string
	FooterRight         string
}

func DefaultPage() LandingPage {
	return LandingPage{
		Title:       "Gormes — Hermes, In a Single Static Binary",
		Description: "Zero-CGO Go shell for Hermes Agent. One static binary, in-process tool loop, Route-B reconnect.",
		Nav: []NavLink{
			{Label: "Install", Href: "#install"},
			{Label: "Features", Href: "#features"},
			{Label: "Roadmap", Href: "#roadmap"},
			{Label: "GitHub", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
		},
		HeroKicker:   "OPEN SOURCE · MIT LICENSE",
		HeroHeadline: "Hermes, In a Single Static Binary.",
		HeroSubhead:  "Zero-CGO. No Python runtime on the host. One file you scp anywhere — Termux, Alpine, a fresh VPS — and it runs the same Hermes brain.",
		PrimaryCTA:   Link{Label: "Install", Href: "#install"},
		SecondaryCTA: Link{Label: "View Source", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
		InstallSteps: []InstallStep{
			{Label: "1. INSTALL", Command: "curl -fsSL https://gormes.ai/install.sh | sh"},
			{Label: "2. RUN", Command: "gormes"},
		},
		InstallFootnote:     "Requires Hermes backend at localhost:8642.",
		InstallFootnoteLink: "Install Hermes →",
		InstallFootnoteHref: "https://github.com/NousResearch/hermes-agent#quickstart",
		FeaturesLabel:       "FEATURES",
		FeaturesHeadline:    "Why a Go layer matters.",
		FeatureCards: []FeatureCard{
			{Title: "Single Static Binary", Body: "Zero CGO. ~8 MB. scp it to Termux, Alpine, a fresh VPS — it runs."},
			{Title: "Boots Like a Tool", Body: "No Python warmup. 16 ms render mailbox keeps the TUI responsive under load."},
			{Title: "In-Process Tool Loop", Body: "Streamed tool_calls execute against a Go-native registry. No bounce through Python."},
			{Title: "Survives Dropped Streams", Body: "Route-B reconnect treats SSE drops as a resilience problem, not a happy-path omission."},
		},
		LedgerLabel:    "SHIPPING STATE",
		LedgerHeadline: "What ships now, what doesn't.",
		ShippingStates: []ShipState{
			{State: "SHIPPED", Tone: "shipped", Name: "Phase 1 — Bubble Tea TUI shell."},
			{State: "SHIPPED", Tone: "shipped", Name: "Phase 2 — Tool registry + Telegram adapter + session resume."},
			{State: "NEXT", Tone: "next", Name: "Phase 3 — SQLite + FTS5 transcript memory."},
			{State: "LATER", Tone: "later", Name: "Phase 4 — Native prompt building + agent orchestration."},
		},
		FooterLeft:  "Gormes v0.1.0 · TrebuchetDynamics",
		FooterRight: "MIT License · 2026",
	}
}
```

- [ ] **Step 2: Replace `layout.tmpl`**

Overwrite `gormes/www.gormes.ai/internal/site/templates/layout.tmpl` with:

```html
{{define "layout"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <meta name="description" content="{{.Description}}">
  <link rel="stylesheet" href="/static/site.css">
</head>
<body>
  <div class="page">
    <header class="topbar">
      <div class="topbar-inner">
        <a class="brand" href="#top">Gormes</a>
        <nav class="topnav" aria-label="Primary">
          {{range .Nav}}<a href="{{.Href}}">{{.Label}}</a>{{end}}
        </nav>
      </div>
    </header>
    <main>
      {{template "index" .}}
    </main>
    <footer class="footer">
      <div class="footer-inner">
        <p class="footer-left">{{.FooterLeft}}</p>
        <p class="footer-right">{{.FooterRight}}</p>
      </div>
    </footer>
  </div>
</body>
</html>
{{end}}
```

- [ ] **Step 3: Replace `index.tmpl`**

Overwrite `gormes/www.gormes.ai/internal/site/templates/index.tmpl` with:

```html
{{define "index"}}
<section id="top" class="hero">
  <div class="hero-inner">
    <p class="kicker">{{.HeroKicker}}</p>
    <h1 class="hero-title">{{.HeroHeadline}}</h1>
    <p class="hero-subhead">{{.HeroSubhead}}</p>
    <div class="hero-ctas">
      <a class="btn btn-primary" href="{{.PrimaryCTA.Href}}">{{.PrimaryCTA.Label}}</a>
      <a class="btn btn-secondary" href="{{.SecondaryCTA.Href}}">{{.SecondaryCTA.Label}}</a>
    </div>
  </div>
</section>

<section id="install" class="install">
  <div class="install-inner">
    <div class="install-grid">
      {{range .InstallSteps}}{{template "install_step" .}}{{end}}
    </div>
    <p class="install-footnote">{{.InstallFootnote}} <a href="{{.InstallFootnoteHref}}">{{.InstallFootnoteLink}}</a></p>
  </div>
</section>

<section id="features" class="features">
  <div class="features-inner">
    <p class="kicker">{{.FeaturesLabel}}</p>
    <h2 class="section-title">{{.FeaturesHeadline}}</h2>
    <div class="features-grid">
      {{range .FeatureCards}}{{template "feature_card" .}}{{end}}
    </div>
  </div>
</section>

<section id="roadmap" class="ledger">
  <div class="ledger-inner">
    <p class="kicker">{{.LedgerLabel}}</p>
    <h2 class="section-title">{{.LedgerHeadline}}</h2>
    <ul class="ledger-list">
      {{range .ShippingStates}}{{template "ship_state" .}}{{end}}
    </ul>
  </div>
</section>
{{end}}
```

- [ ] **Step 4: Create `partials/install_step.tmpl`**

Create the file with:

```html
{{define "install_step"}}
<div class="install-step">
  <p class="kicker">{{.Label}}</p>
  <pre class="cmd"><code>{{.Command}}</code></pre>
</div>
{{end}}
```

- [ ] **Step 5: Create `partials/feature_card.tmpl`**

Create the file with:

```html
{{define "feature_card"}}
<article class="feature-card">
  <h3>{{.Title}}</h3>
  <p>{{.Body}}</p>
</article>
{{end}}
```

- [ ] **Step 6: Replace `partials/ship_state.tmpl`**

Overwrite `gormes/www.gormes.ai/internal/site/templates/partials/ship_state.tmpl` with:

```html
{{define "ship_state"}}
<li class="ledger-row">
  <span class="status status-{{.Tone}}">{{.State}}</span>
  <span class="ledger-text">{{.Name}}</span>
</li>
{{end}}
```

- [ ] **Step 7: Delete the three obsolete partials**

Run from the repo root:

```bash
cd <repo> && \
git rm gormes/www.gormes.ai/internal/site/templates/partials/command_step.tmpl \
       gormes/www.gormes.ai/internal/site/templates/partials/proof_stat.tmpl \
       gormes/www.gormes.ai/internal/site/templates/partials/ops_module.tmpl
```

Expected output: three `rm '...'` lines.

- [ ] **Step 8: Run the tests and confirm green**

Run from `gormes/www.gormes.ai/`:

```bash
go test ./internal/site/ 2>&1 | tail -5
```

Expected: `ok  github.com/TrebuchetDynamics/gormes-agent/gormes/www.gormes.ai/internal/site  0.0Ns`.

If a test fails because of a copy mismatch, do NOT loosen the test — fix `content.go` to match the spec's locked copy exactly.

- [ ] **Step 9: Commit**

```bash
cd <repo> && \
git add gormes/www.gormes.ai/internal/site/content.go \
        gormes/www.gormes.ai/internal/site/templates/layout.tmpl \
        gormes/www.gormes.ai/internal/site/templates/index.tmpl \
        gormes/www.gormes.ai/internal/site/templates/partials/install_step.tmpl \
        gormes/www.gormes.ai/internal/site/templates/partials/feature_card.tmpl \
        gormes/www.gormes.ai/internal/site/templates/partials/ship_state.tmpl && \
git commit -m "feat(gormes/www): rewrite landing for Hermes-inspired layout

New LandingPage struct (Hero/Install/Features/ShippingStates/Footer)
with locked copy from the redesign spec. New index.tmpl composes
hero → 2-step install → 4-card features → compact shipping ledger.
Adds install_step + feature_card partials, retones ship_state for
the new pill style. Drops command_step, proof_stat, ops_module
partials (were only used by the dropped sections).

Tests committed in the previous commit now pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Rewrite `site.css`

Visual style matching the approved mockup. Tests already pass (they only check text), so this task is verified by visual inspection at `localhost:8080`.

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/static/site.css`

- [ ] **Step 1: Replace `site.css` entirely**

Overwrite `gormes/www.gormes.ai/internal/site/static/site.css` with:

```css
/* gormes.ai landing — dark theme, single-accent. No JS. */

:root {
  --bg-0: #0b0d12;
  --bg-1: #13161e;
  --border: #1c1f29;
  --text: #e6e9ef;
  --muted: rgba(230, 233, 239, 0.65);
  --muted-strong: rgba(230, 233, 239, 0.78);
  --label: rgba(230, 233, 239, 0.55);
  --accent: #5468ff;
  --accent-hover: #6878ff;
  --status-shipped-bg: #0e3b21;
  --status-shipped-fg: #5be79a;
  --status-next-bg: #3b2c0e;
  --status-next-fg: #e7c25b;
  --status-later-bg: #1f2434;
  --status-later-fg: #8a99c7;
  --max-width: 880px;
  --pad: 28px;
  --page-bg: var(--bg-0);
}

* { box-sizing: border-box; }

html, body {
  margin: 0;
  padding: 0;
  background: var(--bg-0);
  color: var(--text);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
}

a { color: var(--accent); text-decoration: none; }
a:hover { color: var(--accent-hover); }

.page { max-width: var(--max-width); margin: 0 auto; }

/* Topbar */
.topbar { border-bottom: 1px solid var(--border); }
.topbar-inner {
  padding: 14px var(--pad);
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.brand { font-weight: 700; letter-spacing: 1px; color: var(--text); }
.topnav a { font-size: 13px; color: var(--muted); margin-left: 18px; }
.topnav a:hover { color: var(--text); }

/* Section labels */
.kicker {
  font-size: 11px;
  letter-spacing: 2px;
  color: var(--label);
  margin: 0 0 14px;
  text-transform: uppercase;
}

/* Hero */
.hero-inner { padding: 60px var(--pad) 50px; }
.hero-title { font-size: 40px; font-weight: 700; line-height: 1.1; margin: 0 0 16px; }
.hero-subhead {
  font-size: 15px;
  color: var(--muted-strong);
  max-width: 640px;
  margin: 0 0 24px;
  line-height: 1.55;
}
.hero-ctas { display: flex; gap: 10px; flex-wrap: wrap; }

.btn {
  display: inline-block;
  font-size: 13px;
  font-weight: 600;
  padding: 10px 18px;
  border-radius: 6px;
  border: 1px solid transparent;
  transition: background 0.15s, border-color 0.15s, color 0.15s;
}
.btn-primary { background: var(--accent); color: #fff; }
.btn-primary:hover { background: var(--accent-hover); color: #fff; }
.btn-secondary {
  background: transparent;
  color: var(--text);
  border-color: var(--border);
}
.btn-secondary:hover { border-color: var(--muted); }

/* Install */
.install { border-top: 1px solid var(--border); }
.install-inner { padding: 32px var(--pad); }
.install-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; }
.install-step { display: flex; flex-direction: column; gap: 6px; }
.install-step .kicker { margin: 0; }
.cmd {
  background: var(--bg-1);
  padding: 14px;
  border-radius: 5px;
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: 12px;
  margin: 0;
  overflow-x: auto;
}
.cmd code { color: var(--text); }
.install-footnote {
  font-size: 11px;
  color: var(--label);
  margin: 14px 0 0;
}
.install-footnote a { color: var(--accent); }

/* Features */
.features { border-top: 1px solid var(--border); }
.features-inner { padding: 50px var(--pad); }
.section-title { font-size: 24px; font-weight: 700; margin: 0 0 24px; }
.features-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.feature-card { background: var(--bg-1); padding: 22px; border-radius: 6px; }
.feature-card h3 { margin: 0 0 8px; font-size: 14px; font-weight: 700; }
.feature-card p { margin: 0; font-size: 12px; color: var(--muted-strong); line-height: 1.55; }

/* Shipping ledger */
.ledger { border-top: 1px solid var(--border); }
.ledger-inner { padding: 40px var(--pad); }
.ledger-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 8px;
}
.ledger-row {
  background: var(--bg-1);
  padding: 14px 16px;
  border-radius: 5px;
  display: flex;
  align-items: center;
  gap: 14px;
}
.status {
  font-size: 10px;
  font-weight: 700;
  padding: 4px 9px;
  border-radius: 3px;
  letter-spacing: 1px;
  flex-shrink: 0;
}
.status-shipped { background: var(--status-shipped-bg); color: var(--status-shipped-fg); }
.status-next    { background: var(--status-next-bg);    color: var(--status-next-fg); }
.status-later   { background: var(--status-later-bg);   color: var(--status-later-fg); }
.ledger-text { font-size: 13px; color: var(--muted-strong); }

/* Footer */
.footer { border-top: 1px solid var(--border); }
.footer-inner {
  padding: 24px var(--pad);
  display: flex;
  justify-content: space-between;
}
.footer p { margin: 0; font-size: 11px; color: var(--label); }

/* Responsive */
@media (max-width: 640px) {
  .hero-title { font-size: 28px; }
  .install-grid { grid-template-columns: 1fr; }
  .features-grid { grid-template-columns: 1fr; }
  .topnav a { margin-left: 12px; }
  .footer-inner { flex-direction: column; gap: 8px; }
}
```

- [ ] **Step 2: Verify Go tests still pass**

Run from `gormes/www.gormes.ai/`:

```bash
go test ./internal/site/ 2>&1 | tail -3
```

Expected: `ok` (tests already passed; CSS rewrite shouldn't have broken anything because tests only check `--bg-0` is present).

- [ ] **Step 3: Visual smoke at localhost:8080**

Run in one terminal:

```bash
cd gormes/www.gormes.ai && go run ./cmd/www-gormes -listen :8080
```

In another terminal:

```bash
curl -sS http://localhost:8080/ | head -30
```

Expected: HTML containing `<h1 class="hero-title">Hermes, In a Single Static Binary.</h1>`.

If you have a browser, open `http://localhost:8080` and check the layout matches the approved mockup at `.superpowers/brainstorm/651995-1776692879/content/full-page.html`. Stop the server with Ctrl-C.

- [ ] **Step 4: Commit**

```bash
cd <repo> && \
git add gormes/www.gormes.ai/internal/site/static/site.css && \
git commit -m "style(gormes/www): rewrite site.css for redesigned landing

Dark theme (--bg-0 #0b0d12, surface #13161e), single indigo accent
(#5468ff), kicker labels in 2px-letter-spaced uppercase, status
pills for the shipping ledger (shipped/next/later tones). Mobile
break at 640px collapses the install + features grids to single
column. ~6.5 KB total, no JS, no preprocessor.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Update Playwright smoke

Mirrors the Go assertions in browser space and re-checks mobile overflow.

**Files:**
- Modify: `gormes/www.gormes.ai/tests/home.spec.mjs`

- [ ] **Step 1: Replace `home.spec.mjs`**

Overwrite `gormes/www.gormes.ai/tests/home.spec.mjs` with:

```javascript
import { test, expect } from '@playwright/test';

test('homepage renders the redesigned landing', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('Gormes — Hermes, In a Single Static Binary');
  await expect(page.getByRole('heading', { name: 'Hermes, In a Single Static Binary.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Why a Go layer matters.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: "What ships now, what doesn't." })).toBeVisible();
  await expect(page.getByText('curl -fsSL https://gormes.ai/install.sh | sh')).toBeVisible();
  await expect(page.getByText('Requires Hermes backend at localhost:8642.')).toBeVisible();
  await expect(page.getByText('Run Hermes Through a Go Operator Console.')).toHaveCount(0);
  await expect(page.locator('link[href="/static/site.css"]')).toHaveCount(1);
  await expect(page.locator('script')).toHaveCount(0);
});

test('mobile keeps the install command readable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'Hermes, In a Single Static Binary.' })).toBeVisible();
  await expect(page.getByText('curl -fsSL https://gormes.ai/install.sh | sh')).toBeVisible();

  const hasOverflow = await page.evaluate(() =>
    document.documentElement.scrollWidth > window.innerWidth
  );
  expect(hasOverflow).toBeFalsy();
});
```

- [ ] **Step 2: Run Playwright (best-effort — skip if not installed)**

Run from `gormes/www.gormes.ai/`:

```bash
[ -d node_modules ] && npm run test:e2e 2>&1 | tail -15 || echo "playwright deps not installed; skipping (workflow runs Go tests only)"
```

Expected (if installed): two PASS lines. If not installed, the line is skipped — the deploy workflow doesn't gate on Playwright.

- [ ] **Step 3: Commit**

```bash
cd <repo> && \
git add gormes/www.gormes.ai/tests/home.spec.mjs && \
git commit -m "test(gormes/www): rewrite Playwright smoke for redesigned landing

Browser-side assertions on the new hero headline, the two section
headings (features + shipping ledger), the install command
visibility, and the upstream Hermes link footnote. Mobile test
keeps the no-horizontal-overflow invariant on a 390px viewport.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Final smoke + push

Verify end-to-end — `go test ./...`, static export, file inventory — then push.

**Files:** none modified.

- [ ] **Step 1: Full Go test pass**

Run from `gormes/www.gormes.ai/`:

```bash
go test ./... 2>&1 | tail -5
```

Expected: `ok  github.com/TrebuchetDynamics/gormes-agent/gormes/www.gormes.ai/internal/site  ...`. The two `cmd/` packages report `[no test files]` — that's fine.

- [ ] **Step 2: Build the static export and verify file inventory**

Run from `gormes/www.gormes.ai/`:

```bash
rm -rf /tmp/gormes-dist && \
go run ./cmd/www-gormes-export -out /tmp/gormes-dist && \
find /tmp/gormes-dist -type f | sort
```

Expected output (exactly these files, no others):

```
/tmp/gormes-dist/index.html
/tmp/gormes-dist/install.sh
/tmp/gormes-dist/static/site.css
```

- [ ] **Step 3: Verify acceptance strings render**

```bash
grep -F "Hermes, In a Single Static Binary." /tmp/gormes-dist/index.html && \
grep -F "curl -fsSL https://gormes.ai/install.sh | sh" /tmp/gormes-dist/index.html && \
grep -F "Why a Go layer matters." /tmp/gormes-dist/index.html && \
grep -F "Phase 4 — Native prompt building" /tmp/gormes-dist/index.html && \
echo "all acceptance strings present"
```

Expected: each grep prints its matching line, then `all acceptance strings present`.

- [ ] **Step 4: Verify rejected strings are gone**

```bash
! grep -F "Run Hermes Through a Go Operator Console." /tmp/gormes-dist/index.html && \
! grep -F "Boot Sequence" /tmp/gormes-dist/index.html && \
! grep -F "Proof Rail" /tmp/gormes-dist/index.html && \
echo "no stale strings"
```

Expected: nothing from grep, then `no stale strings`.

- [ ] **Step 5: Push to main**

```bash
git -C <repo> push origin main 2>&1 | tail -5
```

Expected: `<old>..<new>  main -> main` line.

The Cloudflare Pages workflow `Deploy gormes.ai landing` will trigger on the next push affecting `gormes/www.gormes.ai/**`. After ~1 minute it should be live at `https://gormes.ai/`.

- [ ] **Step 6: Post-deploy live verification**

Wait for the workflow run to finish, then:

```bash
curl -sS https://gormes.ai/ | head -20
```

Expected: HTML containing `<h1 class="hero-title">Hermes, In a Single Static Binary.</h1>`. If you instead see the old hero "Run Hermes Through a Go Operator Console", the Pages cache hasn't propagated yet — give it 30–60 seconds.

---

## Self-Review

**Spec coverage check:**

| Spec section | Plan task |
|---|---|
| Page Structure (top→bottom) | Task 2 (templates), Task 3 (CSS) |
| Locked Copy / Hero | Task 1 (assertions), Task 2 (`content.go` `DefaultPage`) |
| Locked Copy / Install | Task 1 (assertions), Task 2 (`InstallSteps` + footnote) |
| Locked Copy / Features | Task 1 (assertions), Task 2 (`FeatureCards`) |
| Locked Copy / Shipping State | Task 1 (assertions), Task 2 (`ShippingStates`) |
| Locked Copy / Footer | Task 1 (assertions), Task 2 (`FooterLeft`/`FooterRight`) |
| Visual Style | Task 3 (`site.css` rewrite — variables match the spec hex codes verbatim) |
| Implementation Notes / trim struct | Task 2 (Step 1 rewrites the struct; obsolete fields gone) |
| Implementation Notes / new partials | Task 2 (Steps 4–5) |
| Implementation Notes / delete obsolete partials | Task 2 (Step 7) |
| Implementation Notes / preserve assets.go shape | Untouched — neither file is in the modify list |
| Implementation Notes / smoke at localhost:8080 | Task 3 Step 3 |
| Acceptance Criteria / `go test ./...` | Task 5 Step 1 |
| Acceptance Criteria / verbatim strings present | Task 5 Step 3 |
| Acceptance Criteria / rejected strings absent | Task 5 Step 4 |
| Acceptance Criteria / `dist/` inventory | Task 5 Step 2 |

No spec gaps.

**Placeholder scan:** every step has either complete code or an exact command. No "implement later", no "similar to Task N", no vague "handle errors".

**Type consistency:** `LandingPage` field names used in `content.go` (Step 1) match exactly what `index.tmpl` (Step 3) and `layout.tmpl` (Step 2) reference. `InstallStep.Label`/`.Command` match what `install_step.tmpl` (Step 4) uses. `FeatureCard.Title`/`.Body` match `feature_card.tmpl` (Step 5). `ShipState.State`/`.Tone`/`.Name` match `ship_state.tmpl` (Step 6). CSS classes `.status-shipped`/`.status-next`/`.status-later` (Task 3) match the lowercase `Tone` values in `DefaultPage` (Task 2 Step 1).

No fixes needed.
