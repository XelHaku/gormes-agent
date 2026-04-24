package autoloop

import (
	"fmt"
	"path/filepath"
	"strconv"
)

type Config struct {
	RepoRoot     string
	ProgressJSON string
	RunRoot      string
	Backend      string
	Mode         string
	MaxAgents    int
}

func ConfigFromEnv(repoRoot string, env map[string]string) (Config, error) {
	if repoRoot == "" {
		return Config{}, fmt.Errorf("repo root is required")
	}

	cfg := Config{
		RepoRoot:     repoRoot,
		ProgressJSON: filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json"),
		RunRoot:      filepath.Join(repoRoot, ".codex", "orchestrator"),
		Backend:      "codexu",
		Mode:         "safe",
		MaxAgents:    4,
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

	return cfg, nil
}
