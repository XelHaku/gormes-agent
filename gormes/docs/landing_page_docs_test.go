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

func TestTargetsIncludePhase2CPersistenceDocs(t *testing.T) {
	want := map[string]bool{
		"superpowers/specs/2026-04-19-gormes-phase2c-persistence-design.md": false,
		"superpowers/plans/2026-04-19-gormes-phase2c-persistence.md":        false,
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
	// Native Gormes pages live under why-gormes.md OR the building-gormes/
	// (contributor-facing) and using-gormes/ (operator-facing) sections.
	// Everything else is a mirrored upstream doc under upstream-hermes/.
	for page := range nativeHugoPages {
		if page == "_index.md" || page == "why-gormes.md" {
			continue
		}
		if strings.HasPrefix(page, "building-gormes/") || strings.HasPrefix(page, "using-gormes/") {
			continue
		}
		t.Fatalf("nativeHugoPages contains unexpected entry (must be _index.md, why-gormes.md, or under building-gormes/ or using-gormes/): %q", page)
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
		"2026-04-19-gormes-phase2c-persistence-design.md",
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

func TestArchPlanTracksShippedPhase2Ledger(t *testing.T) {
	// ARCH_PLAN.md is now a stub (Task 2 of docs redesign).
	// Content has moved to content/building-gormes/architecture_plan/.
	stub := readDoc(t, "ARCH_PLAN.md")
	if !strings.Contains(stub, "content/building-gormes/architecture_plan/") {
		t.Fatalf("ARCH_PLAN.md stub should reference the new split location")
	}

	// Verify the Phase 2 ledger content lives in the split page.
	phase2 := readDoc(t, "content/building-gormes/architecture_plan/phase-2-gateway.md")
	wants := []string{
		"| Phase 2.A — Tool Registry | ✅ complete |",
		"| Phase 2.B.1 — Telegram Scout | ✅ complete |",
		"| Phase 2.C — Thin Mapping Persistence | ✅ complete |",
		"Phase 2.C is intentionally not Phase 3.",
		"Python still owns transcript memory",
	}
	for _, want := range wants {
		if !strings.Contains(phase2, want) {
			t.Fatalf("phase-2-gateway.md is missing %q", want)
		}
	}

	// Verify the roadmap overview lives in the _index.
	index := readDoc(t, "content/building-gormes/architecture_plan/_index.md")
	indexWants := []string{
		"**Public site:** https://gormes.ai",
		"Phase 3 — The Black Box (Memory)",
	}
	for _, want := range indexWants {
		if !strings.Contains(index, want) {
			t.Fatalf("architecture_plan/_index.md is missing %q", want)
		}
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
		"gormes/www.gormes.ai/internal/site/assets.go",
		"gormes/www.gormes.ai/internal/site/content.go",
		"gormes/www.gormes.ai/internal/site/server.go",
		"gormes/www.gormes.ai/internal/site/templates/*.tmpl",
		"gormes/www.gormes.ai/internal/site/static/*",
		"gormes/www.gormes.ai/README.md",
		"gormes/www.gormes.ai/tests/home.spec.mjs",
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
		"../www.gormes.ai/internal/site/assets.go",
		"../www.gormes.ai/internal/site/content.go",
		"../www.gormes.ai/internal/site/server.go",
		"../www.gormes.ai/internal/site/templates/index.tmpl",
		"../www.gormes.ai/internal/site/templates/layout.tmpl",
		"../www.gormes.ai/internal/site/templates/partials/feature_card.tmpl",
		"../www.gormes.ai/internal/site/templates/partials/install_step.tmpl",
		"../www.gormes.ai/internal/site/templates/partials/roadmap_phase.tmpl",
		"../www.gormes.ai/internal/site/static/site.css",
		"../www.gormes.ai/tests/home.spec.mjs",
	} {
		if _, err := os.Stat(filepath.Join(".", rel)); err != nil {
			t.Fatalf("expected implementation file %s to exist: %v", rel, err)
		}
	}

	script := readPlaywrightE2EScript(t, "../www.gormes.ai/package.json")
	if !strings.Contains(script, "playwright") {
		t.Fatalf("www.gormes.ai package.json test:e2e script should reference Playwright")
	}
}

func TestReadmeDocumentsDoctorAndArchitecturalEdge(t *testing.T) {
	raw := readDoc(t, "../README.md")
	// Assert stable README invariants only — things that should not drift
	// across phase boundaries. Binary size, Go minor version, and specific
	// architectural terms (coalescing mailbox ms, Route-B) are too brittle;
	// they belong in docs/ARCH_PLAN.md or per-phase specs, not README.
	// Asserts stable invariants of gormes/README.md (the technical docs),
	// NOT the repo-root README.md (the marketing landing page).
	// Brittle assertions removed:
	//   - "7.9 MB static binary" / specific sizes — drifts with each phase
	//   - "Go 1.22+" — drifts with Go toolchain bumps (now 1.25+)
	//   - Specific phase spec links — drift with phase progression
	wants := []string{
		"# Gormes",
		"single static binary",
		"zero-CGO",
		"Go 1.25+",
		"./bin/gormes doctor --offline",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("README is missing %q", want)
		}
	}
}

func TestArchitectureIndexCrossLinksPrePhase4Gate(t *testing.T) {
	raw := readDoc(t, "content/building-gormes/architecture_plan/_index.md")
	wants := []string{
		"## Phase 4 Entry Gate",
		"Pre-Phase-4 E2E Gate",
		"phase-3-memory",
		"phase-4-brain-transplant",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("architecture index is missing %q", want)
		}
	}
}

func TestGatewayCoreDocReflectsSharedPhase2Surface(t *testing.T) {
	raw := readDoc(t, "content/building-gormes/core-systems/gateway.md")
	wants := []string{
		"Discord adapter",
		"Slack Socket Mode bot",
		"slash-command registry",
		"SessionContext prompt injection",
		"BOOT.md startup hook",
		"Gateway stream consumer",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("gateway core-systems doc is missing %q", want)
		}
	}
}

func TestMemoryDocsReflectShippedOperatorSurfacesAndGonchoNaming(t *testing.T) {
	core := readDoc(t, "content/building-gormes/core-systems/memory.md")
	for _, want := range []string{
		"Tool audit JSONL",
		"Transcript export",
		"gormes memory status",
	} {
		if !strings.Contains(core, want) {
			t.Fatalf("memory core-systems doc is missing %q", want)
		}
	}

	phase3 := readDoc(t, "content/building-gormes/architecture_plan/phase-3-memory.md")
	for _, want := range []string{
		"GONCHO",
		"Honcho-compatible interfaces",
		"| 3.E.2 — Tool Execution Audit Log | ✅ shipped | P0 |",
		"| 3.E.3 — Transcript Export Command | ✅ shipped | P2 |",
		"| 3.E.4 — Extraction State Visibility | ✅ shipped | P1 |",
	} {
		if !strings.Contains(phase3, want) {
			t.Fatalf("phase-3-memory doc is missing %q", want)
		}
	}
}

func TestMirrorStrategyReflectsShippedAuditAndOperatorSurfaces(t *testing.T) {
	raw := readDoc(t, "content/building-gormes/architecture_plan/mirror-strategy.md")
	wants := []string{
		"✅ **Shipped in Gormes (3.E.2)**",
		"✅ **Shipped in Gormes (3.E.3)**",
		"Phase 2.D",
		"Phase 2.G",
		"Phase 2.F.2",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("mirror strategy doc is missing %q", want)
		}
	}
	for _, reject := range []string{
		"Gap: no tool audit trail",
		"Cron not yet implemented in Gormes (Phase 4)",
		"Skills not yet implemented (Phase 5)",
		"Boot hooks not yet implemented",
	} {
		if strings.Contains(raw, reject) {
			t.Fatalf("mirror strategy doc still contains stale claim %q", reject)
		}
	}
}

func TestSubsystemInventoryReflectsShippedPhase2AndPhase3Reality(t *testing.T) {
	raw := readDoc(t, "content/building-gormes/architecture_plan/subsystem-inventory.md")
	wants := []string{
		"typed session-context prompt injection landed",
		"typed delivery-target parsing landed",
		"deterministic frame fan-out landed",
		"Tool execution audit log | None (exceeds Hermes) | 3.E.2 | ✅ shipped",
		"Transcript export command | None (exceeds Hermes; Hermes has no text export) | 3.E.3 | ✅ shipped",
		"Extraction state visibility | None (debug visibility) | 3.E.4 | ✅ shipped",
		"Memory decay | None (Gormes-original) | 3.E.6 | ✅ shipped",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("subsystem inventory is missing %q", want)
		}
	}
}

func TestPhase3IdentityLineagePlanIsLinkedAndContractsAreDocumented(t *testing.T) {
	plan := readDoc(t, "superpowers/plans/2026-04-22-gormes-phase3-identity-lineage-plan.md")
	for _, want := range []string{
		"# Phase 3 Identity + Lineage Implementation Plan",
		"user_id > chat_id > session_id",
		"same-chat default",
		"opt-in cross-chat",
		"parent_session_id",
		"source-filtered session search",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("identity/lineage plan is missing %q", want)
		}
	}

	index := readDoc(t, "content/building-gormes/architecture_plan/_index.md")
	if !strings.Contains(index, "2026-04-22-gormes-phase3-identity-lineage-plan") {
		t.Fatalf("architecture index is missing the phase 3 identity/lineage plan link")
	}

	phase3 := readDoc(t, "content/building-gormes/architecture_plan/phase-3-memory.md")
	for _, want := range []string{
		"2026-04-22-gormes-phase3-identity-lineage-plan",
		"user_id > chat_id > session_id",
		"same-chat default, opt-in cross-chat",
		"parent_session_id",
	} {
		if !strings.Contains(phase3, want) {
			t.Fatalf("phase-3-memory doc is missing %q", want)
		}
	}

	core := readDoc(t, "content/building-gormes/core-systems/memory.md")
	for _, want := range []string{
		"GONCHO identity hierarchy",
		"same-chat by default",
		"parent_session_id",
	} {
		if !strings.Contains(core, want) {
			t.Fatalf("memory core-systems doc is missing %q", want)
		}
	}
}

func TestPhase3IdentityLineageExecutionPlanIsLinkedAndSequenced(t *testing.T) {
	plan := readDoc(t, "superpowers/plans/2026-04-22-gormes-phase3-identity-lineage-execution-plan.md")
	for _, want := range []string{
		"# Phase 3 Identity + Lineage Execution Plan",
		"Relationship last_seen tracking",
		"Cross-chat entity merge + recall fence",
		"parent_session_id lineage for compression splits",
		"Source-filtered FTS/session search across chats",
		"same-chat default",
		"opt-in cross-chat",
		"Rollback and failure containment",
		"go test ./internal/memory ./internal/session ./internal/goncho -count=1",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("identity/lineage execution plan is missing %q", want)
		}
	}

	index := readDoc(t, "content/building-gormes/architecture_plan/_index.md")
	if !strings.Contains(index, "2026-04-22-gormes-phase3-identity-lineage-execution-plan") {
		t.Fatalf("architecture index is missing the phase 3 identity/lineage execution plan link")
	}

	phase3 := readDoc(t, "content/building-gormes/architecture_plan/phase-3-memory.md")
	for _, want := range []string{
		"2026-04-22-gormes-phase3-identity-lineage-execution-plan",
		"3.E.7 schema exposure -> 3.E.7 deny-path fixtures -> 3.E.7 operator evidence -> 3.E.8 parent_session_id -> 3.E.8 lineage-aware hits/evidence",
	} {
		if !strings.Contains(phase3, want) {
			t.Fatalf("phase-3-memory doc is missing %q", want)
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
