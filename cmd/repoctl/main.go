package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TrebuchetDynamics/gormes-agent/internal/repoctl"
)

const usage = "usage: repoctl benchmark record"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 || args[0] != "benchmark" || args[1] != "record" {
		return errors.New(usage)
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}
	return repoctl.RecordBenchmark(repoctl.BenchmarkOptions{
		Root:   root,
		Binary: filepath.Join(root, "bin", "gormes"),
	})
}
