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
	"why-gormes.md": {},
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

func collectSourceDocs(t *testing.T) []string {
	t.Helper()

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
	if rel == "index.md" {
		return "_index.md"
	}
	if strings.HasSuffix(rel, "/index.md") {
		return strings.TrimSuffix(rel, "index.md") + "_index.md"
	}
	if strings.HasSuffix(rel, "/_category_.json") {
		return strings.TrimSuffix(rel, "_category_.json") + "_index.md"
	}
	return rel
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

func resolveContentLink(sourceRel, link string) error {
	sourceDir := filepath.Dir(filepath.Join(hugoContentRoot, sourceRel))
	target := link
	if idx := strings.IndexAny(target, "?#"); idx >= 0 {
		target = target[:idx]
	}
	candidate := filepath.Clean(filepath.Join(sourceDir, target))

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

	checks := map[string][]string{
		"index.html": {
			"Gormes Documentation",
			"Why Gormes",
			"Quick Start on GitHub",
		},
		filepath.Join("why-gormes", "index.html"): {
			"Operational Moat",
			"Wire Doctor",
			"Chaos Resilience",
			"Surgical Architecture",
		},
		filepath.Join("user-guide", "cli", "index.html"): {
			"Stylized preview of the Hermes CLI layout",
			"The Hermes CLI banner, conversation stream, and fixed input prompt",
		},
		filepath.Join("user-guide", "sessions", "index.html"): {
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
