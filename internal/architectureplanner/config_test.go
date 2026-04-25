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

func TestConfigFromEnv_MaxRetriesDefaultAndOverride(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	// Default: MaxRetries = DefaultMaxRetries (2).
	cfg, err := ConfigFromEnv(root, map[string]string{})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.MaxRetries != DefaultMaxRetries {
		t.Fatalf("MaxRetries default = %d, want %d", cfg.MaxRetries, DefaultMaxRetries)
	}

	// Env override accepted; 0 disables retries (pre-L3 behavior).
	cfg, err = ConfigFromEnv(root, map[string]string{"PLANNER_MAX_RETRIES": "0"})
	if err != nil {
		t.Fatalf("ConfigFromEnv() override 0 error = %v", err)
	}
	if cfg.MaxRetries != 0 {
		t.Fatalf("MaxRetries = %d, want 0", cfg.MaxRetries)
	}
	cfg, err = ConfigFromEnv(root, map[string]string{"PLANNER_MAX_RETRIES": "5"})
	if err != nil {
		t.Fatalf("ConfigFromEnv() override 5 error = %v", err)
	}
	if cfg.MaxRetries != 5 {
		t.Fatalf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}

	// Negative and non-integer rejected.
	if _, err := ConfigFromEnv(root, map[string]string{"PLANNER_MAX_RETRIES": "-1"}); err == nil {
		t.Fatal("expected error for negative MaxRetries")
	}
	if _, err := ConfigFromEnv(root, map[string]string{"PLANNER_MAX_RETRIES": "abc"}); err == nil {
		t.Fatal("expected error for non-integer MaxRetries")
	}
}

func TestConfigFromEnv_PlannerTriggersPathDefaultAndOverride(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	// Default: PlannerTriggersPath under repoRoot/.codex/architecture-planner.
	cfg, err := ConfigFromEnv(root, map[string]string{})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	wantDefault := filepath.Join(root, ".codex", "architecture-planner", "triggers.jsonl")
	if cfg.PlannerTriggersPath != wantDefault {
		t.Fatalf("PlannerTriggersPath default = %q, want %q", cfg.PlannerTriggersPath, wantDefault)
	}
	wantCursor := filepath.Join(cfg.RunRoot, "state", "triggers_cursor.json")
	if cfg.TriggersCursorPath != wantCursor {
		t.Fatalf("TriggersCursorPath default = %q, want %q", cfg.TriggersCursorPath, wantCursor)
	}

	// Env override: PLANNER_TRIGGERS_PATH replaces the default; the cursor
	// path is NOT env-overridable (it follows RunRoot).
	cfg, err = ConfigFromEnv(root, map[string]string{
		"PLANNER_TRIGGERS_PATH": "/srv/triggers.jsonl",
		"RUN_ROOT":              "/srv/planner",
	})
	if err != nil {
		t.Fatalf("ConfigFromEnv() override error = %v", err)
	}
	if cfg.PlannerTriggersPath != "/srv/triggers.jsonl" {
		t.Fatalf("PlannerTriggersPath override = %q, want /srv/triggers.jsonl", cfg.PlannerTriggersPath)
	}
	if cfg.TriggersCursorPath != filepath.Join("/srv/planner", "state", "triggers_cursor.json") {
		t.Fatalf("TriggersCursorPath = %q, should follow RunRoot override", cfg.TriggersCursorPath)
	}
}
