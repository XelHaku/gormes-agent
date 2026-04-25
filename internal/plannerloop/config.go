package plannerloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
)

// defaultPlannerLoopRunRoot returns .codex/planner-loop/ unless only the
// legacy .codex/architecture-planner/ directory exists, in which case the
// legacy path wins so existing planner_state.json, planner reports, and
// trigger cursor remain accessible across the rename.
func defaultPlannerLoopRunRoot(repoRoot string) string {
	canonical := filepath.Join(repoRoot, ".codex", "planner-loop")
	legacy := filepath.Join(repoRoot, ".codex", "architecture-planner")

	// Prefer the canonical root only once it has real planner run state.
	// Trigger ledgers may be created under the canonical path before a
	// migration is complete; those should not strand existing reports,
	// cursors, and state under the legacy architecture-planner root.
	if plannerRunRootHasState(canonical) {
		return canonical
	}
	if plannerRunRootHasState(legacy) {
		return legacy
	}
	if _, err := os.Stat(canonical); err == nil {
		return canonical
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return canonical
}

func plannerRunRootHasState(path string) bool {
	for _, rel := range []string{
		"planner_state.json",
		"context.json",
		"latest_prompt.txt",
		filepath.Join("state", "runs.jsonl"),
	} {
		info, err := os.Stat(filepath.Join(path, rel))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// defaultBuilderLoopRunRootFromPlanner mirrors builderloop's default but is
// inlined here to avoid plannerloop importing internals from builderloop
// for a path constant. Same fallback logic: prefer the new
// .codex/builder-loop/ but use the legacy .codex/orchestrator/ if only
// that exists.
func defaultBuilderLoopRunRootFromPlanner(repoRoot string) string {
	canonical := filepath.Join(repoRoot, ".codex", "builder-loop")
	legacy := filepath.Join(repoRoot, ".codex", "orchestrator")
	if _, err := os.Stat(canonical); err == nil {
		return canonical
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return canonical
}

// defaultPlannerTriggersPathFromPlanner returns the planner-loop triggers
// path unless only the legacy architecture-planner triggers.jsonl exists.
func defaultPlannerTriggersPathFromPlanner(repoRoot string) string {
	canonical := filepath.Join(repoRoot, ".codex", "planner-loop", "triggers.jsonl")
	legacy := filepath.Join(repoRoot, ".codex", "architecture-planner", "triggers.jsonl")
	if _, err := os.Stat(canonical); err == nil {
		return canonical
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return canonical
}

type Config struct {
	RepoRoot        string
	ProgressJSON    string
	RunRoot         string
	AutoloopRunRoot string
	Backend         string
	Mode            string
	HermesDir       string
	GBrainDir       string
	HonchoDir       string
	HermesRepoURL   string
	GBrainRepoURL   string
	HonchoRepoURL   string
	Validate        bool
	SyncRepos       bool
	// PlannerQuarantineLimit caps how many quarantined rows are surfaced in
	// the planner's call-to-action context block. 0 means no cap. Sourced
	// from GORMES_PLANNER_QUARANTINE_LIMIT, default 5 (mirrors the autoloop
	// runtime setting added in Task 4 — kept in sync but as a separate field
	// because the planner's Config is independent of autoloop's).
	PlannerQuarantineLimit int
	// PlannerTriggersPath is the JSONL ledger autoloop appends to whenever
	// a row's quarantine state transition warrants a planner re-run. The
	// planner reads this file at the start of each run, advances a cursor
	// past the consumed events, and surfaces them in the prompt so the
	// next run can react. Defaults to <repoRoot>/.codex/architecture-planner/
	// triggers.jsonl; env-overridable via PLANNER_TRIGGERS_PATH so the
	// autoloop and planner sides can be steered to the same file.
	PlannerTriggersPath string
	// TriggersCursorPath is where the planner persists its bookmark in
	// PlannerTriggersPath. NOT env-overridable — it lives next to the
	// other planner state files under RunRoot/state so a single RUN_ROOT
	// override moves the cursor along with everything else.
	TriggersCursorPath string
	// MaxRetries caps how many follow-up LLM calls the planner issues
	// after validateHealthPreservation rejects an initial regen. The
	// retry feedback names the dropped rows and references the HARD rule
	// so the same LLM session can self-correct. Backend failures are
	// NEVER retried (only validation rejections). Sourced from
	// PLANNER_MAX_RETRIES; default DefaultMaxRetries (2). Set to 0 to
	// disable retries (pre-L3 single-attempt behavior).
	MaxRetries int
	// BackendTimeout bounds each planner LLM backend invocation. A stuck
	// codexu/claudeu child must not block the periodic planner scheduler
	// indefinitely or leave autoloop paused. Sourced from
	// PLANNER_BACKEND_TIMEOUT (Go time.ParseDuration syntax, e.g. "20m");
	// default 20 minutes.
	BackendTimeout time.Duration
	// MergeOpenPullRequests controls whether planner cycles merge all
	// non-draft open pull requests before collecting context.
	MergeOpenPullRequests bool
	// PRConflictAction controls DIRTY/conflicted PR handling during planner
	// PR intake. "close" deletes the branch after closing; "skip" leaves it.
	PRConflictAction string
	// EvaluationWindow is the lookback for L4 self-evaluation. The planner
	// correlates row reshapes recorded in its own ledger within this window
	// against autoloop's ledger to determine whether previous reshapes
	// unstuck the row, are still failing, or haven't been retried yet.
	// Sourced from PLANNER_EVALUATION_WINDOW (Go time.ParseDuration syntax,
	// e.g. "168h" for seven days); default DefaultEvaluationWindow (7 days).
	EvaluationWindow time.Duration
	// EscalationThreshold is the consecutive-reshape count at which a row
	// whose latest L4 outcome is still_failing is auto-marked NeedsHuman by
	// StampVerdicts. NeedsHuman is sticky — only a human edit clears it.
	// Sourced from PLANNER_ESCALATION_THRESHOLD; default
	// DefaultEscalationThreshold (3). Must be positive.
	EscalationThreshold int
	// GormesOriginalPaths is the deny-list of impl-tree path prefixes
	// considered Gormes-original (no upstream Hermes/GBrain/Honcho analog).
	// Sourced from PLANNER_GORMES_ORIGINAL_PATHS (CSV). When nil/empty,
	// ScanImplementation falls back to DefaultGormesOriginalPaths so the
	// env var override completely replaces the seed list rather than
	// augmenting it.
	GormesOriginalPaths []string
	// ImplLookback bounds how far back ScanImplementation looks when
	// computing ImplInventory.RecentlyChanged. Sourced from
	// PLANNER_IMPL_LOOKBACK (Go time.ParseDuration syntax, e.g. "24h");
	// default 24*time.Hour.
	ImplLookback time.Duration
	// TriggerReason is the planner's per-run trigger label. Sourced from
	// PLANNER_TRIGGER_REASON; default "". Set by the impl-tree path unit
	// (Phase D Task 4) to "impl_change" so the L4 priority order
	// event > impl_change > scheduled can resolve correctly. Empty means
	// the run is scheduled or reactive via the trigger ledger.
	TriggerReason string
}

// EnvLookup mirrors os.LookupEnv: cmd/ binaries pass os.LookupEnv;
// tests pass MapEnv over a literal. The comma-ok shape preserves the
// distinction between "unset" and "set but empty" if any future knob needs it.
type EnvLookup = func(string) (string, bool)

// MapEnv adapts a map to EnvLookup so test cases can declare env as a map.
func MapEnv(m map[string]string) EnvLookup {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

// envValue strips the comma-ok bool for the common "default-or-override"
// callers in ConfigFromEnv.
func envValue(lookup EnvLookup, key string) string {
	v, _ := lookup(key)
	return v
}

func ConfigFromEnv(repoRoot string, lookup EnvLookup) (Config, error) {
	if repoRoot == "" {
		return Config{}, fmt.Errorf("repo root is required")
	}

	parent := filepath.Dir(repoRoot)
	cfg := Config{
		RepoRoot:               repoRoot,
		ProgressJSON:           filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json"),
		RunRoot:                defaultPlannerLoopRunRoot(repoRoot),
		AutoloopRunRoot:        defaultBuilderLoopRunRootFromPlanner(repoRoot),
		Backend:                "codexu",
		Mode:                   "safe",
		HermesDir:              filepath.Join(parent, "hermes-agent"),
		GBrainDir:              filepath.Join(parent, "gbrain"),
		HonchoDir:              filepath.Join(parent, "honcho"),
		HermesRepoURL:          "https://github.com/NousResearch/hermes-agent.git",
		GBrainRepoURL:          "https://github.com/garrytan/gbrain.git",
		HonchoRepoURL:          "https://github.com/plastic-labs/honcho",
		Validate:               true,
		SyncRepos:              true,
		PlannerQuarantineLimit: 5,
		PlannerTriggersPath:    defaultPlannerTriggersPathFromPlanner(repoRoot),
		MaxRetries:             DefaultMaxRetries,
		BackendTimeout:         20 * time.Minute,
		MergeOpenPullRequests:  true,
		PRConflictAction:       builderloop.PRConflictActionClose,
		EvaluationWindow:       DefaultEvaluationWindow,
		EscalationThreshold:    DefaultEscalationThreshold,
		GormesOriginalPaths:    nil, // ScanImplementation falls back to DefaultGormesOriginalPaths
		ImplLookback:           24 * time.Hour,
		TriggerReason:          "",
	}

	if value := envValue(lookup, "PROGRESS_JSON"); value != "" {
		cfg.ProgressJSON = value
	}
	if value := envValue(lookup, "RUN_ROOT"); value != "" {
		cfg.RunRoot = value
	}
	// BUILDER_LOOP_RUN_ROOT is the canonical override for the path the
	// planner watches for the builder-loop ledger; AUTOLOOP_RUN_ROOT is
	// read as a fallback so existing operator env files keep working
	// across the autoloop -> builder-loop rename.
	if value := envValue(lookup, "BUILDER_LOOP_RUN_ROOT"); value != "" {
		cfg.AutoloopRunRoot = value
	} else if value := envValue(lookup, "AUTOLOOP_RUN_ROOT"); value != "" {
		cfg.AutoloopRunRoot = value
	}
	if value := envValue(lookup, "BACKEND"); value != "" {
		cfg.Backend = value
	}
	if value := envValue(lookup, "MODE"); value != "" {
		cfg.Mode = value
	}
	if value := envValue(lookup, "HERMES_DIR"); value != "" {
		cfg.HermesDir = value
	}
	if value := envValue(lookup, "GBRAIN_DIR"); value != "" {
		cfg.GBrainDir = value
	}
	if value := envValue(lookup, "HONCHO_DIR"); value != "" {
		cfg.HonchoDir = value
	}
	if value := envValue(lookup, "HERMES_REPO_URL"); value != "" {
		cfg.HermesRepoURL = value
	}
	if value := envValue(lookup, "GBRAIN_REPO_URL"); value != "" {
		cfg.GBrainRepoURL = value
	}
	if value := envValue(lookup, "HONCHO_REPO_URL"); value != "" {
		cfg.HonchoRepoURL = value
	}
	if value := envValue(lookup, "PLANNER_VALIDATE"); value == "0" {
		cfg.Validate = false
	}
	if value := envValue(lookup, "PLANNER_SYNC_REPOS"); value == "0" {
		cfg.SyncRepos = false
	}
	if value := envValue(lookup, "GORMES_PLANNER_QUARANTINE_LIMIT"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("GORMES_PLANNER_QUARANTINE_LIMIT must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("GORMES_PLANNER_QUARANTINE_LIMIT must be non-negative")
		}
		cfg.PlannerQuarantineLimit = n
	}
	if value := envValue(lookup, "PLANNER_TRIGGERS_PATH"); value != "" {
		cfg.PlannerTriggersPath = value
	}
	if value := envValue(lookup, "PLANNER_MAX_RETRIES"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("PLANNER_MAX_RETRIES must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("PLANNER_MAX_RETRIES must be non-negative")
		}
		cfg.MaxRetries = n
	}
	if value := envValue(lookup, "PLANNER_BACKEND_TIMEOUT"); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("PLANNER_BACKEND_TIMEOUT must be a Go duration (e.g. \"20m\"): %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("PLANNER_BACKEND_TIMEOUT must be positive")
		}
		cfg.BackendTimeout = d
	}
	if value := envValue(lookup, "MERGE_OPEN_PULL_REQUESTS"); value != "" {
		b, err := parseBoolEnv(value)
		if err != nil {
			return Config{}, fmt.Errorf("MERGE_OPEN_PULL_REQUESTS: %w", err)
		}
		cfg.MergeOpenPullRequests = b
	}
	if value := envValue(lookup, "PR_INTAKE_CONFLICT_ACTION"); value != "" {
		action, err := parsePRConflictAction(value)
		if err != nil {
			return Config{}, fmt.Errorf("PR_INTAKE_CONFLICT_ACTION: %w", err)
		}
		cfg.PRConflictAction = action
	}
	if value := envValue(lookup, "PLANNER_EVALUATION_WINDOW"); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("PLANNER_EVALUATION_WINDOW must be a Go duration (e.g. \"168h\"): %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("PLANNER_EVALUATION_WINDOW must be positive")
		}
		cfg.EvaluationWindow = d
	}
	if value := envValue(lookup, "PLANNER_ESCALATION_THRESHOLD"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("PLANNER_ESCALATION_THRESHOLD must be an integer: %w", err)
		}
		if n <= 0 {
			return Config{}, fmt.Errorf("PLANNER_ESCALATION_THRESHOLD must be positive")
		}
		cfg.EscalationThreshold = n
	}
	if value := envValue(lookup, "PLANNER_GORMES_ORIGINAL_PATHS"); value != "" {
		cfg.GormesOriginalPaths = splitCSV(value)
	}
	if value := envValue(lookup, "PLANNER_IMPL_LOOKBACK"); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("PLANNER_IMPL_LOOKBACK must be a Go duration (e.g. \"24h\"): %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("PLANNER_IMPL_LOOKBACK must be positive")
		}
		cfg.ImplLookback = d
	}
	if value := envValue(lookup, "PLANNER_TRIGGER_REASON"); value != "" {
		cfg.TriggerReason = value
	}

	// TriggersCursorPath derives from the (possibly env-overridden) RunRoot
	// so a single RUN_ROOT override moves the cursor with the rest of the
	// planner's on-disk state.
	cfg.TriggersCursorPath = filepath.Join(cfg.RunRoot, "state", "triggers_cursor.json")

	return cfg, nil
}

// splitCSV parses a comma-separated env value into a slice of trimmed,
// non-empty entries. Mirrors the helper of the same name in the autoloop
// package; kept local so the planner config doesn't import the autoloop
// runtime just for a string utility.
func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseBoolEnv(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q (want 1/0/true/false/yes/no/on/off)", value)
}

func parsePRConflictAction(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case builderloop.PRConflictActionClose:
		return builderloop.PRConflictActionClose, nil
	case builderloop.PRConflictActionSkip:
		return builderloop.PRConflictActionSkip, nil
	}
	return "", fmt.Errorf("invalid action %q (want close or skip)", value)
}

func (cfg Config) ExternalRepos() []ExternalRepo {
	return []ExternalRepo{
		{Name: "hermes-agent", Path: cfg.HermesDir, CloneURL: cfg.HermesRepoURL},
		{Name: "gbrain", Path: cfg.GBrainDir, CloneURL: cfg.GBrainRepoURL},
		{Name: "honcho", Path: cfg.HonchoDir, CloneURL: cfg.HonchoRepoURL},
	}
}

func (cfg Config) SourceRoots() []SourceRoot {
	return []SourceRoot{
		{Name: "hermes-agent", Path: cfg.HermesDir},
		{Name: "gbrain", Path: cfg.GBrainDir},
		{Name: "honcho", Path: cfg.HonchoDir},
		{Name: "upstream-hermes", Path: filepath.Join(cfg.RepoRoot, "docs", "content", "upstream-hermes")},
		{Name: "upstream-gbrain", Path: filepath.Join(cfg.RepoRoot, "docs", "content", "upstream-gbrain")},
		{Name: "building-gormes", Path: filepath.Join(cfg.RepoRoot, "docs", "content", "building-gormes")},
		{Name: "www.gormes.ai", Path: filepath.Join(cfg.RepoRoot, "www.gormes.ai")},
		{Name: "hugo-docs", Path: filepath.Join(cfg.RepoRoot, "docs")},
	}
}
