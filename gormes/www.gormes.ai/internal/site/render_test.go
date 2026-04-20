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
		// Shipping ledger
		"SHIPPING STATE",
		"What ships now, what doesn&#39;t.",
		"Phase 1 — Bubble Tea TUI shell.",
		"Phase 2.A–C — Tool registry + Telegram adapter + session resume.",
		"Phase 2.B.2+ — Wider gateway (Discord, Slack, more adapters).",
		"Phase 3.A–C + 3.D.5 — SQLite + FTS5 lattice, ontological graph, neural recall, USER.md mirror.",
		"Phase 3.D — Ollama embeddings + semantic fusion.",
		"Phase 4 — Brain transplant. Hermes backend becomes optional.",
		"Phase 5 — 100% Go. Python tool scripts ported. Hermes-off.",
		// Footer
		"Gormes v0.1.0 · TrebuchetDynamics",
		"MIT License · 2026",
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
		"Phase 3 — SQLite + FTS5 transcript memory.",
		"Phase 3.A–C — SQLite + FTS5 lattice, ontological graph, neural recall.",
		"Phase 4 — Native prompt building + agent orchestration.",
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
