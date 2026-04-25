package plannerloop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
)

type ExternalRepo struct {
	Name     string
	Path     string
	CloneURL string
}

type RepoSyncResult struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Action   string `json:"action"`
	CloneURL string `json:"clone_url,omitempty"`
	Output   string `json:"output,omitempty"`
}

func SyncExternalRepos(ctx context.Context, cfg Config, runner builderloop.Runner) ([]RepoSyncResult, error) {
	if runner == nil {
		runner = builderloop.ExecRunner{}
	}

	results := make([]RepoSyncResult, 0, len(cfg.ExternalRepos()))
	for _, repo := range cfg.ExternalRepos() {
		result, err := syncExternalRepo(ctx, cfg, runner, repo)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func syncExternalRepo(ctx context.Context, cfg Config, runner builderloop.Runner, repo ExternalRepo) (RepoSyncResult, error) {
	info, err := os.Stat(repo.Path)
	if err != nil {
		if os.IsNotExist(err) {
			if repo.CloneURL == "" {
				return RepoSyncResult{}, fmt.Errorf("%s clone URL is required", repo.Name)
			}
			if err := os.MkdirAll(filepath.Dir(repo.Path), 0o755); err != nil {
				return RepoSyncResult{}, err
			}
			result := runner.Run(ctx, builderloop.Command{
				Name: "git",
				Args: []string{"clone", repo.CloneURL, repo.Path},
				Dir:  cfg.RepoRoot,
			})
			if result.Err != nil {
				return RepoSyncResult{}, commandError("git clone "+repo.Name, result)
			}
			return RepoSyncResult{
				Name:     repo.Name,
				Path:     repo.Path,
				Action:   "clone",
				CloneURL: repo.CloneURL,
				Output:   commandOutput(result),
			}, nil
		}
		return RepoSyncResult{}, err
	}
	if !info.IsDir() {
		return RepoSyncResult{}, fmt.Errorf("%s exists but is not a directory: %s", repo.Name, repo.Path)
	}
	if !isGitRepoPath(repo.Path) {
		return RepoSyncResult{}, fmt.Errorf("%s exists but is not a git repository: %s", repo.Name, repo.Path)
	}

	result := runner.Run(ctx, builderloop.Command{
		Name: "git",
		Args: []string{"-C", repo.Path, "pull", "--ff-only"},
		Dir:  cfg.RepoRoot,
	})
	if result.Err != nil {
		return RepoSyncResult{}, commandError("git pull "+repo.Name, result)
	}
	return RepoSyncResult{
		Name:     repo.Name,
		Path:     repo.Path,
		Action:   "pull",
		CloneURL: repo.CloneURL,
		Output:   commandOutput(result),
	}, nil
}

func isGitRepoPath(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	return false
}

func commandOutput(result builderloop.Result) string {
	if result.Stdout != "" {
		return strings.TrimSpace(result.Stdout)
	}
	return strings.TrimSpace(result.Stderr)
}
