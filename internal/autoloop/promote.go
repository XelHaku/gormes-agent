package autoloop

import (
	"context"
	"errors"
	"fmt"
)

type PromoteOptions struct {
	Runner        Runner
	RepoRoot      string
	WorkerBranch  string
	WorkerCommit  string
	PromotionMode string
}

func PromoteWorker(ctx context.Context, opts PromoteOptions) error {
	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	mode := opts.PromotionMode
	if mode == "" {
		mode = "pr"
	}

	if opts.RepoRoot == "" {
		return errors.New("repo root is required")
	}
	if opts.WorkerBranch == "" {
		return errors.New("worker branch is required")
	}
	if opts.WorkerCommit == "" {
		return errors.New("worker commit is required")
	}
	if mode != "pr" && mode != "cherry-pick" {
		return fmt.Errorf("invalid promotion mode: %s", mode)
	}

	if mode == "cherry-pick" {
		return cherryPick(ctx, runner, opts.RepoRoot, opts.WorkerCommit)
	}

	push := runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"push", "origin", opts.WorkerBranch},
		Dir:  opts.RepoRoot,
	})
	if push.Err != nil {
		return cherryPick(ctx, runner, opts.RepoRoot, opts.WorkerCommit)
	}

	pr := runner.Run(ctx, Command{
		Name: "gh",
		Args: []string{"pr", "create", "--fill", "--head", opts.WorkerBranch},
		Dir:  opts.RepoRoot,
	})
	if pr.Err != nil {
		return cherryPick(ctx, runner, opts.RepoRoot, opts.WorkerCommit)
	}

	return nil
}

func cherryPick(ctx context.Context, runner Runner, repoRoot string, workerCommit string) error {
	result := runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"cherry-pick", "-Xtheirs", workerCommit},
		Dir:  repoRoot,
	})

	return result.Err
}
