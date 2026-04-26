package builderloop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func repoHasGit(repoRoot string) bool {
	if repoRoot == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(repoRoot, ".git"))
	return err == nil
}

func gitCurrentBranch(repoRoot string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git branch --show-current: %w", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", fmt.Errorf("git branch --show-current returned empty (detached HEAD?)")
	}
	return branch, nil
}

func gitCreateWorkerWorktree(repoRoot, worktreePath, branch string) error {
	if worktreePath == "" {
		return fmt.Errorf("worker worktree path is required")
	}
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return fmt.Errorf("create worker worktree parent: %w", err)
	}
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "add", "-b", branch, worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add -b %s %s: %w: %s", branch, worktreePath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitRemoveWorkerWorktree(repoRoot, worktreePath string) error {
	if worktreePath == "" {
		return nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "remove", worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove %s: %w: %s", worktreePath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitHeadSha(repoRoot string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func ensureUpstreamNotBehind(repoRoot string) error {
	if !repoHasGit(repoRoot) {
		return nil
	}

	upstreamCmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	upstreamOut, err := upstreamCmd.Output()
	if err != nil {
		// Branches without an upstream can still be local integration branches.
		return nil
	}
	upstream := strings.TrimSpace(string(upstreamOut))
	if upstream == "" {
		return nil
	}

	countCmd := exec.Command("git", "-C", repoRoot, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	countOut, err := countCmd.Output()
	if err != nil {
		return fmt.Errorf("check git upstream divergence: %w", err)
	}
	var ahead, behind int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(countOut)), "%d\t%d", &ahead, &behind); err != nil {
		return fmt.Errorf("parse git upstream divergence %q: %w", strings.TrimSpace(string(countOut)), err)
	}
	if behind > 0 {
		return fmt.Errorf("current branch is behind upstream %s (@{upstream}) by %d commit(s), ahead by %d; reconcile before running builder-loop", upstream, behind, ahead)
	}
	return nil
}

func gitChangedPaths(repoRoot, baseCommit, headCommit string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "diff", "--name-only", "-z", baseCommit, headCommit)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s %s: %w", baseCommit, headCommit, err)
	}

	var paths []string
	for _, path := range strings.Split(string(out), "\x00") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func sanitizeBranchSegment(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-_.")
	if len(out) > 60 {
		out = strings.TrimRight(out[:60], "-_.")
	}
	if out == "" {
		return "task"
	}
	return out
}
