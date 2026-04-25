package autoloop

import (
	"fmt"
	"path/filepath"
)

func WorkerBranchName(runID string, workerID int, candidate Candidate) string {
	slug := sanitizeBranchSegment(candidate.PhaseID + "-" + candidate.SubphaseID + "-" + candidate.ItemName)
	return fmt.Sprintf("autoloop/%s/w%d/%s", runID, workerID, slug)
}

func WorkerWorktreePath(cfg Config, runID string, workerID int) string {
	runRoot := cfg.RunRoot
	if runRoot == "" {
		runRoot = filepath.Join(cfg.RepoRoot, ".codex", "orchestrator")
	}
	return filepath.Join(runRoot, "worktrees", runID, fmt.Sprintf("w%d", workerID))
}

func WorkerRepoRoot(workerRoot string, repoSubdir string) string {
	if repoSubdir == "" || repoSubdir == "." {
		return workerRoot
	}

	return filepath.Join(workerRoot, repoSubdir)
}
