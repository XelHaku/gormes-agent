package site

import (
	"io/fs"
	"regexp"
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
		// Hero — serious infrastructure framing with short mobile lines.
		"OPEN SOURCE · MIT LICENSE",
		"UNDER CONSTRUCTION",
		"One Go Binary. No Python. No Drift.",
		// Terminal-prompt signature: mono "$" in accent color before
		// the serif headline. aria-hidden so screen readers don't
		// announce "dollar sign".
		`<span class="hero-prompt" aria-hidden="true">$</span>`,
		"Gormes is a Go-native runtime for AI agents.",
		"Built to solve the operations problem",
		"One static binary. No virtualenvs. No dependency hell.",
		// v6 hero filter splits the stamp from the body — "Early-stage."
		// reads as positioning identity in mono caps, the body line
		// below carries the caveat in muted body color.
		`class="hero-note"`,
		`class="hero-note-stamp"`,
		`class="hero-note-body"`,
		"Early-stage.",
		"Reliability-first runtime for developers who ship agents, not demos.",
		// v19 trimmed nav further: "Install" removed (duplicated the
		// INSTALL CTA button below). Roadmap + GitHub only.
		`<a href="#roadmap">Roadmap</a>`,
		`<a href="https://github.com/TrebuchetDynamics/gormes-agent">GitHub</a>`,
		// Install — clearer structure and promoted source-backed note.
		"INSTALL",
		"Source-backed installers. One managed checkout.",
		"1. UNIX / MACOS / TERMUX",
		"curl -fsSL https://gormes.ai/install.sh | sh",
		"2. WINDOWS POWERSHELL",
		"irm https://gormes.ai/install.ps1 | iex",
		"3. RUN",
		"Source-backed for now",
		"Read the installer source →",
		// Copy button (clipboard JS is allowed for this widget only)
		`class="copy-btn"`,
		"navigator.clipboard.writeText",
		// Features — pain frame before technical cards. v19 split
		// the combined headline into pain + fix-subhead.
		"WHY GORMES",
		"Why Hermes breaks in production",
		`class="why-fix-subhead"`,
		"How Gormes fixes it",
		"Hermes breaks in production because:",
		"environments drift",
		"installs fail",
		"agents crash mid-run",
		"streams drop and lose work",
		"Single Static Binary",
		"No Runtime Drift",
		"Streams That Don&#39;t Drop",
		"Local Validation",
		"Route-B reconnect treats SSE drops",
		"gormes doctor --offline",
		// Roadmap section — summary block first, full generated checklist collapsed.
		"BUILD STATE",
		"What works today, and what&#39;s still being wired up.",
		"Current focus",
		"Gateway stability",
		"Memory system",
		"Next milestone",
		"Full Go-native runtime, no Hermes",
		`<details class="roadmap-details">`,
		"View full phase-by-phase checklist",
		// Fuzzy phase-title presence (each phase renders)
		"Phase 1",
		"Phase 2",
		"Phase 3",
		"Phase 4",
		"Phase 5",
		"Phase 6",
		// Status tone classes driven by current phase-level data.
		"roadmap-status-progress",
		"roadmap-status-planned",
		// Complete work still appears at item level even when no whole phase is complete.
		"roadmap-item-shipped",
		// Structural class anchors
		"roadmap-phase",
		// Footer — brand text + company anchor + license
		`Gormes v0.1.0 · <a href="https://trebuchetdynamics.com/">TrebuchetDynamics</a>`,
		`<nav class="footer-nav" aria-label="Secondary">`,
		// v6: · separator now lives in the actual HTML (not just CSS
		// pseudo-elements) so screen readers / text-extraction views
		// see it. aria-hidden so SR users don't read "middle dot".
		`<span class="footer-nav-sep" aria-hidden="true">·</span>`,
		`<a href="https://docs.gormes.ai/">Docs</a>`,
		`<a href="https://trebuchetdynamics.com/">Company</a>`,
		"MIT License · 2026",
		// CSS link
		`href="/static/site.css"`,
		// Favicons — full set wired into <head>.
		`href="/static/favicon.ico"`,
		`href="/static/favicon-16x16.png"`,
		`href="/static/favicon-32x32.png"`,
		`href="/static/apple-touch-icon.png"`,
		// Open Graph + Twitter cards. Asserts the property/name keys
		// and the canonical URL/image so social previews stay healthy.
		`property="og:type" content="website"`,
		`property="og:site_name" content="Gormes"`,
		`property="og:url" content="https://gormes.ai/"`,
		`property="og:image" content="https://gormes.ai/static/social-card.png"`,
		`property="og:image:width" content="1200"`,
		`property="og:image:height" content="630"`,
		`name="twitter:card" content="summary_large_image"`,
		`name="twitter:image" content="https://gormes.ai/static/social-card.png"`,
	}
	rejects := []string{
		`<div class="hero-image">`,
		`go-gopher-bear-lowpoly.png`,
		"Run Hermes Through a Go Operator Console.",
		"Hermes, In a Single Static Binary.",
		"Requires Hermes backend at localhost:8642.",
		"Install Hermes →",
		"No Python runtime on the host",
		"Boot Sequence",
		"Proof Rail",
		"01 / INSTALL HERMES",
		"Why Hermes users switch",
		"Inspect the Machine",
		"~8 MB",
		"~12 MB",
		// Old hero/features copy that conflated frontend with full replacement
		"One Go Binary. Same Hermes Brain.",
		"A static Go binary that talks to your Hermes backend over HTTP.",
		"Why a Go layer matters.",
		"Boots Like a Tool",
		"In-Process Tool Loop",
		"Survives Dropped Streams",
		// Older revisions that buried the first-screen hierarchy.
		"Gormes is a Go-native rewrite of Hermes Agent — built to solve the operations problem, not the AI problem.",
		"Gormes is a Go-native runtime for AI agents — built to fix",
		"Why Hermes-stack agents break in production.",
		"Rerun the installer to update the managed Gormes checkout.",
		"Source-backed for now →",
		"not production-ready yet",
		// Obsolete single-row ledger copy replaced by grouped roadmap
		"Phase 3 — SQLite + FTS5 transcript memory.",
		"Phase 3.A–C — SQLite + FTS5 lattice, ontological graph, neural recall.",
		"Phase 4 — Native prompt building + agent orchestration.",
		"Phase 4 — Brain transplant. Hermes backend becomes optional.",
		"Phase 5 — 100% Go. Python tool scripts ported. Hermes-off.",
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

	// There should be exactly 7 roadmap phase blocks — not a specific
	// phase-name assertion, just that the roadmap actually renders the
	// full phase set from progress.json.
	if n := strings.Count(text, `class="roadmap-phase"`); n != 7 {
		t.Errorf("roadmap phase count = %d, want 7", n)
	}

	// The progress tracker label follows a "N/M shipped" shape driven
	// by progress.json Stats(). We assert the shape, not the numbers.
	trackerRE := regexp.MustCompile(`\d+/\d+ shipped`)
	if !trackerRE.MatchString(text) {
		t.Errorf("missing N/M shipped progress tracker label; body:\n%s", text)
	}
}

func TestEmbeddedTemplates_ArePresentAndParse(t *testing.T) {
	files := []string{
		"templates/layout.tmpl",
		"templates/index.tmpl",
		"templates/partials/install_step.tmpl",
		"templates/partials/feature_card.tmpl",
		"templates/partials/roadmap_phase.tmpl",
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

	for _, want := range []string{"layout", "index", "install_step", "feature_card", "roadmap_phase"} {
		if templates.Lookup(want) == nil {
			t.Fatalf("parsed templates missing %q", want)
		}
	}
}
