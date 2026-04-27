package plannerloop

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
)

func TestConfigFromEnvDefaultsToArchitecturePlannerPaths(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	cfg, err := ConfigFromEnv(root, MapEnv(map[string]string{}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.RepoRoot != root {
		t.Fatalf("RepoRoot = %q, want %q", cfg.RepoRoot, root)
	}
	if cfg.ProgressJSON != filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json") {
		t.Fatalf("ProgressJSON = %q", cfg.ProgressJSON)
	}
	if cfg.RunRoot != filepath.Join(root, ".codex", "planner-loop") {
		t.Fatalf("RunRoot = %q", cfg.RunRoot)
	}
	if cfg.AutoloopRunRoot != filepath.Join(root, ".codex", "builder-loop") {
		t.Fatalf("AutoloopRunRoot = %q", cfg.AutoloopRunRoot)
	}
	if cfg.Backend != "codexu" {
		t.Fatalf("Backend = %q, want codexu", cfg.Backend)
	}
	if cfg.Mode != "safe" {
		t.Fatalf("Mode = %q, want safe", cfg.Mode)
	}
	if cfg.MergeOpenPullRequests != true {
		t.Fatalf("MergeOpenPullRequests = %v, want true", cfg.MergeOpenPullRequests)
	}
	if cfg.PRIntakeEmptyBackoff != 5*time.Minute {
		t.Fatalf("PRIntakeEmptyBackoff = %s, want 5m", cfg.PRIntakeEmptyBackoff)
	}
	if cfg.PRConflictAction != builderloop.PRConflictActionClose {
		t.Fatalf("PRConflictAction = %q, want %q", cfg.PRConflictAction, builderloop.PRConflictActionClose)
	}
	if cfg.BackendTimeout != 20*time.Minute {
		t.Fatalf("BackendTimeout = %s, want 20m", cfg.BackendTimeout)
	}
	if !cfg.GitRepairEnabled {
		t.Fatalf("GitRepairEnabled = false, want true")
	}
}

func TestConfigFromEnvPreservesLegacyRunRootWhenCanonicalOnlyHasTriggers(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".codex", "architecture-planner")
	canonical := filepath.Join(root, ".codex", "planner-loop")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "planner_state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy state) error = %v", err)
	}
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatalf("MkdirAll(canonical) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "triggers.jsonl"), nil, 0o644); err != nil {
		t.Fatalf("WriteFile(canonical triggers) error = %v", err)
	}

	cfg, err := ConfigFromEnv(root, MapEnv(map[string]string{}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.RunRoot != legacy {
		t.Fatalf("RunRoot = %q, want legacy state root %q", cfg.RunRoot, legacy)
	}
}

func TestConfigFromEnvReadsOverrides(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	cfg, err := ConfigFromEnv(root, MapEnv(map[string]string{
		"PROGRESS_JSON":                 "/tmp/progress.json",
		"RUN_ROOT":                      "/tmp/planner",
		"AUTOLOOP_RUN_ROOT":             "/tmp/autoloop",
		"BACKEND":                       "claudeu",
		"MODE":                          "full",
		"HERMES_DIR":                    "/tmp/hermes",
		"GBRAIN_DIR":                    "/tmp/gbrain",
		"HONCHO_DIR":                    "/tmp/honcho",
		"MERGE_OPEN_PULL_REQUESTS":      "0",
		"PR_INTAKE_EMPTY_BACKOFF":       "2m30s",
		"PR_INTAKE_CONFLICT_ACTION":     "skip",
		"PLANNER_GORMES_ORIGINAL_PATHS": "cmd/builder-loop/,internal/progress/",
		"PLANNER_IMPL_LOOKBACK":         "48h",
		"PLANNER_TRIGGER_REASON":        "impl_change",
		"PLANNER_BACKEND_TIMEOUT":       "7m",
		"PLANNER_GIT_REPAIR":            "0",
	}))
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
	if cfg.MergeOpenPullRequests != false {
		t.Fatalf("MergeOpenPullRequests = %v, want false", cfg.MergeOpenPullRequests)
	}
	if cfg.PRIntakeEmptyBackoff != 150*time.Second {
		t.Fatalf("PRIntakeEmptyBackoff = %s, want 2m30s", cfg.PRIntakeEmptyBackoff)
	}
	if cfg.PRConflictAction != builderloop.PRConflictActionSkip {
		t.Fatalf("PRConflictAction = %q, want %q", cfg.PRConflictAction, builderloop.PRConflictActionSkip)
	}
	if !reflect.DeepEqual(cfg.GormesOriginalPaths, []string{"cmd/builder-loop/", "internal/progress/"}) {
		t.Fatalf("GormesOriginalPaths = %#v", cfg.GormesOriginalPaths)
	}
	if cfg.ImplLookback != 48*time.Hour {
		t.Fatalf("ImplLookback = %s, want 48h", cfg.ImplLookback)
	}
	if cfg.TriggerReason != "impl_change" {
		t.Fatalf("TriggerReason = %q, want impl_change", cfg.TriggerReason)
	}
	if cfg.BackendTimeout != 7*time.Minute {
		t.Fatalf("BackendTimeout = %s, want 7m", cfg.BackendTimeout)
	}
	if cfg.GitRepairEnabled {
		t.Fatalf("GitRepairEnabled = true, want false")
	}
}

func TestConfigFromEnvRejectsEmptyRepoRoot(t *testing.T) {
	if _, err := ConfigFromEnv("", MapEnv(map[string]string{})); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
}

func TestConfigFromEnv_MaxRetriesDefaultAndOverride(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	// Default: MaxRetries = DefaultMaxRetries (2).
	cfg, err := ConfigFromEnv(root, MapEnv(map[string]string{}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.MaxRetries != DefaultMaxRetries {
		t.Fatalf("MaxRetries default = %d, want %d", cfg.MaxRetries, DefaultMaxRetries)
	}

	// Env override accepted; 0 disables retries (pre-L3 behavior).
	cfg, err = ConfigFromEnv(root, MapEnv(map[string]string{"PLANNER_MAX_RETRIES": "0"}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() override 0 error = %v", err)
	}
	if cfg.MaxRetries != 0 {
		t.Fatalf("MaxRetries = %d, want 0", cfg.MaxRetries)
	}
	cfg, err = ConfigFromEnv(root, MapEnv(map[string]string{"PLANNER_MAX_RETRIES": "5"}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() override 5 error = %v", err)
	}
	if cfg.MaxRetries != 5 {
		t.Fatalf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}

	// Negative and non-integer rejected.
	if _, err := ConfigFromEnv(root, MapEnv(map[string]string{"PLANNER_MAX_RETRIES": "-1"})); err == nil {
		t.Fatal("expected error for negative MaxRetries")
	}
	if _, err := ConfigFromEnv(root, MapEnv(map[string]string{"PLANNER_MAX_RETRIES": "abc"})); err == nil {
		t.Fatal("expected error for non-integer MaxRetries")
	}
}

func TestConfigFromEnv_BackendTimeoutValidation(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	if _, err := ConfigFromEnv(root, MapEnv(map[string]string{"PLANNER_BACKEND_TIMEOUT": "soon"})); err == nil {
		t.Fatal("expected error for invalid PLANNER_BACKEND_TIMEOUT")
	}
	if _, err := ConfigFromEnv(root, MapEnv(map[string]string{"PLANNER_BACKEND_TIMEOUT": "0"})); err == nil {
		t.Fatal("expected error for non-positive PLANNER_BACKEND_TIMEOUT")
	}
	if _, err := ConfigFromEnv(root, MapEnv(map[string]string{"PLANNER_BACKEND_TIMEOUT": "-1s"})); err == nil {
		t.Fatal("expected error for negative PLANNER_BACKEND_TIMEOUT")
	}
}

func TestConfigFromEnv_PlannerTriggersPathDefaultAndOverride(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	// Default: PlannerTriggersPath under repoRoot/.codex/planner-loop.
	cfg, err := ConfigFromEnv(root, MapEnv(map[string]string{}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	wantDefault := filepath.Join(root, ".codex", "planner-loop", "triggers.jsonl")
	if cfg.PlannerTriggersPath != wantDefault {
		t.Fatalf("PlannerTriggersPath default = %q, want %q", cfg.PlannerTriggersPath, wantDefault)
	}
	wantCursor := filepath.Join(cfg.RunRoot, "state", "triggers_cursor.json")
	if cfg.TriggersCursorPath != wantCursor {
		t.Fatalf("TriggersCursorPath default = %q, want %q", cfg.TriggersCursorPath, wantCursor)
	}

	// Env override: PLANNER_TRIGGERS_PATH replaces the default; the cursor
	// path is NOT env-overridable (it follows RunRoot).
	cfg, err = ConfigFromEnv(root, MapEnv(map[string]string{
		"PLANNER_TRIGGERS_PATH": "/srv/triggers.jsonl",
		"RUN_ROOT":              "/srv/planner",
	}))
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

func TestConfigFromEnv_ImplInventoryDefaultsAndValidation(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	cfg, err := ConfigFromEnv(root, MapEnv(map[string]string{}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.GormesOriginalPaths != nil {
		t.Fatalf("GormesOriginalPaths default = %#v, want nil fallback", cfg.GormesOriginalPaths)
	}
	if cfg.ImplLookback != 24*time.Hour {
		t.Fatalf("ImplLookback default = %s, want 24h", cfg.ImplLookback)
	}
	if cfg.TriggerReason != "" {
		t.Fatalf("TriggerReason default = %q, want empty", cfg.TriggerReason)
	}

	if _, err := ConfigFromEnv(root, MapEnv(map[string]string{"PLANNER_IMPL_LOOKBACK": "soon"})); err == nil {
		t.Fatal("expected error for invalid PLANNER_IMPL_LOOKBACK")
	}
	if _, err := ConfigFromEnv(root, MapEnv(map[string]string{"PLANNER_IMPL_LOOKBACK": "0"})); err == nil {
		t.Fatal("expected error for non-positive PLANNER_IMPL_LOOKBACK")
	}
}
