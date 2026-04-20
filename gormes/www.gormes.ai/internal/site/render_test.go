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
		// Roadmap section
		"SHIPPING STATE",
		"What ships now, what doesn&#39;t.",
		// Phase 1 header + items
		"SHIPPED · EVOLVING",
		"Phase 1 — Dashboard",
		"Bubble Tea TUI shell",
		"16 ms render mailbox",
		"Route-B SSE reconnect",
		"Wire Doctor",
		"Streaming token renderer",
		"Ongoing: polish, bug fixes, TUI ergonomics",
		// Phase 2 header + items
		"IN PROGRESS · 3/7",
		"Phase 2 — Gateway",
		"2.A Go-native tool registry + kernel tool loop",
		"2.B.1 Telegram adapter",
		"2.C Thin session persistence (bbolt)",
		"2.B.2+ Wider platforms (23 upstream connectors queued)",
		"2.D Cron / scheduled automations",
		"2.E Subagent delegation",
		"2.F Hooks + lifecycle",
		// Phase 3 header + items
		"IN PROGRESS · 4/5",
		"Phase 3 — Memory",
		"3.A SQLite + FTS5 lattice",
		"3.B Ontological graph + LLM extractor",
		"3.C Neural recall + context injection",
		"3.D.5 USER.md mirror — Gormes-original, no upstream equivalent",
		"3.D Ollama embeddings + semantic fusion",
		// Phase 4 header + subtitle + items
		"PLANNED · 0/8",
		"Phase 4 — Brain Transplant",
		"Ships hermes-off after 4.A–4.D. Backend becomes optional.",
		"4.A Provider adapters",
		"4.B Context engine + compression",
		"4.C Native Go prompt builder",
		"4.D Smart model routing",
		"4.E Trajectory + insights",
		"4.F Title generation",
		"4.G Credentials + OAuth flows",
		"4.H Rate / retry / prompt caching",
		// Phase 5 header + subtitle + collapsed row
		"LATER · 0/16",
		"Phase 5 — Final Purge (100% Go)",
		"Delete the last Python dependency. Ship entirely in Go.",
		"5.A–5.P",
		"See ARCH_PLAN §7.",
		// Footer — brand text + company anchor + license
		`Gormes v0.1.0 · <a href="https://trebuchetdynamics.com/">TrebuchetDynamics</a>`,
		"MIT License · 2026",
		// Nav now includes Company link at the trebuchetdynamics.com URL
		`<a href="https://trebuchetdynamics.com/">Company</a>`,
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
