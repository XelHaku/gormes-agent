package architectureplanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

type ExternalRepo struct {
	Name     string
	Path     string
	CloneURL string
}

func SyncExternalRepos(ctx context.Context, cfg Config, runner autoloop.Runner) error {
	if runner == nil {
		runner = autoloop.ExecRunner{}
	}

	for _, repo := range cfg.ExternalRepos() {
		if err := syncExternalRepo(ctx, cfg, runner, repo); err != nil {
			return err
		}
	}
	return nil
}

func syncExternalRepo(ctx context.Context, cfg Config, runner autoloop.Runner, repo ExternalRepo) error {
	info, err := os.Stat(repo.Path)
	if err != nil {
		if os.IsNotExist(err) {
			if repo.CloneURL == "" {
				return fmt.Errorf("%s clone URL is required", repo.Name)
			}
			if err := os.MkdirAll(filepath.Dir(repo.Path), 0o755); err != nil {
				return err
			}
			result := runner.Run(ctx, autoloop.Command{
				Name: "git",
				Args: []string{"clone", repo.CloneURL, repo.Path},
				Dir:  cfg.RepoRoot,
			})
			if result.Err != nil {
				return commandError("git clone "+repo.Name, result)
			}
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists but is not a directory: %s", repo.Name, repo.Path)
	}
	if !isGitRepoPath(repo.Path) {
		return fmt.Errorf("%s exists but is not a git repository: %s", repo.Name, repo.Path)
	}

	result := runner.Run(ctx, autoloop.Command{
		Name: "git",
		Args: []string{"-C", repo.Path, "pull", "--ff-only"},
		Dir:  cfg.RepoRoot,
	})
	if result.Err != nil {
		return commandError("git pull "+repo.Name, result)
	}
	return nil
}

func isGitRepoPath(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	return false
}
