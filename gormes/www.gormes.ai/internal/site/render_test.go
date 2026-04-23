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
		// Hero
		"OPEN SOURCE · MIT LICENSE",
		"One Go Binary. Same Hermes Brain.",
		"A static Go binary that talks to your Hermes backend over HTTP.",
		// Install
		"1. INSTALL",
		"curl -fsSL https://gormes.ai/install.sh | sh",
		"2. RUN",
		"Requires Hermes backend at localhost:8642.",
		"Install Hermes →",
		// Copy button (clipboard JS is allowed for this widget only)
		`class="copy-btn"`,
		"navigator.clipboard.writeText",
		// Features
		"FEATURES",
		"Why a Go layer matters.",
		"Single Static Binary",
		"Boots Like a Tool",
		"In-Process Tool Loop",
		"Survives Dropped Streams",
		"Route-B reconnect treats SSE drops",
		// Roadmap section — structural checks only. The roadmap itself
		// is now driven by progress.json; asserting exact counts or
		// item names here would re-introduce the very drift we just
		// eliminated. We check for tones and structure instead.
		"SHIPPING STATE",
		"What ships now, what doesn&#39;t.",
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
		"roadmap-status-later",
		// Complete work still appears at item level even when no whole phase is complete.
		"roadmap-item-shipped",
		// Structural class anchors
		"roadmap-phase",
		// Footer — brand text + company anchor + license
		`Gormes v0.1.0 · <a href="https://trebuchetdynamics.com/">TrebuchetDynamics</a>`,
		"MIT License · 2026",
		// Nav now includes Company link at the trebuchetdynamics.com URL
		`<a href="https://trebuchetdynamics.com/">Company</a>`,
		// Nav link + in-page note pointing at the Hugo docs site
		`<a href="https://docs.gormes.ai/">Docs</a>`,
		"Deeper reference material lives at",
		`<a href="https://docs.gormes.ai/">docs.gormes.ai →</a>`,
		// CSS link
		`href="/static/site.css"`,
	}
	rejects := []string{
		"Run Hermes Through a Go Operator Console.",
		"Hermes, In a Single Static Binary.",
		"No Python runtime on the host",
		"Boot Sequence",
		"Proof Rail",
		"01 / INSTALL HERMES",
		"Why Hermes users switch",
		"Inspect the Machine",
		"~8 MB",
		"~12 MB",
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

	// There should be exactly 6 roadmap phase blocks — not a specific
	// phase-name assertion, just that the roadmap actually renders the
	// full phase set from progress.json.
	if n := strings.Count(text, `class="roadmap-phase"`); n != 6 {
		t.Errorf("roadmap phase count = %d, want 6", n)
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
