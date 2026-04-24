package autoloop

import (
	"path/filepath"
	"strconv"
)

func WorkerBranchName(runID string, workerID int) string {
	return "codexu/" + runID + "/worker" + strconv.Itoa(workerID)
}

func WorkerRepoRoot(workerRoot string, repoSubdir string) string {
	if repoSubdir == "" || repoSubdir == "." {
		return workerRoot
	}

	return filepath.Join(workerRoot, repoSubdir)
}
