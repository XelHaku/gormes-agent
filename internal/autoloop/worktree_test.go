package autoloop

import "testing"

func TestWorkerBranchName(t *testing.T) {
	got := WorkerBranchName("run-1", 3)
	want := "codexu/run-1/worker3"

	if got != want {
		t.Fatalf("WorkerBranchName() = %q, want %q", got, want)
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
