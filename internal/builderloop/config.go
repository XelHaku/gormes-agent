package builderloop

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	RepoRoot      string
	ProgressJSON  string
	RunRoot       string
	Backend       string
	Mode          string
	MaxAgents     int
	MaxPhase      int
	PriorityBoost []string
	// BackendTimeout bounds each LLM backend invocation launched by
	// autoloop workers or repair agents. A stuck codexu/claudeu/opencode
	// child must not park the infinite loop forever. Sourced from
	// AUTOLOOP_BACKEND_TIMEOUT (Go time.ParseDuration syntax, e.g. "30m");
	// default 30 minutes.
	BackendTimeout time.Duration

	// Reactive autoloop knobs (Tasks 4-7).
	QuarantineThreshold     int      // QUARANTINE_THRESHOLD, default 3
	BackendDegradeThreshold int      // BACKEND_DEGRADE_THRESHOLD, default 3
	BackendFallback         []string // BACKEND_FALLBACK, default nil (no chain)
	IncludeQuarantined      bool     // GORMES_INCLUDE_QUARANTINED, default false
	IncludeNeedsHuman       bool     // GORMES_INCLUDE_NEEDS_HUMAN, default false (L5)
	ReportRepairEnabled     bool     // GORMES_REPORT_REPAIR, default true (Task 6)
	PlannerQuarantineLimit  int      // GORMES_PLANNER_QUARANTINE_LIMIT, default 5 (Task 7)
	MergeOpenPullRequests   bool     // MERGE_OPEN_PULL_REQUESTS, default true
	PRConflictAction        string   // PR_INTAKE_CONFLICT_ACTION, default close
	AutoCommitDirtyWorktree bool     // AUTO_COMMIT_DIRTY_WORKTREE, default true for CLI cycles

	PostPromotionVerifyCommands []string // POST_PROMOTION_VERIFY_COMMANDS, default full-suite gate
	PostPromotionRepairEnabled  bool     // POST_PROMOTION_REPAIR, default true
	PostPromotionRepairAttempts int      // POST_PROMOTION_REPAIR_ATTEMPTS, default 1

	// PrePromotionVerifyCommands runs ON THE WORKER'S WORKTREE before the
	// worker's commit is cherry-picked onto main. Empty = disabled (current
	// post-promotion-only behavior preserved). When set (e.g. via env
	// PRE_PROMOTION_VERIFY_COMMANDS=go test ./...), a failing verify aborts
	// the worker_failed path BEFORE main is touched, so main never enters a
	// briefly-broken state and any repair work happens in the worktree.
	// Compose with PostPromotionVerifyCommands for defense-in-depth (pre
	// catches per-worker breakage; post catches cross-worker integration).
	PrePromotionVerifyCommands []string // PRE_PROMOTION_VERIFY_COMMANDS
	// PrePromotionRepairEnabled gates the LLM-driven repair attempt that
	// runs in the worker's worktree when the pre-promotion verify fails.
	// Default: true. Set PRE_PROMOTION_REPAIR=0 to skip repair and treat
	// the first failure as a terminal worker_failed outcome.
	PrePromotionRepairEnabled bool // PRE_PROMOTION_REPAIR
	// PrePromotionRepairAttempts caps the number of repair iterations per
	// worker before giving up. Default: 1 (one repair pass + one re-verify).
	PrePromotionRepairAttempts int // PRE_PROMOTION_REPAIR_ATTEMPTS

	// PlannerTriggersPath is the JSONL ledger autoloop appends to when a
	// row's quarantine state changes in a way the planner needs to react
	// to. The planner watches this file (via systemd .path unit) and
	// consumes events on its next run via plannertriggers.LoadCursor /
	// ReadTriggersSinceCursor. Default lives under .codex so it is
	// co-located with the planner's other on-disk state.
	PlannerTriggersPath string // PLANNER_TRIGGERS_PATH
}

func ConfigFromEnv(repoRoot string, env map[string]string) (Config, error) {
	if repoRoot == "" {
		return Config{}, fmt.Errorf("repo root is required")
	}

	cfg := Config{
		RepoRoot:       repoRoot,
		ProgressJSON:   filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json"),
		RunRoot:        filepath.Join(repoRoot, ".codex", "orchestrator"),
		Backend:        "codexu",
		Mode:           "safe",
		MaxAgents:      4,
		MaxPhase:       3,
		PriorityBoost:  []string{"2.B.3", "2.B.4", "2.B.10", "2.B.11"},
		BackendTimeout: 30 * time.Minute,

		QuarantineThreshold:     3,
		BackendDegradeThreshold: 3,
		BackendFallback:         nil,
		IncludeQuarantined:      false,
		IncludeNeedsHuman:       false,
		ReportRepairEnabled:     true,
		PlannerQuarantineLimit:  5,
		MergeOpenPullRequests:   true,
		PRConflictAction:        PRConflictActionClose,
		AutoCommitDirtyWorktree: true,

		PostPromotionVerifyCommands: defaultPostPromotionVerifyCommands(),
		PostPromotionRepairEnabled:  true,
		PostPromotionRepairAttempts: 1,
		PrePromotionVerifyCommands:  nil,
		PrePromotionRepairEnabled:   true,
		PrePromotionRepairAttempts:  1,

		PlannerTriggersPath: filepath.Join(repoRoot, ".codex", "architecture-planner", "triggers.jsonl"),
	}

	if value := env["PROGRESS_JSON"]; value != "" {
		cfg.ProgressJSON = value
	}
	if value := env["RUN_ROOT"]; value != "" {
		cfg.RunRoot = value
	}
	if value := env["BACKEND"]; value != "" {
		cfg.Backend = value
	}
	if value := env["MODE"]; value != "" {
		cfg.Mode = value
	}
	if value := env["MAX_AGENTS"]; value != "" {
		maxAgents, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_AGENTS must be an integer: %w", err)
		}
		if maxAgents < 1 {
			return Config{}, fmt.Errorf("MAX_AGENTS must be at least 1")
		}
		cfg.MaxAgents = maxAgents
	}
	if value := env["MAX_PHASE"]; value != "" {
		maxPhase, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_PHASE must be an integer: %w", err)
		}
		if maxPhase < 0 {
			return Config{}, fmt.Errorf("MAX_PHASE must be non-negative")
		}
		cfg.MaxPhase = maxPhase
	}
	if value := env["PRIORITY_BOOST"]; value != "" {
		cfg.PriorityBoost = splitCSV(value)
	}
	if value := env["AUTOLOOP_BACKEND_TIMEOUT"]; value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("AUTOLOOP_BACKEND_TIMEOUT must be a Go duration (e.g. \"30m\"): %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("AUTOLOOP_BACKEND_TIMEOUT must be positive")
		}
		cfg.BackendTimeout = d
	}

	if value := env["QUARANTINE_THRESHOLD"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("QUARANTINE_THRESHOLD must be an integer: %w", err)
		}
		if n < 1 {
			return Config{}, fmt.Errorf("QUARANTINE_THRESHOLD must be at least 1")
		}
		cfg.QuarantineThreshold = n
	}
	if value := env["BACKEND_DEGRADE_THRESHOLD"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("BACKEND_DEGRADE_THRESHOLD must be an integer: %w", err)
		}
		if n < 1 {
			return Config{}, fmt.Errorf("BACKEND_DEGRADE_THRESHOLD must be at least 1")
		}
		cfg.BackendDegradeThreshold = n
	}
	if value, ok := env["BACKEND_FALLBACK"]; ok {
		// Empty string explicitly clears the chain to no fallback (back-compat).
		cfg.BackendFallback = splitCSV(value)
	}
	if value := env["GORMES_INCLUDE_QUARANTINED"]; value != "" {
		b, err := parseBoolEnv(value)
		if err != nil {
			return Config{}, fmt.Errorf("GORMES_INCLUDE_QUARANTINED: %w", err)
		}
		cfg.IncludeQuarantined = b
	}
	if value := env["GORMES_INCLUDE_NEEDS_HUMAN"]; value != "" {
		b, err := parseBoolEnv(value)
		if err != nil {
			return Config{}, fmt.Errorf("GORMES_INCLUDE_NEEDS_HUMAN: %w", err)
		}
		cfg.IncludeNeedsHuman = b
	}
	if value := env["GORMES_REPORT_REPAIR"]; value != "" {
		b, err := parseBoolEnv(value)
		if err != nil {
			return Config{}, fmt.Errorf("GORMES_REPORT_REPAIR: %w", err)
		}
		cfg.ReportRepairEnabled = b
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
	if value := env["MERGE_OPEN_PULL_REQUESTS"]; value != "" {
		b, err := parseBoolEnv(value)
		if err != nil {
			return Config{}, fmt.Errorf("MERGE_OPEN_PULL_REQUESTS: %w", err)
		}
		cfg.MergeOpenPullRequests = b
	}
	if value := env["PR_INTAKE_CONFLICT_ACTION"]; value != "" {
		action, err := parsePRConflictAction(value)
		if err != nil {
			return Config{}, fmt.Errorf("PR_INTAKE_CONFLICT_ACTION: %w", err)
		}
		cfg.PRConflictAction = action
	}
	if value := env["AUTO_COMMIT_DIRTY_WORKTREE"]; value != "" {
		b, err := parseBoolEnv(value)
		if err != nil {
			return Config{}, fmt.Errorf("AUTO_COMMIT_DIRTY_WORKTREE: %w", err)
		}
		cfg.AutoCommitDirtyWorktree = b
	}
	if value := env["POST_PROMOTION_VERIFY_COMMANDS"]; value != "" {
		commands := splitCommandList(value)
		if len(commands) == 0 {
			return Config{}, fmt.Errorf("POST_PROMOTION_VERIFY_COMMANDS must contain at least one command")
		}
		cfg.PostPromotionVerifyCommands = commands
	}
	if value := env["PRE_PROMOTION_VERIFY_COMMANDS"]; value != "" {
		commands := splitCommandList(value)
		if len(commands) == 0 {
			return Config{}, fmt.Errorf("PRE_PROMOTION_VERIFY_COMMANDS must contain at least one command")
		}
		cfg.PrePromotionVerifyCommands = commands
	}
	if value := env["PRE_PROMOTION_REPAIR"]; value != "" {
		b, err := parseBoolEnv(value)
		if err != nil {
			return Config{}, fmt.Errorf("PRE_PROMOTION_REPAIR: %w", err)
		}
		cfg.PrePromotionRepairEnabled = b
	}
	if value := env["PRE_PROMOTION_REPAIR_ATTEMPTS"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("PRE_PROMOTION_REPAIR_ATTEMPTS must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("PRE_PROMOTION_REPAIR_ATTEMPTS must be non-negative")
		}
		cfg.PrePromotionRepairAttempts = n
	}
	if value := env["POST_PROMOTION_REPAIR"]; value != "" {
		b, err := parseBoolEnv(value)
		if err != nil {
			return Config{}, fmt.Errorf("POST_PROMOTION_REPAIR: %w", err)
		}
		cfg.PostPromotionRepairEnabled = b
	}
	if value := env["POST_PROMOTION_REPAIR_ATTEMPTS"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("POST_PROMOTION_REPAIR_ATTEMPTS must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("POST_PROMOTION_REPAIR_ATTEMPTS must be non-negative")
		}
		cfg.PostPromotionRepairAttempts = n
	}
	if value := env["PLANNER_TRIGGERS_PATH"]; value != "" {
		cfg.PlannerTriggersPath = value
	}

	return cfg, nil
}

func defaultPostPromotionVerifyCommands() []string {
	return []string{
		"go test ./... -count=1",
		"(cd www.gormes.ai && go test ./... -count=1)",
		"go run ./cmd/builder-loop progress validate",
		"go run ./cmd/builder-loop run --dry-run",
		"(cd www.gormes.ai && npm run test:e2e -- --reporter=line)",
	}
}

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

func splitCommandList(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	var out []string
	for _, line := range strings.Split(value, "\n") {
		for _, part := range strings.Split(line, ";;") {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

// parseBoolEnv accepts the common shell idioms for booleans. Whitespace is
// trimmed; case is normalised. Returns an error on unknown values rather
// than silently defaulting so misconfigurations surface loudly.
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
	case PRConflictActionClose:
		return PRConflictActionClose, nil
	case PRConflictActionSkip:
		return PRConflictActionSkip, nil
	}
	return "", fmt.Errorf("invalid action %q (want close or skip)", value)
}
