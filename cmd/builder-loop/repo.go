package main

import (
	"fmt"
	"os"

	"github.com/TrebuchetDynamics/gormes-agent/internal/repoctl"
)

func runRepo(root string, args []string) error {
	switch {
	case len(args) == 2 && args[0] == "benchmark" && args[1] == "record":
		return repoctl.RecordBenchmark(repoctl.BenchmarkOptions{
			Root:   root,
			Binary: os.Getenv("BINARY_PATH"),
		})
	case len(args) == 2 && args[0] == "readme" && args[1] == "update":
		return repoctl.UpdateReadme(repoctl.ReadmeOptions{Root: root})
	default:
		return fmt.Errorf(usage)
	}
}
