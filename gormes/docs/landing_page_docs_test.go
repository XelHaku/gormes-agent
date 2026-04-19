package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestLandingPagePlanDocCoversImplementationReality(t *testing.T) {
	raw := readDoc(t, "superpowers/plans/2026-04-19-gormes-landing-page.md")
	wants := []string{
		"www.gormes.io/internal/site",
		"//go:embed",
		"Playwright smoke testing",
		"www.gormes.io/tests/home.spec.mjs",
		"npm run test:e2e",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("plan doc is missing %q", want)
		}
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
