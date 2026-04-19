package site

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_RendersApprovedPhase1Story(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	wants := []string{
		"The Agent That GOes With You.",
		"Open Source • MIT License • Phase 1 Go Port",
		"API_SERVER_ENABLED=true hermes gateway start",
		"./bin/gormes",
		"Phase 1 uses your existing Hermes backend.",
		"The Port Is Already Moving",
		"Help Finish the Port",
		"Same agent. Same memory. Same workflows.",
	}

	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page missing %q\nbody:\n%s", want, body)
		}
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
