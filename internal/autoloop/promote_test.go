package autoloop

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPromoteFallsBackToCherryPickWhenGHFails(t *testing.T) {
	runner := &FakeRunner{
		Results: []Result{
			{Err: errors.New("push failed")},
			{},
		},
	}

	err := PromoteWorker(context.Background(), PromoteOptions{
		Runner:       runner,
		RepoRoot:     "/tmp/repo",
		WorkerBranch: "codexu/run/worker1",
		WorkerCommit: "abc123",
	})
	if err != nil {
		t.Fatalf("PromoteWorker() error = %v, want nil", err)
	}

	want := []Command{
		{
			Name: "git",
			Args: []string{"push", "origin", "codexu/run/worker1"},
			Dir:  "/tmp/repo",
		},
		{
			Name: "git",
			Args: []string{"cherry-pick", "-Xtheirs", "abc123"},
			Dir:  "/tmp/repo",
		},
	}
	if !reflect.DeepEqual(runner.Commands, want) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, want)
	}
}
