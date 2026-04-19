package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var targets = []string{
	"superpowers/specs/2026-04-18-gormes-frontend-adapter-design.md",
	"superpowers/plans/2026-04-18-gormes-phase1-frontend-adapter.md",
	"superpowers/specs/2026-04-19-gormes-landing-page-design.md",
	"superpowers/plans/2026-04-19-gormes-landing-page.md",
	"superpowers/specs/2026-04-19-gormes-ai-cutover-design.md",
	"superpowers/plans/2026-04-19-gormes-ai-cutover.md",
}

func TestTargetsIncludeLandingPageDocs(t *testing.T) {
	want := map[string]bool{
		"superpowers/specs/2026-04-19-gormes-landing-page-design.md": false,
		"superpowers/plans/2026-04-19-gormes-landing-page.md":        false,
	}

	for _, target := range targets {
		if _, ok := want[target]; ok {
			want[target] = true
		}
	}

	for rel, seen := range want {
		if !seen {
			t.Fatalf("docs target missing %s", rel)
		}
	}
}

func TestTargetsIncludeAICutoverDocs(t *testing.T) {
	want := map[string]bool{
		"superpowers/specs/2026-04-19-gormes-ai-cutover-design.md": false,
		"superpowers/plans/2026-04-19-gormes-ai-cutover.md":        false,
	}

	for _, target := range targets {
		if _, ok := want[target]; ok {
			want[target] = true
		}
	}

	for rel, seen := range want {
		if !seen {
			t.Fatalf("docs target missing %s", rel)
		}
	}
}

func TestLandingPageDesignDocCoversApprovedStory(t *testing.T) {
	raw := readDoc(t, "superpowers/specs/2026-04-19-gormes-landing-page-design.md")
	wants := []string{
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
}

func TestLandingPagePlanDocReferencesRealImplementationFilesAndCommands(t *testing.T) {
	raw := readDoc(t, "superpowers/plans/2026-04-19-gormes-landing-page.md")
	wants := []string{
		"www.gormes.ai/internal/site/assets.go",
		"www.gormes.ai/internal/site/content.go",
		"www.gormes.ai/internal/site/server.go",
		"www.gormes.ai/internal/site/templates/*.tmpl",
		"www.gormes.ai/internal/site/static/*",
		"www.gormes.ai/tests/home.spec.mjs",
		"cd gormes && go test ./docs",
		"npm run test:e2e",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("plan doc is missing %q", want)
		}
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

	pkgJSON := readDoc(t, "../../www.gormes.ai/package.json")
	if !strings.Contains(pkgJSON, `"test:e2e": "playwright test --project=chromium"`) {
		t.Fatalf("www.gormes.ai package.json does not define the documented test:e2e script")
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
