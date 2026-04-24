package architectureplanner

import (
	"fmt"
	"path/filepath"
)

type Config struct {
	RepoRoot      string
	ProgressJSON  string
	RunRoot       string
	Backend       string
	Mode          string
	HermesDir     string
	GBrainDir     string
	HonchoDir     string
	HermesRepoURL string
	GBrainRepoURL string
	HonchoRepoURL string
	Validate      bool
	SyncRepos     bool
}

func ConfigFromEnv(repoRoot string, env map[string]string) (Config, error) {
	if repoRoot == "" {
		return Config{}, fmt.Errorf("repo root is required")
	}

	parent := filepath.Dir(repoRoot)
	cfg := Config{
		RepoRoot:      repoRoot,
		ProgressJSON:  filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json"),
		RunRoot:       filepath.Join(repoRoot, ".codex", "architecture-planner"),
		Backend:       "codexu",
		Mode:          "safe",
		HermesDir:     filepath.Join(parent, "hermes-agent"),
		GBrainDir:     filepath.Join(parent, "gbrain"),
		HonchoDir:     filepath.Join(parent, "honcho"),
		HermesRepoURL: "https://github.com/NousResearch/hermes-agent.git",
		GBrainRepoURL: "https://github.com/garrytan/gbrain.git",
		HonchoRepoURL: "https://github.com/plastic-labs/honcho",
		Validate:      true,
		SyncRepos:     true,
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

	return cfg, nil
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
