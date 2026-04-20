package site

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
)

func TestRenderIndex_RendersOperatorConsoleTruth(t *testing.T) {
	body, err := RenderIndex()
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}

	text := string(body)
	wants := []string{
		"Run Hermes Through a Go Operator Console.",
		"Install Hermes fast. Then boot Gormes.",
		"Why Hermes users switch",
		"Shipping State, Not Wishcasting",
		"Inspect the Machine",
		"curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash",
		"Works on Linux, macOS, WSL2, and Android via Termux.",
		"Windows: Native Windows is not supported. Please install WSL2",
		"source ~/.bashrc    # reload shell (or: source ~/.zshrc)",
		"./bin/gormes doctor --offline",
		`class="hero hero-deck"`,
		`id="proof"`,
		`class="activation-grid"`,
		`class="ops-section ops-grid"`,
		`id="features-title"`,
		`class="shipping-ledger"`,
		`class="ship-state-list"`,
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

	if !bytes.Contains(body, []byte(`href="/static/site.css"`)) {
		t.Fatalf("rendered page missing stylesheet link\n%s", text)
	}
}

func TestEmbeddedTemplates_ArePresentAndParse(t *testing.T) {
	files := []string{
		"templates/layout.tmpl",
		"templates/index.tmpl",
		"templates/partials/command_step.tmpl",
		"templates/partials/ops_module.tmpl",
		"templates/partials/proof_stat.tmpl",
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

	for _, want := range []string{"layout", "index", "command_step", "ops_module", "proof_stat", "ship_state"} {
		if templates.Lookup(want) == nil {
			t.Fatalf("parsed templates missing %q", want)
		}
	}
}
