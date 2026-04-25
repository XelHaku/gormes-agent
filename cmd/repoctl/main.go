// Command repoctl wraps the existing internal/repoctl package as a
// standalone binary. The same verbs are still reachable via
// `builder-loop repo …` for back-compat; this entry point exists so an
// operator (or CI) can update repo metadata without dragging in the rest
// of the builder-loop's surface or environment.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/TrebuchetDynamics/gormes-agent/internal/repoctl"
)

const usage = "usage: repoctl [--repo-root <path>] {benchmark record|readme update}"

var errParse = errors.New("parse error")

func main() {
	if err := run(os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, errParse) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(stdout, stderr io.Writer, args []string) error {
	args, root, err := resolveRepoRoot(args)
	if err != nil {
		return err
	}
	switch {
	case len(args) >= 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help"):
		_, err := fmt.Fprintln(stdout, usage)
		return err
	case len(args) == 2 && args[0] == "benchmark" && args[1] == "record":
		return repoctl.RecordBenchmark(repoctl.BenchmarkOptions{
			Root:   root,
			Binary: os.Getenv("BINARY_PATH"),
		})
	case len(args) == 2 && args[0] == "readme" && args[1] == "update":
		return repoctl.UpdateReadme(repoctl.ReadmeOptions{Root: root})
	default:
		return fmt.Errorf("%w\n%s", errParse, usage)
	}
}

func resolveRepoRoot(args []string) ([]string, string, error) {
	out := make([]string, 0, len(args))
	root := os.Getenv("REPO_ROOT")
	for i := 0; i < len(args); i++ {
		if args[i] == "--repo-root" {
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("%w: --repo-root requires a value\n%s", errParse, usage)
			}
			root = args[i+1]
			i++
			continue
		}
		out = append(out, args[i])
	}
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, "", err
		}
		root = cwd
	}
	return out, root, nil
}
