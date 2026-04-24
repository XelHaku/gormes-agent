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
		if maxPhase < 1 {
			return Config{}, fmt.Errorf("MAX_PHASE must be at least 1")
		}
		cfg.MaxPhase = maxPhase
	}
	if value := env["PRIORITY_BOOST"]; value != "" {
		cfg.PriorityBoost = splitCSV(value)
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
