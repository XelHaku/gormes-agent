package architectureplanner

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

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
	// MergeOpenPullRequests controls whether planner cycles merge all
	// non-draft open pull requests before collecting context.
	MergeOpenPullRequests bool
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
}

func ConfigFromEnv(repoRoot string, env map[string]string) (Config, error) {
	if repoRoot == "" {
		return Config{}, fmt.Errorf("repo root is required")
	}

	parent := filepath.Dir(repoRoot)
	cfg := Config{
		RepoRoot:               repoRoot,
		ProgressJSON:           filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json"),
		RunRoot:                filepath.Join(repoRoot, ".codex", "architecture-planner"),
		AutoloopRunRoot:        filepath.Join(repoRoot, ".codex", "orchestrator"),
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
		PlannerTriggersPath:    filepath.Join(repoRoot, ".codex", "architecture-planner", "triggers.jsonl"),
		MaxRetries:             DefaultMaxRetries,
		MergeOpenPullRequests:  true,
		EvaluationWindow:       DefaultEvaluationWindow,
		EscalationThreshold:    DefaultEscalationThreshold,
	}

	if value := env["PROGRESS_JSON"]; value != "" {
		cfg.ProgressJSON = value
	}
	if value := env["RUN_ROOT"]; value != "" {
		cfg.RunRoot = value
	}
	if value := env["AUTOLOOP_RUN_ROOT"]; value != "" {
		cfg.AutoloopRunRoot = value
	}
	if value := env["BACKEND"]; value != "" {
		cfg.Backend = value
	}
	if value := env["MODE"]; value != "" {
		cfg.Mode = value
	}
	if value := env["HERMES_DIR"]; value != "" {
		cfg.HermesDir = value
	}
	if value := env["GBRAIN_DIR"]; value != "" {
		cfg.GBrainDir = value
	}
	if value := env["HONCHO_DIR"]; value != "" {
		cfg.HonchoDir = value
	}
	if value := env["HERMES_REPO_URL"]; value != "" {
		cfg.HermesRepoURL = value
	}
	if value := env["GBRAIN_REPO_URL"]; value != "" {
		cfg.GBrainRepoURL = value
	}
	if value := env["HONCHO_REPO_URL"]; value != "" {
		cfg.HonchoRepoURL = value
	}
	if value := env["PLANNER_VALIDATE"]; value == "0" {
		cfg.Validate = false
	}
	if value := env["PLANNER_SYNC_REPOS"]; value == "0" {
		cfg.SyncRepos = false
	}
	if value := env["GORMES_PLANNER_QUARANTINE_LIMIT"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("GORMES_PLANNER_QUARANTINE_LIMIT must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("GORMES_PLANNER_QUARANTINE_LIMIT must be non-negative")
		}
		cfg.PlannerQuarantineLimit = n
	}
	if value := env["PLANNER_TRIGGERS_PATH"]; value != "" {
		cfg.PlannerTriggersPath = value
	}
	if value := env["PLANNER_MAX_RETRIES"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("PLANNER_MAX_RETRIES must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("PLANNER_MAX_RETRIES must be non-negative")
		}
		cfg.MaxRetries = n
	}
	if value := env["MERGE_OPEN_PULL_REQUESTS"]; value != "" {
		b, err := parseBoolEnv(value)
		if err != nil {
			return Config{}, fmt.Errorf("MERGE_OPEN_PULL_REQUESTS: %w", err)
		}
		cfg.MergeOpenPullRequests = b
	}
	if value := env["PLANNER_EVALUATION_WINDOW"]; value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("PLANNER_EVALUATION_WINDOW must be a Go duration (e.g. \"168h\"): %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("PLANNER_EVALUATION_WINDOW must be positive")
		}
		cfg.EvaluationWindow = d
	}
	if value := env["PLANNER_ESCALATION_THRESHOLD"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("PLANNER_ESCALATION_THRESHOLD must be an integer: %w", err)
		}
		if n <= 0 {
			return Config{}, fmt.Errorf("PLANNER_ESCALATION_THRESHOLD must be positive")
		}
		cfg.EscalationThreshold = n
	}

	// TriggersCursorPath derives from the (possibly env-overridden) RunRoot
	// so a single RUN_ROOT override moves the cursor with the rest of the
	// planner's on-disk state.
	cfg.TriggersCursorPath = filepath.Join(cfg.RunRoot, "state", "triggers_cursor.json")

	return cfg, nil
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
