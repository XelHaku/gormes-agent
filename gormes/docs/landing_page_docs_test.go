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

func TestTargetsIncludeManifestoSyncDocs(t *testing.T) {
	want := map[string]bool{
		"superpowers/specs/2026-04-19-gormes-doc-sync-manifesto-design.md": false,
		"superpowers/plans/2026-04-19-gormes-doc-sync-manifesto.md":        false,
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

func TestDocsHarnessAllowsNativeGormesManifestoPage(t *testing.T) {
	if _, ok := nativeHugoPages["why-gormes.md"]; !ok {
		t.Fatalf("nativeHugoPages should explicitly allow why-gormes.md")
	}
	if len(nativeHugoPages) != 1 {
		extras := make([]string, 0, len(nativeHugoPages)-1)
		for page := range nativeHugoPages {
			if page != "why-gormes.md" {
				extras = append(extras, page)
			}
		}
		t.Fatalf("nativeHugoPages must contain only why-gormes.md; got %d entries with extras: %v", len(nativeHugoPages), extras)
	}
}

func TestDocsHomePageIsGormesBranded(t *testing.T) {
	raw := readDoc(t, "content/_index.md")
	wants := []string{
		`title: "Gormes Documentation"`,
		"# Gormes",
		"[Why Gormes](why-gormes)",
		"Route-B reconnect",
		"Quick Start on GitHub",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("docs home is missing %q", want)
		}
	}

	rejects := []string{
		"Hermes Agent Documentation",
		"# Hermes Agent",
		"The self-improving AI agent built by Nous Research.",
	}
	for _, reject := range rejects {
		if strings.Contains(raw, reject) {
			t.Fatalf("docs home should not contain stale copy %q", reject)
		}
	}
}

func TestWhyGormesManifestoPageExistsAndCarriesApprovedSections(t *testing.T) {
	raw := readDoc(t, "content/why-gormes.md")
	wants := []string{
		`title: "Why Gormes"`,
		"## Operational Moat",
		"## Wire Doctor",
		"## Chaos Resilience",
		"## Surgical Architecture",
		"thundering herd",
		"Tool Registry",
		"Telegram Scout",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("why-gormes page is missing %q", want)
		}
	}
}

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

func TestSpecIndexAndPhase2SpecsCrossLink(t *testing.T) {
	indexRaw := readDoc(t, "superpowers/specs/README.md")
	indexWants := []string{
		"# Gormes Specs Index",
		"2026-04-19-gormes-doc-sync-manifesto-design.md",
		"2026-04-19-gormes-phase2-tools-design.md",
		"2026-04-19-gormes-phase2b-telegram.md",
		"../../ARCH_PLAN.md",
	}
	for _, want := range indexWants {
		if !strings.Contains(indexRaw, want) {
			t.Fatalf("spec index is missing %q", want)
		}
	}

	toolsRaw := readDoc(t, "superpowers/specs/2026-04-19-gormes-phase2-tools-design.md")
	telegramRaw := readDoc(t, "superpowers/specs/2026-04-19-gormes-phase2b-telegram.md")
	for _, raw := range []string{toolsRaw, telegramRaw} {
		for _, want := range []string{
			"## Related Documents",
			"../../ARCH_PLAN.md",
			"README.md",
		} {
			if !strings.Contains(raw, want) {
				t.Fatalf("phase-2 spec is missing %q", want)
			}
		}
	}

	if !strings.Contains(toolsRaw, "2026-04-19-gormes-phase2b-telegram.md") {
		t.Fatalf("phase2 tools spec should link to the telegram spec")
	}
	if !strings.Contains(telegramRaw, "2026-04-19-gormes-phase2-tools-design.md") {
		t.Fatalf("telegram spec should link to the tool registry spec")
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

func TestReadmeDocumentsDoctorAndArchitecturalEdge(t *testing.T) {
	raw := readDoc(t, "../README.md")
	wants := []string{
		"# Gormes",
		"7.9 MB static binary",
		"Go 1.22+",
		"Zero-dependencies inside the process boundary",
		"./bin/gormes doctor --offline",
		"Route-B reconnect",
		"16 ms coalescing mailbox",
		"[Why Gormes](docs/content/why-gormes.md)",
		"[Phase 2.A — Tool Registry](docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md)",
		"[Phase 2.B.1 — Telegram Scout](docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md)",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("README is missing %q", want)
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
