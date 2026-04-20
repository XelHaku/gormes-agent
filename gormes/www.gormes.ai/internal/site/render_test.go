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

	if !bytes.Contains(body, []byte(`href="/static/site.css"`)) {
		t.Fatalf("rendered page missing stylesheet link\n%s", text)
	}
}

func TestEmbeddedTemplates_ArePresentAndParse(t *testing.T) {
	files := []string{
		"templates/layout.tmpl",
		"templates/index.tmpl",
		"templates/partials/code_block.tmpl",
		"templates/partials/feature_card.tmpl",
		"templates/partials/phase_item.tmpl",
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

	for _, want := range []string{"layout", "index", "code_block", "feature_card", "phase_item"} {
		if templates.Lookup(want) == nil {
			t.Fatalf("parsed templates missing %q", want)
		}
	}
}
