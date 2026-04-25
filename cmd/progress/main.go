// Command progress is a thin wrapper around internal/progressctl: it
// validates the canonical progress.json and regenerates the markered docs
// the planner-builder loop reads from. Operators can call it directly via
// `go run ./cmd/progress validate` instead of the longer
// `go run ./cmd/builder-loop progress validate`; both routes share the
// same code.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progressctl"
)

const usage = "usage: progress [--repo-root <path>] {validate [--format text|json]|write}"

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
	if len(args) == 0 {
		return fmt.Errorf("%w\n%s", errParse, usage)
	}
	switch args[0] {
	case "--help", "-h", "help":
		_, err := fmt.Fprintln(stdout, usage)
		return err
	case "validate":
		format, err := parseFormat(args[1:])
		if err != nil {
			return err
		}
		return progressctl.Validate(stdout, root, format)
	case "write":
		if len(args) != 1 {
			return fmt.Errorf("%w\n%s", errParse, usage)
		}
		return progressctl.Write(stdout, root)
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

func parseFormat(args []string) (string, error) {
	format := "text"
	for i := 0; i < len(args); i++ {
		if args[i] == "--format" {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%w: --format requires a value\n%s", errParse, usage)
			}
			switch args[i+1] {
			case "text", "json":
				format = args[i+1]
			default:
				return "", fmt.Errorf("%w: --format must be text or json (got %q)\n%s",
					errParse, args[i+1], usage)
			}
			i++
			continue
		}
		return "", fmt.Errorf("%w: unexpected argument %q\n%s", errParse, args[i], usage)
	}
	return format, nil
}
