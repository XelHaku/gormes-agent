package autoloop

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestPromoteCreatesPRWhenPushAndGHSucceed(t *testing.T) {
	runner := &FakeRunner{
		Results: []Result{
			{},
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
			Name: "gh",
			Args: []string{"pr", "create", "--fill", "--head", "codexu/run/worker1"},
			Dir:  "/tmp/repo",
		},
	}
	if !reflect.DeepEqual(runner.Commands, want) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, want)
	}
}

func TestPromoteFallsBackToCherryPickWhenPushFails(t *testing.T) {
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

func TestPromoteFallsBackToCherryPickWhenGHFails(t *testing.T) {
	runner := &FakeRunner{
		Results: []Result{
			{},
			{Err: errors.New("gh failed")},
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
			Name: "gh",
			Args: []string{"pr", "create", "--fill", "--head", "codexu/run/worker1"},
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

func TestPromoteCherryPickModeRunsOnlyCherryPick(t *testing.T) {
	runner := &FakeRunner{
		Results: []Result{
			{},
		},
	}

	err := PromoteWorker(context.Background(), PromoteOptions{
		Runner:        runner,
		RepoRoot:      "/tmp/repo",
		WorkerBranch:  "codexu/run/worker1",
		WorkerCommit:  "abc123",
		PromotionMode: "cherry-pick",
	})
	if err != nil {
		t.Fatalf("PromoteWorker() error = %v, want nil", err)
	}

	want := []Command{
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

func TestPromoteMissingRequiredFieldsReturnsErrorBeforeCommands(t *testing.T) {
	tests := []struct {
		name string
		opts PromoteOptions
		want string
	}{
		{
			name: "repo root",
			opts: PromoteOptions{
				WorkerBranch: "codexu/run/worker1",
				WorkerCommit: "abc123",
			},
			want: "repo root",
		},
		{
			name: "worker branch",
			opts: PromoteOptions{
				RepoRoot:     "/tmp/repo",
				WorkerCommit: "abc123",
			},
			want: "worker branch",
		},
		{
			name: "worker commit",
			opts: PromoteOptions{
				RepoRoot:     "/tmp/repo",
				WorkerBranch: "codexu/run/worker1",
			},
			want: "worker commit",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &FakeRunner{}
			test.opts.Runner = runner

			err := PromoteWorker(context.Background(), test.opts)
			if err == nil {
				t.Fatal("PromoteWorker() error = nil, want error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("PromoteWorker() error = %q, want containing %q", err, test.want)
			}
			if len(runner.Commands) != 0 {
				t.Fatalf("Commands = %#v, want none", runner.Commands)
			}
		})
	}
}

func TestPromoteInvalidModeReturnsErrorBeforeCommands(t *testing.T) {
	runner := &FakeRunner{}

	err := PromoteWorker(context.Background(), PromoteOptions{
		Runner:        runner,
		RepoRoot:      "/tmp/repo",
		WorkerBranch:  "codexu/run/worker1",
		WorkerCommit:  "abc123",
		PromotionMode: "merge",
	})
	if err == nil {
		t.Fatal("PromoteWorker() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid promotion mode") {
		t.Fatalf("PromoteWorker() error = %q, want containing %q", err, "invalid promotion mode")
	}
	if len(runner.Commands) != 0 {
		t.Fatalf("Commands = %#v, want none", runner.Commands)
	}
}

func TestPromoteReturnsCherryPickFailure(t *testing.T) {
	wantErr := errors.New("cherry-pick failed")
	runner := &FakeRunner{
		Results: []Result{
			{Err: wantErr},
		},
	}

	err := PromoteWorker(context.Background(), PromoteOptions{
		Runner:        runner,
		RepoRoot:      "/tmp/repo",
		WorkerBranch:  "codexu/run/worker1",
		WorkerCommit:  "abc123",
		PromotionMode: "cherry-pick",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("PromoteWorker() error = %v, want %v", err, wantErr)
	}
}
