package main

import (
	"context"
	"fmt"
	"os"

	"github.com/TrebuchetDynamics/gormes-agent/internal/repoctl"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if len(args) == 2 && args[0] == "benchmark" && args[1] == "record" {
		return repoctl.RecordBenchmark(repoctl.BenchmarkOptions{Root: root, Binary: os.Getenv("BINARY_PATH")})
	}
	if len(args) == 2 && args[0] == "progress" && args[1] == "sync" {
		return repoctl.SyncProgress(repoctl.ProgressOptions{Root: root})
	}
	if len(args) == 2 && args[0] == "compat" && args[1] == "go122" {
		return repoctl.CheckGo122(context.Background(), repoctl.Go122Options{Root: root})
	}
	if len(args) == 2 && args[0] == "readme" && args[1] == "update" {
		return repoctl.UpdateReadme(repoctl.ReadmeOptions{Root: root})
	}
	return fmt.Errorf("usage: repoctl benchmark record | progress sync | compat go122 | readme update")
}
