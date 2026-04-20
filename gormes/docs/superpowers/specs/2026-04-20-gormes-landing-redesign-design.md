# Gormes Landing Redesign

**Status:** Approved 2026-04-20
**Owner:** xel
**Implementation skill:** subagent-driven-development

## Goal

Replace the current `gormes.ai` landing page with a shorter, polished, Hermes-inspired layout. Keep one Gormes-specific section — the "Shipping State" ledger — so the page stays honest about what isn't shipped yet (Phase 3 memory, Phase 4 brain). Lead with the deployment story (zero-CGO single static binary) because that's the angle that justifies the existence of a Go port over the Python upstream.

## Approach

"Inspired by Hermes": borrow the upstream `hermes-agent.nousresearch.com` page's brevity, hero treatment, and 4–6-card features rhythm. Drop the current operator-console framing (Boot Sequence panel, Proof Rail, 3-step activation, 4 ops modules). Keep only the shipping ledger so credibility doesn't collapse.

The current Gormes page renders from `gormes/www.gormes.ai/internal/site/`:

- `content.go` — `LandingPage` struct + `DefaultPage()` factory holding all copy
- `templates/layout.tmpl` — outer shell (header nav, footer)
- `templates/index.tmpl` — section composition
- `templates/partials/*.tmpl` — per-section partials (`command_step`, `ops_module`, `proof_stat`, `ship_state`)
- `static/site.css` — dark theme styling

The redesign reuses this Go-rendered architecture. No client-side JS, no framework swap. Net result is a cleaner `LandingPage`, fewer partials, simpler CSS.

## Page Structure (top to bottom)

1. **Header nav** — `Gormes` brandmark · links: Install, Features, Roadmap, GitHub
2. **Hero** — kicker `OPEN SOURCE · MIT LICENSE` → headline `Hermes, In a Single Static Binary.` → subhead → two CTAs (Install primary, View Source secondary)
3. **Install** — two-column numbered steps `1. INSTALL` (curl one-liner) + `2. RUN` (`gormes`) + footnote linking to upstream Hermes install
4. **Features** — section label + section headline + 2×2 grid of 4 cards: Single Static Binary, Boots Like a Tool, In-Process Tool Loop, Survives Dropped Streams
5. **Shipping State** — section label + headline + 4-row compact ledger (Phase 1 SHIPPED, Phase 2 SHIPPED, Phase 3 NEXT, Phase 4 LATER) with colored status pills
6. **Footer** — version + attribution on left, license on right

No demo CTA section, no Inspect-the-Machine link list, no Boot Sequence panel.

## Locked Copy

### Hero
- **Kicker:** `OPEN SOURCE · MIT LICENSE`
- **Headline:** `Hermes, In a Single Static Binary.`
- **Subhead:** `Zero-CGO. No Python runtime on the host. One file you scp anywhere — Termux, Alpine, a fresh VPS — and it runs the same Hermes brain.`
- **Primary CTA:** `Install` → `#install`
- **Secondary CTA:** `View Source` → `https://github.com/TrebuchetDynamics/gormes-agent`

### Install
- **Step 1 label:** `1. INSTALL`
- **Step 1 command:** `curl -fsSL https://gormes.ai/install.sh | sh`
- **Step 2 label:** `2. RUN`
- **Step 2 command:** `gormes`
- **Footnote:** `Requires Hermes backend at localhost:8642.` followed by link `Install Hermes →` pointing at `https://github.com/NousResearch/hermes-agent#quickstart`

### Features
- **Section label:** `FEATURES`
- **Section headline:** `Why a Go layer matters.`
- **Card 1 — Single Static Binary:** `Zero CGO. ~8 MB. scp it to Termux, Alpine, a fresh VPS — it runs.`
- **Card 2 — Boots Like a Tool:** `No Python warmup. 16 ms render mailbox keeps the TUI responsive under load.`
- **Card 3 — In-Process Tool Loop:** `Streamed tool_calls execute against a Go-native registry. No bounce through Python.`
- **Card 4 — Survives Dropped Streams:** `Route-B reconnect treats SSE drops as a resilience problem, not a happy-path omission.`

### Shipping State
- **Section label:** `SHIPPING STATE`
- **Section headline:** `What ships now, what doesn't.`
- **Row 1:** `SHIPPED` · `Phase 1 — Bubble Tea TUI shell.`
- **Row 2:** `SHIPPED` · `Phase 2 — Tool registry + Telegram adapter + session resume.`
- **Row 3:** `NEXT` · `Phase 3 — SQLite + FTS5 transcript memory.`
- **Row 4:** `LATER` · `Phase 4 — Native prompt building + agent orchestration.`

### Footer
- **Left:** `Gormes v0.1.0 · TrebuchetDynamics` (version pulled from `cmd/gormes/version.go` if practical; otherwise hardcoded for now)
- **Right:** `MIT License · 2026`

## Visual Style

- **Theme:** dark, mirrored from current page but flatter
- **Background:** `#0b0d12`
- **Surface (cards, code blocks):** `#13161e`
- **Border:** `#1c1f29`
- **Body text:** `#e6e9ef`
- **Muted text (subhead, footnote, labels):** `#e6e9ef` at 55–78 % opacity
- **Accent (primary CTA):** `#5468ff` (indigo)
- **Status pill colors (shipping ledger):**
  - SHIPPED: bg `#0e3b21` / text `#5be79a`
  - NEXT: bg `#3b2c0e` / text `#e7c25b`
  - LATER: bg `#1f2434` / text `#8a99c7`
- **Typography:** existing system font stack stays; only sizing/weights change. Code blocks use system monospace.
- **Spacing rhythm:** sections separated by a 1px `#1c1f29` rule + ~40–60 px vertical padding.

## Out of Scope

- Animations, scroll effects, hover micro-interactions beyond CSS focus/hover defaults
- Light-mode toggle
- Internationalization
- Open Graph image redesign (existing one stays)
- Any change to `install.sh` or the install pipeline
- Any change to the docs.gormes.ai site
- Reordering or modifying the existing CTA destinations beyond what's locked above
- Pulling version dynamically from build flags if it requires plumbing through `ldflags` — hardcode `v0.1.0` and revisit later

## Implementation Notes

- Keep `LandingPage` struct in `content.go` but trim it down to only the fields the new page needs (Hero, Install, Features, ShippingStates, Footer). Remove obsolete fields (HeroPanel, ProofStats, ActivationSteps, OpsModules, ContributorLinks). Update tests accordingly — `render_test.go`, `static_export_test.go`, `tests/home.spec.mjs` all assert on copy that will change.
- Replace `templates/index.tmpl` with the new section composition. Delete or rewrite `command_step.tmpl`, `proof_stat.tmpl`, `ops_module.tmpl`. Keep `ship_state.tmpl` (may need a slight visual update for the new pill style).
- Rewrite `static/site.css`. Existing CSS is ~10.6 KB; expect ~6–8 KB after the rewrite. Single-file CSS, no preprocessor.
- Tests must reject the old hero copy ("Run Hermes Through a Go Operator Console.") and the old activation steps. Tests must require the new locked copy strings (especially the install one-liner already at `dist/install.sh`, and the hero headline).
- Preserve the existing exported routes and embed shape (`assets.go`'s `[]assets`, `siteFS`, `NewServer()`, `ExportDir()`). The Cloudflare Pages deploy workflow is unchanged — `go run ./cmd/www-gormes-export` produces `dist/index.html`, `dist/install.sh`, `dist/static/site.css` exactly as today.
- Smoke-test by running `go run ./cmd/www-gormes -listen :8080` locally and curling `/`, `/install.sh`, and `/static/site.css`.

## Acceptance Criteria

- `go test ./...` from `gormes/www.gormes.ai/` passes (existing tests rewritten against new copy).
- Rendered `/` contains, verbatim:
  - `Hermes, In a Single Static Binary.`
  - `curl -fsSL https://gormes.ai/install.sh | sh`
  - `Why a Go layer matters.`
  - `What ships now, what doesn't.`
  - All four shipping ledger row labels.
- Rendered `/` does **not** contain any of:
  - `Run Hermes Through a Go Operator Console.`
  - `Boot Sequence`
  - `Proof Rail`
  - `01 / INSTALL HERMES` (or any other activation-step label)
  - `Why Hermes users switch`
  - `Inspect the Machine`
- `/install.sh` continues to serve the bootstrap script unchanged.
- `dist/` static export contains `index.html`, `install.sh`, `static/site.css`, and nothing else.
- Visual sanity check at `localhost:8080` matches the approved mockup at `.superpowers/brainstorm/651995-1776692879/content/full-page.html`.
