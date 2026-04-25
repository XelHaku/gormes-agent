package site

import (
	"os"
	"path/filepath"
	"regexp"
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
		"One Go Binary. No Python. No Drift.",
		"curl -fsSL https://gormes.ai/install.sh | sh",
		"irm https://gormes.ai/install.ps1 | iex",
		"Rerun the installer to update the managed Gormes checkout.",
		"Why Hermes breaks in production — and how Gormes fixes it.",
		"What works today, and what&#39;s still being wired up.",
		// Favicons + social-card meta tags rendered in <head>.
		`href="/static/favicon.ico"`,
		`href="/static/apple-touch-icon.png"`,
		`property="og:image" content="https://gormes.ai/static/social-card.png"`,
		`name="twitter:card" content="summary_large_image"`,
		// Structural roadmap checks — no exact counts or item names,
		// those come from progress.json and must not be locked in here.
		"roadmap-status-progress",
		"roadmap-status-planned",
		"roadmap-item-shipped",
		"roadmap-phase",
		// Fuzzy phase-title presence — each phase renders, no subtitles
		"Phase 1",
		"Phase 2",
		"Phase 3",
		"Phase 4",
		"Phase 5",
		"Phase 6",
	}
	rejects := []string{
		"Run Hermes Through a Go Operator Console.",
		"Hermes, In a Single Static Binary.",
		"Requires Hermes backend at localhost:8642.",
		"Install Hermes →",
		"No Python runtime on the host",
		"~8 MB",
		"~12 MB",
		"Phase 3 — SQLite + FTS5 transcript memory.",
		// Old single-row ledger copy must not survive the grouped rewrite
		"Phase 4 — Native prompt building + agent orchestration.",
		"Phase 4 — Brain transplant. Hermes backend becomes optional.",
		"Phase 5 — 100% Go. Python tool scripts ported. Hermes-off.",
		"Phase 3.A–C + 3.D.5 — SQLite + FTS5 lattice, ontological graph, neural recall, USER.md mirror",
		"Boot Sequence",
		"Proof Rail",
		// Old hero/features copy that conflated frontend with full replacement
		"One Go Binary. Same Hermes Brain.",
		"A static Go binary that talks to your Hermes backend over HTTP.",
		"Why a Go layer matters.",
		"Boots Like a Tool",
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

	// Roadmap renders all 7 phases — structural, not copy-specific.
	if n := strings.Count(text, `class="roadmap-phase"`); n != 7 {
		t.Errorf("dist/index.html roadmap phase count = %d, want 7", n)
	}

	// The progress tracker follows "N/M shipped" — shape, not numbers.
	trackerRE := regexp.MustCompile(`\d+/\d+ shipped`)
	if !trackerRE.MatchString(text) {
		t.Errorf("dist/index.html missing N/M shipped tracker label")
	}

	cssPath := filepath.Join(root, "static", "site.css")
	css, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("read %s: %v", cssPath, err)
	}
	if !strings.Contains(string(css), "--bg-0") {
		t.Fatalf("site.css missing --bg-0 design token")
	}

	// Favicon set + OG social card must land in dist/static/. Guarding
	// against regressions in the embed list — if a new icon is added it
	// has to flow through ExportDir too.
	for _, asset := range []string{
		"favicon.ico",
		"favicon-16x16.png",
		"favicon-32x32.png",
		"apple-touch-icon.png",
		"android-chrome-192x192.png",
		"android-chrome-512x512.png",
		"social-card.png",
	} {
		path := filepath.Join(root, "static", asset)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("static export missing %s: %v", asset, err)
		}
		if info.Size() == 0 {
			t.Fatalf("static export %s is empty", asset)
		}
	}

	installBody, err := os.ReadFile(filepath.Join(root, "install.sh"))
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	if !strings.Contains(string(installBody), "https://github.com/TrebuchetDynamics/gormes-agent.git") {
		t.Fatalf("install.sh missing TrebuchetDynamics repo URL")
	}

	for _, asset := range []string{"install.ps1", "install.cmd"} {
		body, err := os.ReadFile(filepath.Join(root, asset))
		if err != nil {
			t.Fatalf("read %s: %v", asset, err)
		}
		if len(body) == 0 {
			t.Fatalf("%s is empty in static export", asset)
		}
	}

	ps1Body, err := os.ReadFile(filepath.Join(root, "install.ps1"))
	if err != nil {
		t.Fatalf("read install.ps1: %v", err)
	}
	for _, want := range []string{"LOCALAPPDATA", "gormes-agent", "Invoke-Main"} {
		if !strings.Contains(string(ps1Body), want) {
			t.Fatalf("install.ps1 missing %q in static export", want)
		}
	}

	cmdBody, err := os.ReadFile(filepath.Join(root, "install.cmd"))
	if err != nil {
		t.Fatalf("read install.cmd: %v", err)
	}
	if !strings.Contains(string(cmdBody), "install.ps1") {
		t.Fatalf("install.cmd missing PowerShell handoff in static export")
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
