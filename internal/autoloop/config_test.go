package autoloop

import (
	"path/filepath"
	"testing"
)

func TestConfigFromEnvDefaultsToRepoRootPaths(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	cfg, err := ConfigFromEnv(root, map[string]string{})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.RepoRoot != root {
		t.Fatalf("RepoRoot = %q, want %q", cfg.RepoRoot, root)
	}

	wantProgressJSON := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	if cfg.ProgressJSON != wantProgressJSON {
		t.Fatalf("ProgressJSON = %q, want %q", cfg.ProgressJSON, wantProgressJSON)
	}

	wantRunRoot := filepath.Join(root, ".codex", "orchestrator")
	if cfg.RunRoot != wantRunRoot {
		t.Fatalf("RunRoot = %q, want %q", cfg.RunRoot, wantRunRoot)
	}

	if cfg.Backend != "codexu" {
		t.Fatalf("Backend = %q, want %q", cfg.Backend, "codexu")
	}

	if cfg.Mode != "safe" {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, "safe")
	}

	if cfg.MaxAgents != 4 {
		t.Fatalf("MaxAgents = %d, want %d", cfg.MaxAgents, 4)
	}
}

func TestConfigFromEnvReadsOverrides(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	cfg, err := ConfigFromEnv(root, map[string]string{
		"RUN_ROOT":   "/tmp/run",
		"BACKEND":    "claudeu",
		"MODE":       "full",
		"MAX_AGENTS": "7",
	})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.RunRoot != "/tmp/run" {
		t.Fatalf("RunRoot = %q, want %q", cfg.RunRoot, "/tmp/run")
	}

	if cfg.Backend != "claudeu" {
		t.Fatalf("Backend = %q, want %q", cfg.Backend, "claudeu")
	}

	if cfg.Mode != "full" {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, "full")
	}

	if cfg.MaxAgents != 7 {
		t.Fatalf("MaxAgents = %d, want %d", cfg.MaxAgents, 7)
	}
}
