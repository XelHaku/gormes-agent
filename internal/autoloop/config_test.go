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

	if cfg.MaxPhase != 0 {
		t.Fatalf("MaxPhase = %d, want %d", cfg.MaxPhase, 0)
	}
}

func TestConfigFromEnvReadsOverrides(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	cfg, err := ConfigFromEnv(root, map[string]string{
		"RUN_ROOT":   "/tmp/run",
		"BACKEND":    "claudeu",
		"MODE":       "full",
		"MAX_AGENTS": "7",
		"MAX_PHASE":  "5",
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

	if cfg.MaxPhase != 5 {
		t.Fatalf("MaxPhase = %d, want %d", cfg.MaxPhase, 5)
	}
}

func TestConfigFromEnvRejectsEmptyRepoRoot(t *testing.T) {
	if _, err := ConfigFromEnv("", map[string]string{}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
}

func TestConfigFromEnvRejectsInvalidMaxAgents(t *testing.T) {
	if _, err := ConfigFromEnv("repo", map[string]string{"MAX_AGENTS": "many"}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
}

func TestConfigFromEnvRejectsZeroMaxAgents(t *testing.T) {
	if _, err := ConfigFromEnv("repo", map[string]string{"MAX_AGENTS": "0"}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
}

func TestConfigFromEnvRejectsInvalidMaxPhase(t *testing.T) {
	if _, err := ConfigFromEnv("repo", map[string]string{"MAX_PHASE": "many"}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
}

func TestConfigFromEnvAllowsZeroMaxPhaseAsUnbounded(t *testing.T) {
	cfg, err := ConfigFromEnv("repo", map[string]string{"MAX_PHASE": "0"})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.MaxPhase != 0 {
		t.Fatalf("MaxPhase = %d, want 0", cfg.MaxPhase)
	}
}

func TestConfigFromEnvReadsProgressJSONOverride(t *testing.T) {
	cfg, err := ConfigFromEnv("repo", map[string]string{"PROGRESS_JSON": "/tmp/progress.json"})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.ProgressJSON != "/tmp/progress.json" {
		t.Fatalf("ProgressJSON = %q, want %q", cfg.ProgressJSON, "/tmp/progress.json")
	}
}
