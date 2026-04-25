package autoloop

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
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

	// Reactive autoloop knobs (Tasks 4-7).
	QuarantineThreshold     int      // QUARANTINE_THRESHOLD, default 3
	BackendDegradeThreshold int      // BACKEND_DEGRADE_THRESHOLD, default 3
	BackendFallback         []string // BACKEND_FALLBACK, default nil (no chain)
	IncludeQuarantined      bool     // GORMES_INCLUDE_QUARANTINED, default false
	ReportRepairEnabled     bool     // GORMES_REPORT_REPAIR, default true (Task 6)
	PlannerQuarantineLimit  int      // GORMES_PLANNER_QUARANTINE_LIMIT, default 5 (Task 7)
}

func ConfigFromEnv(repoRoot string, env map[string]string) (Config, error) {
	if repoRoot == "" {
		return Config{}, fmt.Errorf("repo root is required")
	}

	cfg := Config{
		RepoRoot:      repoRoot,
		ProgressJSON:  filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json"),
		RunRoot:       filepath.Join(repoRoot, ".codex", "orchestrator"),
		Backend:       "codexu",
		Mode:          "safe",
		MaxAgents:     4,
		MaxPhase:      3,
		PriorityBoost: []string{"2.B.3", "2.B.4", "2.B.10", "2.B.11"},

		QuarantineThreshold:     3,
		BackendDegradeThreshold: 3,
		BackendFallback:         nil,
		IncludeQuarantined:      false,
		ReportRepairEnabled:     true,
		PlannerQuarantineLimit:  5,
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

	return cfg, nil
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
