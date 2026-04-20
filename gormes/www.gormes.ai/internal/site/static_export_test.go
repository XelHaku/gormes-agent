package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportDir_WritesStaticSite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "dist")

	if err := ExportDir(root); err != nil {
		t.Fatalf("ExportDir: %v", err)
	}

	indexBody, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	text := string(indexBody)
	wants := []string{
		"One Go Binary. Same Hermes Brain.",
		"curl -fsSL https://gormes.ai/install.sh | sh",
		"Why a Go layer matters.",
		"What ships now, what doesn&#39;t.",
		"Phase 3.A–C + 3.D.5 — SQLite + FTS5 lattice, ontological graph, neural recall, USER.md mirror.",
		"Phase 3.D — Ollama embeddings + semantic fusion.",
		"Phase 4 — Brain transplant. Hermes backend becomes optional.",
		"Phase 5 — 100% Go. Python tool scripts ported. Hermes-off.",
	}
	rejects := []string{
		"Run Hermes Through a Go Operator Console.",
		"Hermes, In a Single Static Binary.",
		"No Python runtime on the host",
		"~8 MB",
		"~12 MB",
		"Phase 3 — SQLite + FTS5 transcript memory.",
		"Phase 4 — Native prompt building + agent orchestration.",
		"Boot Sequence",
		"Proof Rail",
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("dist/index.html missing %q", want)
		}
	}
	for _, reject := range rejects {
		if strings.Contains(text, reject) {
			t.Fatalf("dist/index.html still contains stale token %q", reject)
		}
	}

	cssPath := filepath.Join(root, "static", "site.css")
	css, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("read %s: %v", cssPath, err)
	}
	if !strings.Contains(string(css), "--bg-0") {
		t.Fatalf("site.css missing --bg-0 design token")
	}

	installBody, err := os.ReadFile(filepath.Join(root, "install.sh"))
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	if !strings.Contains(string(installBody), "github.com/TrebuchetDynamics/gormes-agent/gormes/cmd/gormes") {
		t.Fatalf("install.sh missing TrebuchetDynamics module path")
	}
}

func TestExportDir_RecreatesDist(t *testing.T) {
	root := filepath.Join(t.TempDir(), "dist")
	stalePath := filepath.Join(root, "stale.txt")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := ExportDir(root); err != nil {
		t.Fatalf("ExportDir: %v", err)
	}

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale file still present after export: err=%v", err)
	}
}
