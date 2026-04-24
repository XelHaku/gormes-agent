package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

var commandStdout io.Writer = os.Stdout

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

	switch {
	case len(args) == 1 && args[0] == "run":
		cfg, err := autoloop.ConfigFromEnv(root, autoloopEnv())
		if err != nil {
			return err
		}
		return runAutoloop(cfg, false)
	case len(args) == 2 && args[0] == "run" && args[1] == "--dry-run":
		cfg, err := autoloop.ConfigFromEnv(root, autoloopEnv())
		if err != nil {
			return err
		}
		return runAutoloop(cfg, true)
	case len(args) == 1 && args[0] == "digest":
		digest, err := autoloop.DigestLedger(filepath.Join(digestRunRoot(root), "state", "runs.jsonl"))
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(commandStdout, digest)
		return err
	default:
		return fmt.Errorf("usage: autoloop run [--dry-run] | digest")
	}
}

func digestRunRoot(root string) string {
	if runRoot := os.Getenv("RUN_ROOT"); runRoot != "" {
		return runRoot
	}

	return filepath.Join(root, ".codex", "orchestrator")
}

func runAutoloop(cfg autoloop.Config, dryRun bool) error {
	summary, err := autoloop.RunOnce(context.Background(), autoloop.RunOptions{
		Config: cfg,
		DryRun: dryRun,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(commandStdout, "candidates: %d\nselected: %d\n", summary.Candidates, len(summary.Selected))
	for _, candidate := range summary.Selected {
		fmt.Fprintf(commandStdout, "- %s/%s %s [%s]\n", candidate.PhaseID, candidate.SubphaseID, candidate.ItemName, candidate.Status)
	}

	return nil
}

func autoloopEnv() map[string]string {
	env := map[string]string{}
	for _, key := range []string{"PROGRESS_JSON", "RUN_ROOT", "BACKEND", "MODE", "MAX_AGENTS"} {
		env[key] = os.Getenv(key)
	}

	return env
}
