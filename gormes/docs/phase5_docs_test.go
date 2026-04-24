package docs_test

import (
	"os"
	"strings"
	"testing"
)

func TestPhase5DocsTrackExecuteCodeCloseout(t *testing.T) {
	phase5 := readDoc(t, "content/building-gormes/architecture_plan/phase-5-final-purge.md")
	for _, want := range []string{
		"**Status:** 🔨 in progress",
		"| 5.K — Code Execution | ✅ complete |",
		"timeout/output caps",
		"filesystem/network blocking",
	} {
		if !strings.Contains(phase5, want) {
			t.Fatalf("phase-5-final-purge.md is missing %q", want)
		}
	}

	toolExecution := readDoc(t, "content/building-gormes/core-systems/tool-execution.md")
	for _, want := range []string{
		"`execute_code`",
		"`sh`/`python`",
		"filesystem/network blocking",
	} {
		if !strings.Contains(toolExecution, want) {
			t.Fatalf("tool-execution.md is missing %q", want)
		}
	}

	docsProgress, err := os.ReadFile("data/progress.json")
	if err != nil {
		t.Fatalf("read docs/data/progress.json: %v", err)
	}
	for _, want := range []string{
		`"5": {`,
		`"status": "in_progress"`,
		`"5.K": {`,
		`"status": "complete"`,
	} {
		if !strings.Contains(string(docsProgress), want) {
			t.Fatalf("docs/data/progress.json is missing %q", want)
		}
	}

	siteProgress, err := os.ReadFile("../www.gormes.ai/internal/site/data/progress.json")
	if err != nil {
		t.Fatalf("read site progress copy: %v", err)
	}
	for _, want := range []string{
		`"name": "Sandboxed exec"`,
		`"status": "complete"`,
		"`execute_code`",
	} {
		if !strings.Contains(string(siteProgress), want) {
			t.Fatalf("site progress copy is missing %q", want)
		}
	}
}
