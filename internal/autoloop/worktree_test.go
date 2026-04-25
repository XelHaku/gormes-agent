package autoloop

import (
	"path/filepath"
	"testing"
)

func TestWorkerBranchNameIncludesRunWorkerAndSlug(t *testing.T) {
	got := WorkerBranchName("20260101T000000Z", 3, Candidate{
		PhaseID:    "2",
		SubphaseID: "2.B.3",
		ItemName:   "Slack CommandRegistry parser wiring",
	})
	want := "autoloop/20260101T000000Z/w3/2-2.b.3-slack-commandregistry-parser-wiring"

	if got != want {
		t.Fatalf("WorkerBranchName() = %q, want %q", got, want)
	}
}

func TestWorkerBranchNameTruncatesLongSlug(t *testing.T) {
	got := WorkerBranchName("run", 1, Candidate{
		PhaseID:    "2",
		SubphaseID: "2.F.3",
		ItemName:   "this is a deliberately very long item name that should be truncated to keep the branch reasonable",
	})

	if len(got) > len("autoloop/run/w1/")+60 {
		t.Fatalf("WorkerBranchName() = %q, want slug truncated to <=60 chars", got)
	}
}

func TestWorkerWorktreePathUsesRunRoot(t *testing.T) {
	got := WorkerWorktreePath(Config{
		RepoRoot: "/repo",
		RunRoot:  "/repo/.codex/orchestrator",
	}, "20260425T014000Z", 2)
	want := filepath.Join("/repo", ".codex", "orchestrator", "worktrees", "20260425T014000Z", "w2")

	if got != want {
		t.Fatalf("WorkerWorktreePath() = %q, want %q", got, want)
	}
}

func TestWorkerRepoRootHonorsSubdir(t *testing.T) {
	tests := []struct {
		name       string
		workerRoot string
		repoSubdir string
		want       string
	}{
		{
			name:       "root",
			workerRoot: "/tmp/wt/worker1",
			repoSubdir: ".",
			want:       "/tmp/wt/worker1",
		},
		{
			name:       "subdir",
			workerRoot: "/tmp/wt/worker1",
			repoSubdir: "gormes",
			want:       "/tmp/wt/worker1/gormes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := WorkerRepoRoot(test.workerRoot, test.repoSubdir)
			if got != test.want {
				t.Fatalf("WorkerRepoRoot() = %q, want %q", got, test.want)
			}
		})
	}
}
