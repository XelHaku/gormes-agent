package docs_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

const (
	sourceDocsRoot  = "../../website/docs"
	hugoContentRoot = "./content"
)

var (
	bannedPatterns = []struct {
		name    string
		pattern *regexp.Regexp
	}{
		{"docusaurus-admonition", regexp.MustCompile(`(?m)^:::`)},
		{"jsx-comment", regexp.MustCompile(`\{/\*.*\*/\}`)},
		{"jsx-class-name", regexp.MustCompile(`className=`)},
		{"root-relative-link", regexp.MustCompile(`\]\(/[^)]+\)`)},
		{"raw-react-component", regexp.MustCompile(`(?m)^<[A-Z][A-Za-z0-9]*(?:\s|/|>)`)},
	}
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
)

var (
	gatewayDonorMapRequiredHeadings = []string{
		"## Status",
		"## Why This Adapter Is Reusable",
		"## Picoclaw Donor Files",
		"## What To Copy vs What To Rebuild",
		"## Gormes Mapping",
		"## Implementation Notes",
		"## Risks / Mismatches",
		"## Port Order Recommendation",
		"## Code References",
	}
	gatewayDonorMapAllowedRecommendations = map[string]struct{}{
		"copy candidate":     {},
		"adapt pattern only": {},
		"not worth reusing":  {},
	}
	gatewayDonorMapPinnedProvenance = []string{
		"<picoclaw donor repo>",
		"6421f146a99df1bebcd4b1ca8de2a289dfca3622",
		"https://github.com/sipeed/picoclaw",
		"relative to that donor root, not relative to the Gormes repo",
	}
	gatewayDonorMapHubRowPattern         = regexp.MustCompile(`(?m)^\| ([^|]+) \| ` + "`" + `([^` + "`" + `]+)` + "`" + ` \| [^|]+ \| \[([^\]]+)\]\(\./([^/]+)/\) \|$`)
	gatewayDonorMapRecommendationPattern = regexp.MustCompile("Recommendation: `([^`]+)`\\.")
)

var targets = []string{
	"ARCH_PLAN.md",
	"THEORETICAL_ADVANTAGES_GORMES_HERMES.md",
	"superpowers/specs/2026-04-18-gormes-frontend-adapter-design.md",
	"superpowers/plans/2026-04-18-gormes-phase1-frontend-adapter.md",
	"superpowers/specs/2026-04-19-gormes-landing-page-design.md",
	"superpowers/plans/2026-04-19-gormes-landing-page.md",
	"superpowers/specs/2026-04-19-gormes-ai-cutover-design.md",
	"superpowers/plans/2026-04-19-gormes-ai-cutover.md",
	"superpowers/specs/2026-04-19-gormes-doc-sync-manifesto-design.md",
	"superpowers/plans/2026-04-19-gormes-doc-sync-manifesto.md",
	"superpowers/specs/2026-04-19-gormes-phase2c-persistence-design.md",
	"superpowers/plans/2026-04-19-gormes-phase2c-persistence.md",
}

var nativeHugoPages = map[string]struct{}{
	"_index.md":                                                      {},
	"why-gormes.md":                                                  {},
	"building-gormes/_index.md":                                      {},
	"building-gormes/contract-readiness.md":                          {},
	"building-gormes/autoloop/_index.md":                             {},
	"building-gormes/autoloop/autoloop-handoff.md":                   {},
	"building-gormes/autoloop/agent-queue.md":                        {},
	"building-gormes/autoloop/next-slices.md":                        {},
	"building-gormes/autoloop/blocked-slices.md":                     {},
	"building-gormes/autoloop/umbrella-cleanup.md":                   {},
	"building-gormes/autoloop/progress-schema.md":                    {},
	"building-gormes/upstream-lessons.md":                            {},
	"building-gormes/gateway-donor-map/_index.md":                    {},
	"building-gormes/gateway-donor-map/shared-adapter-patterns.md":   {},
	"building-gormes/porting-a-subsystem.md":                         {},
	"building-gormes/testing.md":                                     {},
	"building-gormes/what-hermes-gets-wrong.md":                      {},
	"building-gormes/gateway-donor-map/telegram.md":                  {},
	"building-gormes/gateway-donor-map/discord.md":                   {},
	"building-gormes/gateway-donor-map/slack.md":                     {},
	"building-gormes/gateway-donor-map/whatsapp.md":                  {},
	"building-gormes/gateway-donor-map/matrix.md":                    {},
	"building-gormes/gateway-donor-map/irc.md":                       {},
	"building-gormes/gateway-donor-map/line.md":                      {},
	"building-gormes/gateway-donor-map/onebot.md":                    {},
	"building-gormes/gateway-donor-map/qq.md":                        {},
	"building-gormes/gateway-donor-map/wecom.md":                     {},
	"building-gormes/gateway-donor-map/weixin.md":                    {},
	"building-gormes/gateway-donor-map/feishu.md":                    {},
	"building-gormes/gateway-donor-map/dingtalk.md":                  {},
	"building-gormes/gateway-donor-map/vk.md":                        {},
	"building-gormes/gateway-donor-map/webhook.md":                   {},
	"building-gormes/goncho_honcho_memory/_index.md":                 {},
	"building-gormes/goncho_honcho_memory/01-prompts.md":             {},
	"building-gormes/goncho_honcho_memory/02-tool-schemas.md":        {},
	"building-gormes/architecture_plan/_index.md":                    {},
	"building-gormes/architecture_plan/phase-1-dashboard.md":         {},
	"building-gormes/architecture_plan/phase-2-gateway.md":           {},
	"building-gormes/architecture_plan/phase-3-memory.md":            {},
	"building-gormes/architecture_plan/phase-4-brain-transplant.md":  {},
	"building-gormes/architecture_plan/phase-5-final-purge.md":       {},
	"building-gormes/architecture_plan/phase-6-learning-loop.md":     {},
	"building-gormes/architecture_plan/subsystem-inventory.md":       {},
	"building-gormes/architecture_plan/mirror-strategy.md":           {},
	"building-gormes/architecture_plan/technology-radar.md":          {},
	"building-gormes/architecture_plan/procfile-process-managers.md": {},
	"building-gormes/architecture_plan/boundaries.md":                {},
	"building-gormes/architecture_plan/why-go.md":                    {},
	"building-gormes/core-systems/_index.md":                         {},
	"building-gormes/core-systems/gateway.md":                        {},
	"building-gormes/core-systems/learning-loop.md":                  {},
	"building-gormes/core-systems/memory.md":                         {},
	"building-gormes/core-systems/tool-execution.md":                 {},
	"using-gormes/_index.md":                                         {},
	"using-gormes/configuration.md":                                  {},
	"using-gormes/faq.md":                                            {},
	"using-gormes/install.md":                                        {},
	"using-gormes/quickstart.md":                                     {},
	"using-gormes/telegram-adapter.md":                               {},
	"using-gormes/tui-mode.md":                                       {},
	"using-gormes/wire-doctor.md":                                    {},
}

func TestMirroredDocsCoverage(t *testing.T) {
	sourcePaths := collectSourceDocs(t)
	contentPaths := collectContentDocs(t)

	expected := make(map[string]struct{}, len(sourcePaths))
	for _, rel := range sourcePaths {
		expected[mapSourceToContent(rel)] = struct{}{}
	}

	seen := make(map[string]struct{}, len(contentPaths))
	for _, rel := range contentPaths {
		seen[rel] = struct{}{}
	}

	for rel := range expected {
		if _, ok := seen[rel]; !ok {
			t.Fatalf("missing mirrored path %s", rel)
		}
	}
	for rel := range seen {
		if _, ok := expected[rel]; ok {
			continue
		}
		if _, ok := nativeHugoPages[rel]; ok {
			continue
		}
		t.Fatalf("unexpected content file %s", rel)
	}
}

func TestProgressJsonHasSingleCanonicalDocsCopy(t *testing.T) {
	canonical := filepath.Join(hugoContentRoot, "building-gormes", "architecture_plan", "progress.json")
	if _, err := os.Stat(canonical); err != nil {
		t.Fatalf("canonical progress.json missing: %v", err)
	}

	duplicate := filepath.Join("data", "progress.json")
	if _, err := os.Stat(duplicate); err == nil {
		t.Fatalf("non-canonical progress copy must not exist: %s", duplicate)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat duplicate progress.json: %v", err)
	}
}

func TestHugoContentRendersViaGoldmark(t *testing.T) {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Table, extension.Strikethrough))

	for _, rel := range collectContentDocs(t) {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(hugoContentRoot, rel))
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}

			var buf bytes.Buffer
			if err := md.Convert(raw, &buf); err != nil {
				t.Fatalf("goldmark render %s: %v", rel, err)
			}
			if buf.Len() == 0 {
				t.Fatalf("goldmark produced empty output for %s", rel)
			}
		})
	}
}

func TestHugoContentAvoidsPortabilityHazards(t *testing.T) {
	for _, rel := range collectContentDocs(t) {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(hugoContentRoot, rel))
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}

			scanForHazards(t, rel, string(raw))
		})
	}
}

func TestHugoInternalLinksResolve(t *testing.T) {
	for _, rel := range collectContentDocs(t) {
		raw, err := os.ReadFile(filepath.Join(hugoContentRoot, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}

		inFence := false
		for _, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}

			for _, match := range markdownLinkPattern.FindAllStringSubmatch(line, -1) {
				if strings.HasPrefix(strings.TrimSpace(line), "![") {
					continue
				}
				link := strings.TrimSpace(match[1])
				if link == "" || isExternalLink(link) || strings.HasPrefix(link, "#") {
					continue
				}
				if strings.HasPrefix(link, "/") {
					t.Fatalf("%s: root-relative internal link %q", rel, link)
				}
				if err := resolveContentLink(rel, link); err != nil {
					t.Fatalf("%s: unresolved internal link %q: %v", rel, link, err)
				}
			}
		}
	}
}

func TestGatewayDonorMapInvariants(t *testing.T) {
	const donorMapDir = "content/building-gormes/gateway-donor-map"

	entries, err := os.ReadDir(donorMapDir)
	if err != nil {
		t.Fatalf("read donor map dir: %v", err)
	}

	dossierRecommendations := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		name := entry.Name()
		path := filepath.Join(donorMapDir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(raw)

		switch name {
		case "_index.md", "shared-adapter-patterns.md":
			for _, want := range gatewayDonorMapPinnedProvenance {
				if !strings.Contains(content, want) {
					t.Fatalf("%s missing pinned provenance string %q", path, want)
				}
			}
			continue
		}

		for _, heading := range gatewayDonorMapRequiredHeadings {
			if !strings.Contains(content, heading) {
				t.Fatalf("%s missing heading %q", path, heading)
			}
		}

		match := gatewayDonorMapRecommendationPattern.FindStringSubmatch(content)
		if len(match) != 2 {
			t.Fatalf("%s missing final recommendation label", path)
		}
		recommendation := match[1]
		if _, ok := gatewayDonorMapAllowedRecommendations[recommendation]; !ok {
			t.Fatalf("%s has unsupported recommendation %q", path, recommendation)
		}

		dossierRecommendations[strings.TrimSuffix(name, ".md")] = recommendation
	}

	indexPath := filepath.Join(donorMapDir, "_index.md")
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read %s: %v", indexPath, err)
	}

	matches := gatewayDonorMapHubRowPattern.FindAllStringSubmatch(string(indexRaw), -1)
	if len(matches) == 0 {
		t.Fatalf("%s missing triage table rows", indexPath)
	}

	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		channel := match[1]
		recommendation := match[2]
		label := match[3]
		slug := match[4]

		if channel != label {
			t.Fatalf("%s row channel %q does not match dossier label %q", indexPath, channel, label)
		}
		want, ok := dossierRecommendations[slug]
		if !ok {
			t.Fatalf("%s row for %q points to unknown dossier %q", indexPath, channel, slug)
		}
		if recommendation != want {
			t.Fatalf("%s row for %q has recommendation %q, dossier has %q", indexPath, channel, recommendation, want)
		}
		seen[slug] = struct{}{}
	}

	if len(seen) != len(dossierRecommendations) {
		var missing []string
		for slug := range dossierRecommendations {
			if _, ok := seen[slug]; !ok {
				missing = append(missing, slug)
			}
		}
		sort.Strings(missing)
		t.Fatalf("%s missing triage rows for dossiers: %s", indexPath, strings.Join(missing, ", "))
	}
}

func collectSourceDocs(t *testing.T) []string {
	t.Helper()

	if _, err := os.Stat(sourceDocsRoot); os.IsNotExist(err) {
		t.Skipf("upstream website docs source not present in standalone Gormes repo: %s", sourceDocsRoot)
	}

	var paths []string
	err := filepath.WalkDir(sourceDocsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(sourceDocsRoot, path)
		if err != nil {
			return err
		}
		if !isMirroredSourceFile(rel) {
			return nil
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk source docs: %v", err)
	}
	sort.Strings(paths)

	return paths
}

func collectContentDocs(t *testing.T) []string {
	t.Helper()

	var paths []string
	err := filepath.WalkDir(hugoContentRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(hugoContentRoot, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk hugo content: %v", err)
	}
	sort.Strings(paths)

	return paths
}

func isMirroredSourceFile(rel string) bool {
	switch filepath.Base(rel) {
	case "_category_.json", "index.md":
		return true
	}
	return filepath.Ext(rel) == ".md"
}

func mapSourceToContent(rel string) string {
	rel = filepath.ToSlash(rel)
	// Upstream docs mirror lives under content/upstream-hermes/.
	const mirrorPrefix = "upstream-hermes/"
	if rel == "index.md" {
		return mirrorPrefix + "_index.md"
	}
	if strings.HasSuffix(rel, "/index.md") {
		return mirrorPrefix + strings.TrimSuffix(rel, "index.md") + "_index.md"
	}
	if strings.HasSuffix(rel, "/_category_.json") {
		return mirrorPrefix + strings.TrimSuffix(rel, "_category_.json") + "_index.md"
	}
	return mirrorPrefix + rel
}

func scanForHazards(t *testing.T, rel, raw string) {
	t.Helper()

	inFence := false
	for i, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		isImageLine := strings.HasPrefix(trimmed, "![")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, hazard := range bannedPatterns {
			if hazard.name == "root-relative-link" && isImageLine {
				continue
			}
			if hazard.pattern.MatchString(line) {
				t.Fatalf("%s:%d %s: %q", rel, i+1, hazard.name, line)
			}
		}
	}
}

func isExternalLink(link string) bool {
	switch {
	case strings.HasPrefix(link, "http://"),
		strings.HasPrefix(link, "https://"),
		strings.HasPrefix(link, "mailto:"),
		strings.HasPrefix(link, "tel:"),
		strings.HasPrefix(link, "ftp://"),
		strings.HasPrefix(link, "//"):
		return true
	default:
		return false
	}
}

func TestResolveContentLinkUsesRenderedLeafURLBase(t *testing.T) {
	sourceRel := "building-gormes/autoloop/autoloop-handoff.md"

	if err := resolveContentLink(sourceRel, "./agent-queue/"); !os.IsNotExist(err) {
		t.Fatalf("leaf-relative sibling link resolved from source directory; got %v", err)
	}
	if err := resolveContentLink(sourceRel, "../agent-queue/"); err != nil {
		t.Fatalf("rendered leaf-relative sibling link should resolve: %v", err)
	}
}

func resolveContentLink(sourceRel, link string) error {
	target := link
	if idx := strings.IndexAny(target, "?#"); idx >= 0 {
		target = target[:idx]
	}

	sourceDir := filepath.Dir(sourceRel)
	renderedDir := sourceDir
	if base := filepath.Base(sourceRel); base != "_index.md" && filepath.Ext(base) == ".md" {
		renderedDir = filepath.Join(sourceDir, strings.TrimSuffix(base, ".md"))
	}

	candidateRel := filepath.Clean(filepath.Join(renderedDir, target))
	candidate := filepath.Join(hugoContentRoot, candidateRel)
	checks := []string{
		candidate + ".md",
		filepath.Join(candidate, "_index.md"),
		filepath.Join(candidate, "index.md"),
	}
	if strings.HasSuffix(target, "/") {
		checks = append([]string{filepath.Join(candidate, "_index.md")}, checks...)
	}

	for _, check := range checks {
		if _, err := os.Stat(check); err == nil {
			return nil
		}
	}

	return os.ErrNotExist
}

func TestHugoBuildProducesRenderedContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skip Hugo build in short mode")
	}

	dest := t.TempDir()
	cmd := exec.Command("go", "run", "github.com/gohugoio/hugo@v0.160.1", "--panicOnWarning", "--cleanDestinationDir", "--destination", dest)
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hugo build failed: %v\n%s", err, output)
	}

	// Hugo's default filename-based routing preserves section directories:
	// content/upstream-hermes/user-guide/cli.md -> /upstream-hermes/user-guide/cli/.
	// Tests previously expected the flattened (/upstream-hermes/:slug/)
	// form from the old [permalinks] override; that override was removed
	// because it clashed with the nested building-gormes/architecture_plan/
	// routes asserted by TestHugoBuild.
	checks := map[string][]string{
		"index.html": {
			"USING GORMES",
			"BUILDING GORMES",
			"UPSTREAM HERMES",
		},
		filepath.Join("why-gormes", "index.html"): {
			"Operational Moat",
			"Wire Doctor",
			"Chaos Resilience",
			"Surgical Architecture",
		},
		filepath.Join("upstream-hermes", "user-guide", "cli", "index.html"): {
			"Stylized preview of the Hermes CLI layout",
			"The Hermes CLI banner, conversation stream, and fixed input prompt",
		},
		filepath.Join("upstream-hermes", "user-guide", "sessions", "index.html"): {
			"Stylized preview of the Previous Conversation recap panel",
			"Resume mode shows a compact recap panel",
		},
	}

	for rel, wants := range checks {
		raw, err := os.ReadFile(filepath.Join(dest, rel))
		if err != nil {
			t.Fatalf("read rendered %s: %v", rel, err)
		}
		content := string(raw)
		for _, want := range wants {
			if !strings.Contains(content, want) {
				t.Fatalf("rendered %s missing %q", rel, want)
			}
		}
	}
}
