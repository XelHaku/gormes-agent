package autoloop

import (
	"path/filepath"
	"reflect"
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

	if cfg.MaxPhase != 3 {
		t.Fatalf("MaxPhase = %d, want %d", cfg.MaxPhase, 3)
	}

	wantPriorityBoost := []string{"2.B.3", "2.B.4", "2.B.10", "2.B.11"}
	if !reflect.DeepEqual(cfg.PriorityBoost, wantPriorityBoost) {
		t.Fatalf("PriorityBoost = %#v, want %#v", cfg.PriorityBoost, wantPriorityBoost)
	}
}

func TestConfigFromEnvReadsOverrides(t *testing.T) {
	root := filepath.Join("tmp", "repo")

	cfg, err := ConfigFromEnv(root, map[string]string{
		"RUN_ROOT":       "/tmp/run",
		"BACKEND":        "claudeu",
		"MODE":           "full",
		"MAX_AGENTS":     "7",
		"MAX_PHASE":      "5",
		"PRIORITY_BOOST": "3.E.7, 4.A ",
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

	if want := []string{"3.E.7", "4.A"}; !reflect.DeepEqual(cfg.PriorityBoost, want) {
		t.Fatalf("PriorityBoost = %#v, want %#v", cfg.PriorityBoost, want)
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

func TestConfigFromEnvReactiveDefaults(t *testing.T) {
	cfg, err := ConfigFromEnv("repo", map[string]string{})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.QuarantineThreshold != 3 {
		t.Fatalf("QuarantineThreshold = %d, want 3", cfg.QuarantineThreshold)
	}
	if cfg.BackendDegradeThreshold != 3 {
		t.Fatalf("BackendDegradeThreshold = %d, want 3", cfg.BackendDegradeThreshold)
	}
	if cfg.BackendFallback != nil {
		t.Fatalf("BackendFallback = %#v, want nil", cfg.BackendFallback)
	}
	if cfg.IncludeQuarantined != false {
		t.Fatalf("IncludeQuarantined = %v, want false", cfg.IncludeQuarantined)
	}
	if cfg.IncludeNeedsHuman != false {
		t.Fatalf("IncludeNeedsHuman = %v, want false", cfg.IncludeNeedsHuman)
	}
	if cfg.ReportRepairEnabled != true {
		t.Fatalf("ReportRepairEnabled = %v, want true", cfg.ReportRepairEnabled)
	}
	if cfg.PlannerQuarantineLimit != 5 {
		t.Fatalf("PlannerQuarantineLimit = %d, want 5", cfg.PlannerQuarantineLimit)
	}
	if cfg.MergeOpenPullRequests != true {
		t.Fatalf("MergeOpenPullRequests = %v, want true", cfg.MergeOpenPullRequests)
	}
	if cfg.AutoCommitDirtyWorktree != true {
		t.Fatalf("AutoCommitDirtyWorktree = %v, want true", cfg.AutoCommitDirtyWorktree)
	}
	wantVerify := defaultPostPromotionVerifyCommands()
	if !reflect.DeepEqual(cfg.PostPromotionVerifyCommands, wantVerify) {
		t.Fatalf("PostPromotionVerifyCommands = %#v, want %#v", cfg.PostPromotionVerifyCommands, wantVerify)
	}
	if cfg.PostPromotionRepairEnabled != true {
		t.Fatalf("PostPromotionRepairEnabled = %v, want true", cfg.PostPromotionRepairEnabled)
	}
	if cfg.PostPromotionRepairAttempts != 1 {
		t.Fatalf("PostPromotionRepairAttempts = %d, want 1", cfg.PostPromotionRepairAttempts)
	}
}

func TestConfigFromEnvReactiveOverrides(t *testing.T) {
	cfg, err := ConfigFromEnv("repo", map[string]string{
		"QUARANTINE_THRESHOLD":            "7",
		"BACKEND_DEGRADE_THRESHOLD":       "2",
		"BACKEND_FALLBACK":                "codexu, claudeu ,opencode",
		"GORMES_INCLUDE_QUARANTINED":      "true",
		"GORMES_INCLUDE_NEEDS_HUMAN":      "yes",
		"GORMES_REPORT_REPAIR":            "0",
		"GORMES_PLANNER_QUARANTINE_LIMIT": "9",
		"MERGE_OPEN_PULL_REQUESTS":        "0",
		"AUTO_COMMIT_DIRTY_WORKTREE":      "false",
		"POST_PROMOTION_VERIFY_COMMANDS":  "go test ./internal/autoloop -count=1;;go run ./cmd/autoloop progress validate",
		"POST_PROMOTION_REPAIR":           "off",
		"POST_PROMOTION_REPAIR_ATTEMPTS":  "2",
	})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.QuarantineThreshold != 7 {
		t.Fatalf("QuarantineThreshold = %d, want 7", cfg.QuarantineThreshold)
	}
	if cfg.BackendDegradeThreshold != 2 {
		t.Fatalf("BackendDegradeThreshold = %d, want 2", cfg.BackendDegradeThreshold)
	}
	want := []string{"codexu", "claudeu", "opencode"}
	if !reflect.DeepEqual(cfg.BackendFallback, want) {
		t.Fatalf("BackendFallback = %#v, want %#v", cfg.BackendFallback, want)
	}
	if cfg.IncludeQuarantined != true {
		t.Fatalf("IncludeQuarantined = %v, want true", cfg.IncludeQuarantined)
	}
	if cfg.IncludeNeedsHuman != true {
		t.Fatalf("IncludeNeedsHuman = %v, want true", cfg.IncludeNeedsHuman)
	}
	if cfg.ReportRepairEnabled != false {
		t.Fatalf("ReportRepairEnabled = %v, want false", cfg.ReportRepairEnabled)
	}
	if cfg.PlannerQuarantineLimit != 9 {
		t.Fatalf("PlannerQuarantineLimit = %d, want 9", cfg.PlannerQuarantineLimit)
	}
	if cfg.MergeOpenPullRequests != false {
		t.Fatalf("MergeOpenPullRequests = %v, want false", cfg.MergeOpenPullRequests)
	}
	if cfg.AutoCommitDirtyWorktree != false {
		t.Fatalf("AutoCommitDirtyWorktree = %v, want false", cfg.AutoCommitDirtyWorktree)
	}
	verifyWant := []string{"go test ./internal/autoloop -count=1", "go run ./cmd/autoloop progress validate"}
	if !reflect.DeepEqual(cfg.PostPromotionVerifyCommands, verifyWant) {
		t.Fatalf("PostPromotionVerifyCommands = %#v, want %#v", cfg.PostPromotionVerifyCommands, verifyWant)
	}
	if cfg.PostPromotionRepairEnabled != false {
		t.Fatalf("PostPromotionRepairEnabled = %v, want false", cfg.PostPromotionRepairEnabled)
	}
	if cfg.PostPromotionRepairAttempts != 2 {
		t.Fatalf("PostPromotionRepairAttempts = %d, want 2", cfg.PostPromotionRepairAttempts)
	}
}

func TestConfigFromEnvBackendFallbackEmptyYieldsEmptySlice(t *testing.T) {
	cfg, err := ConfigFromEnv("repo", map[string]string{"BACKEND_FALLBACK": ""})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	// Explicit empty -> nil slice (not [""]). Length must be 0.
	if len(cfg.BackendFallback) != 0 {
		t.Fatalf("BackendFallback = %#v, want length 0", cfg.BackendFallback)
	}
}

func TestConfigFromEnvBackendFallbackUnsetKeepsDefault(t *testing.T) {
	// Map with no key at all -> keep default nil.
	cfg, err := ConfigFromEnv("repo", map[string]string{})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.BackendFallback != nil {
		t.Fatalf("BackendFallback = %#v, want nil", cfg.BackendFallback)
	}
}

func TestConfigFromEnvQuarantineThresholdEmptyKeepsDefault(t *testing.T) {
	cfg, err := ConfigFromEnv("repo", map[string]string{"QUARANTINE_THRESHOLD": ""})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.QuarantineThreshold != 3 {
		t.Fatalf("QuarantineThreshold = %d, want default 3", cfg.QuarantineThreshold)
	}
}

func TestConfigFromEnvReportRepairFalseValues(t *testing.T) {
	for _, v := range []string{"0", "false", "no", "off", "FALSE"} {
		cfg, err := ConfigFromEnv("repo", map[string]string{"GORMES_REPORT_REPAIR": v})
		if err != nil {
			t.Fatalf("ConfigFromEnv(%q) error = %v", v, err)
		}
		if cfg.ReportRepairEnabled {
			t.Fatalf("ReportRepairEnabled = true for %q, want false", v)
		}
	}
}

func TestConfigFromEnvRejectsInvalidQuarantineThreshold(t *testing.T) {
	if _, err := ConfigFromEnv("repo", map[string]string{"QUARANTINE_THRESHOLD": "abc"}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
	if _, err := ConfigFromEnv("repo", map[string]string{"QUARANTINE_THRESHOLD": "0"}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error for zero threshold")
	}
}

func TestConfigFromEnvRejectsInvalidReportRepair(t *testing.T) {
	if _, err := ConfigFromEnv("repo", map[string]string{"GORMES_REPORT_REPAIR": "maybe"}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
}

func TestConfigFromEnvRejectsInvalidPostPromotionRepair(t *testing.T) {
	if _, err := ConfigFromEnv("repo", map[string]string{"POST_PROMOTION_REPAIR": "maybe"}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
}

func TestConfigFromEnvRejectsInvalidPostPromotionRepairAttempts(t *testing.T) {
	if _, err := ConfigFromEnv("repo", map[string]string{"POST_PROMOTION_REPAIR_ATTEMPTS": "many"}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want error")
	}
	if _, err := ConfigFromEnv("repo", map[string]string{"POST_PROMOTION_REPAIR_ATTEMPTS": "-1"}); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want non-negative error")
	}
}
