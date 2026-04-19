package docs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var staleGormesIORefPattern = regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?gormes\.io`)

func TestAICutoverDocsExistAndCarryExpectedTitles(t *testing.T) {
	spec := readDoc(t, "superpowers/specs/2026-04-19-gormes-ai-cutover-design.md")
	if !strings.Contains(spec, "Gormes.ai Hard Cutover Design Spec") {
		t.Fatalf("cutover spec missing its title")
	}

	plan := readDoc(t, "superpowers/plans/2026-04-19-gormes-ai-cutover.md")
	if !strings.Contains(plan, "Gormes.ai Hard Cutover Implementation Plan") {
		t.Fatalf("cutover plan missing its title")
	}
}

func TestLandingPageDesignDocDescribesCurrentGormesAIModule(t *testing.T) {
	raw := readDoc(t, "superpowers/specs/2026-04-19-gormes-landing-page-design.md")
	wants := []string{
		"# Gormes.ai Landing Page Design Spec",
		"`www.gormes.ai`",
		"The Agent That GOes With You.",
		"Phase 1 uses your existing Hermes backend.",
		"Pure single-binary Go arrives later in the roadmap.",
		"Visual Direction",
		"Roadmap Block",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("design doc is missing %q", want)
		}
	}

	if hasStaleGormesIOReference(raw) {
		t.Fatalf("design doc should not contain stale .io identity")
	}
}

func TestLandingPagePlanDocDocumentsCurrentAICutoverImplementation(t *testing.T) {
	raw := readDoc(t, "superpowers/plans/2026-04-19-gormes-landing-page.md")
	wants := []string{
		"# Gormes.ai Landing Page Implementation Plan",
		"www.gormes.ai/internal/site/assets.go",
		"www.gormes.ai/internal/site/content.go",
		"www.gormes.ai/internal/site/server.go",
		"www.gormes.ai/internal/site/templates/*.tmpl",
		"www.gormes.ai/internal/site/static/*",
		"www.gormes.ai/README.md",
		"www.gormes.ai/tests/home.spec.mjs",
		"cd gormes && go test ./docs",
		"npm run test:e2e",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("landing-page plan doc for the upcoming .ai cutover is missing %q", want)
		}
	}

	if hasStaleGormesIOReference(raw) {
		t.Fatalf("landing-page plan doc should not contain stale .io identity")
	}

	for _, rel := range []string{
		"../../www.gormes.ai/internal/site/assets.go",
		"../../www.gormes.ai/internal/site/content.go",
		"../../www.gormes.ai/internal/site/server.go",
		"../../www.gormes.ai/internal/site/templates/index.tmpl",
		"../../www.gormes.ai/internal/site/templates/layout.tmpl",
		"../../www.gormes.ai/internal/site/templates/partials/code_block.tmpl",
		"../../www.gormes.ai/internal/site/templates/partials/feature_card.tmpl",
		"../../www.gormes.ai/internal/site/templates/partials/phase_item.tmpl",
		"../../www.gormes.ai/internal/site/static/site.css",
		"../../www.gormes.ai/tests/home.spec.mjs",
	} {
		if _, err := os.Stat(filepath.Join(".", rel)); err != nil {
			t.Fatalf("expected implementation file %s to exist: %v", rel, err)
		}
	}

	script := readPlaywrightE2EScript(t, "../../www.gormes.ai/package.json")
	if !strings.Contains(script, "playwright") {
		t.Fatalf("www.gormes.ai package.json test:e2e script should reference Playwright")
	}
}

func readDoc(t *testing.T, rel string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(".", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(raw)
}

func readPlaywrightE2EScript(t *testing.T, rel string) string {
	t.Helper()

	type packageJSON struct {
		Scripts map[string]string `json:"scripts"`
	}

	var pkg packageJSON
	if err := json.Unmarshal([]byte(readDoc(t, rel)), &pkg); err != nil {
		t.Fatalf("parse %s as json: %v", rel, err)
	}

	script, ok := pkg.Scripts["test:e2e"]
	if !ok {
		t.Fatalf("%s does not define scripts.test:e2e", rel)
	}

	return script
}

func hasStaleGormesIOReference(raw string) bool {
	return staleGormesIORefPattern.FindStringIndex(raw) != nil
}
