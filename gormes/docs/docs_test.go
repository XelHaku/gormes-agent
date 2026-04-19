package docs_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Portability rules from spec §21.3: these patterns break some SSGs.
var bannedPatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"github-admonition", regexp.MustCompile(`^> \[!(NOTE|WARNING|TIP|IMPORTANT|CAUTION)\]`)},
	{"root-relative-link", regexp.MustCompile(`\]\(/[^)]+\)`)},
	{"raw-html-block", regexp.MustCompile(`(?m)^<(div|span|details|summary|section)\b`)},
}

var targets = []string{
	"ARCH_PLAN.md",
	"THEORETICAL_ADVANTAGES_GORMES_HERMES.md",
	"superpowers/specs/2026-04-18-gormes-frontend-adapter-design.md",
	"superpowers/plans/2026-04-18-gormes-phase1-frontend-adapter.md",
	"superpowers/specs/2026-04-19-gormes-landing-page-design.md",
	"superpowers/plans/2026-04-19-gormes-landing-page.md",
}

func TestMarkdownRendersCleanViaGoldmark(t *testing.T) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Table, extension.Strikethrough),
	)
	for _, rel := range targets {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(".", rel))
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			var buf bytes.Buffer
			if err := md.Convert(raw, &buf); err != nil {
				t.Errorf("goldmark render %s: %v", rel, err)
			}
			if buf.Len() == 0 {
				t.Errorf("goldmark produced empty output for %s", rel)
			}
		})
	}
}

func TestMarkdownAvoidsPortabilityHazards(t *testing.T) {
	for _, rel := range targets {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(".", rel))
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			inFence := false
			for i, line := range strings.Split(string(raw), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
					inFence = !inFence
					continue
				}
				if inFence {
					continue
				}
				for _, b := range bannedPatterns {
					if b.pattern.MatchString(line) {
						t.Errorf("%s:%d %s — %q", rel, i+1, b.name, line)
					}
				}
			}
		})
	}
}
