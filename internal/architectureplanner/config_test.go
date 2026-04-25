package architectureplanner

import (
	"path/filepath"
	"testing"
)

func TestConfigFromEnvDefaultsToArchitecturePlannerPaths(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	cfg, err := ConfigFromEnv(root, map[string]string{})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.RepoRoot != root {
		t.Fatalf("RepoRoot = %q, want %q", cfg.RepoRoot, root)
	}
	if cfg.ProgressJSON != filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json") {
		t.Fatalf("ProgressJSON = %q", cfg.ProgressJSON)
	}
	if cfg.RunRoot != filepath.Join(root, ".codex", "architecture-planner") {
		t.Fatalf("RunRoot = %q", cfg.RunRoot)
	}
	if cfg.AutoloopRunRoot != filepath.Join(root, ".codex", "orchestrator") {
		t.Fatalf("AutoloopRunRoot = %q", cfg.AutoloopRunRoot)
	}
	if cfg.Backend != "codexu" {
		t.Fatalf("Backend = %q, want codexu", cfg.Backend)
	}
	if cfg.Mode != "safe" {
		t.Fatalf("Mode = %q, want safe", cfg.Mode)
	}
}

func TestConfigFromEnvReadsOverrides(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	cfg, err := ConfigFromEnv(root, map[string]string{
		"PROGRESS_JSON":     "/tmp/progress.json",
		"RUN_ROOT":          "/tmp/planner",
		"AUTOLOOP_RUN_ROOT": "/tmp/autoloop",
		"BACKEND":           "claudeu",
		"MODE":              "full",
		"HERMES_DIR":        "/tmp/hermes",
		"GBRAIN_DIR":        "/tmp/gbrain",
		"HONCHO_DIR":        "/tmp/honcho",
	})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.ProgressJSON != "/tmp/progress.json" {
		t.Fatalf("ProgressJSON = %q", cfg.ProgressJSON)
	}
	if cfg.RunRoot != "/tmp/planner" {
		t.Fatalf("RunRoot = %q", cfg.RunRoot)
	}
	if cfg.AutoloopRunRoot != "/tmp/autoloop" {
		t.Fatalf("AutoloopRunRoot = %q", cfg.AutoloopRunRoot)
	}
	if cfg.Backend != "claudeu" {
		t.Fatalf("Backend = %q", cfg.Backend)
	}
	if cfg.Mode != "full" {
		t.Fatalf("Mode = %q", cfg.Mode)
	}
	if cfg.HermesDir != "/tmp/hermes" {
		t.Fatalf("HermesDir = %q", cfg.HermesDir)
	}
	if cfg.GBrainDir != "/tmp/gbrain" {
		t.Fatalf("GBrainDir = %q", cfg.GBrainDir)
	}
	if cfg.HonchoDir != "/tmp/honcho" {
		t.Fatalf("HonchoDir = %q", cfg.HonchoDir)
	}
}

func TestConfigFromEnvRejectsEmptyRepoRoot(t *testing.T) {
	if _, err := ConfigFromEnv("", map[string]string{}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
}
