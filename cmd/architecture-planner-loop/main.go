package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/TrebuchetDynamics/gormes-agent/internal/architectureplanner"
	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

var commandStdout io.Writer = os.Stdout
var commandRunner autoloop.Runner = autoloop.ExecRunner{}

const usage = "usage: architecture-planner-loop run [--dry-run] [--codexu|--claudeu] [--mode safe|full|unattended] | status | show-report | doctor"

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
	if len(args) == 0 {
		args = []string{"run"}
	}

	switch args[0] {
	case "run":
		opts, err := parseRunOptions(args[1:])
		if err != nil {
			return err
		}
		if opts.help {
			_, err := fmt.Fprintln(commandStdout, usage)
			return err
		}
		cfg, err := architectureplanner.ConfigFromEnv(root, plannerEnv(opts))
		if err != nil {
			return err
		}
		summary, err := architectureplanner.RunOnce(context.Background(), architectureplanner.RunOptions{
			Config: cfg,
			Runner: commandRunner,
			DryRun: opts.dryRun,
		})
		if err != nil {
			return err
		}
		return printRunSummary(summary, opts.dryRun)
	case "status":
		cfg, err := architectureplanner.ConfigFromEnv(root, plannerEnv(runOptions{}))
		if err != nil {
			return err
		}
		return printStatus(filepath.Join(cfg.RunRoot, "planner_state.json"))
	case "show-report":
		cfg, err := architectureplanner.ConfigFromEnv(root, plannerEnv(runOptions{}))
		if err != nil {
			return err
		}
		return printFile(filepath.Join(cfg.RunRoot, "latest_planner_report.md"))
	case "doctor":
		cfg, err := architectureplanner.ConfigFromEnv(root, plannerEnv(runOptions{}))
		if err != nil {
			return err
		}
		return doctor(cfg)
	case "--help", "-h", "help":
		_, err := fmt.Fprintln(commandStdout, usage)
		return err
	default:
		return fmt.Errorf(usage)
	}
}

type runOptions struct {
	dryRun  bool
	backend string
	mode    string
	help    bool
}

func parseRunOptions(args []string) (runOptions, error) {
	opts := runOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			opts.dryRun = true
		case "--codexu":
			opts.backend = "codexu"
		case "--claudeu":
			opts.backend = "claudeu"
		case "--mode":
			if i+1 >= len(args) {
				return runOptions{}, fmt.Errorf(usage)
			}
			i++
			opts.mode = args[i]
		case "--help", "-h":
			opts.help = true
		default:
			return runOptions{}, fmt.Errorf(usage)
		}
	}
	return opts, nil
}

func plannerEnv(opts runOptions) map[string]string {
	env := map[string]string{}
	for _, key := range []string{
		"PROGRESS_JSON",
		"RUN_ROOT",
		"BACKEND",
		"MODE",
		"HERMES_DIR",
		"GBRAIN_DIR",
		"HONCHO_DIR",
		"HERMES_REPO_URL",
		"GBRAIN_REPO_URL",
		"HONCHO_REPO_URL",
		"PLANNER_VALIDATE",
		"PLANNER_SYNC_REPOS",
	} {
		env[key] = os.Getenv(key)
	}
	if opts.backend != "" {
		env["BACKEND"] = opts.backend
	}
	if opts.mode != "" {
		env["MODE"] = opts.mode
	}
	return env
}

func printRunSummary(summary architectureplanner.RunSummary, dryRun bool) error {
	label := "architecture planner run"
	if dryRun {
		label = "architecture planner dry-run"
	}
	fmt.Fprintf(commandStdout, "%s\n", label)
	fmt.Fprintf(commandStdout, "backend: %s\n", summary.Backend)
	fmt.Fprintf(commandStdout, "mode: %s\n", summary.Mode)
	fmt.Fprintf(commandStdout, "progress: %s\n", summary.ProgressJSON)
	fmt.Fprintf(commandStdout, "progress items: %d\n", summary.ProgressItems)
	fmt.Fprintf(commandStdout, "run root: %s\n", summary.RunRoot)
	for _, root := range summary.SourceRoots {
		status := "missing"
		if root.Exists {
			status = "present"
		}
		fmt.Fprintf(commandStdout, "- %s: %s (%s, files=%d)\n", root.Name, root.Path, status, root.FileCount)
	}
	if !dryRun {
		fmt.Fprintf(commandStdout, "report: %s\n", summary.ReportPath)
		fmt.Fprintf(commandStdout, "state: %s\n", summary.StatePath)
	}
	return nil
}

func printStatus(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	fmt.Fprintf(commandStdout, "Last run UTC: %s\n", stateString(state, "last_run_utc"))
	fmt.Fprintf(commandStdout, "Backend: %s\n", stateString(state, "backend"))
	fmt.Fprintf(commandStdout, "Mode: %s\n", stateString(state, "mode"))
	fmt.Fprintf(commandStdout, "Progress JSON: %s\n", stateString(state, "progress_json"))
	fmt.Fprintf(commandStdout, "Report: %s\n", stateString(state, "report_path"))
	fmt.Fprintf(commandStdout, "Context: %s\n", stateString(state, "context_path"))
	return nil
}

func stateString(state map[string]any, key string) string {
	if value, ok := state[key].(string); ok && value != "" {
		return value
	}
	return "unknown"
}

func printFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = commandStdout.Write(data)
	return err
}

func doctor(cfg architectureplanner.Config) error {
	for _, path := range []string{cfg.RepoRoot, filepath.Dir(cfg.ProgressJSON), cfg.HermesDir, cfg.GBrainDir, cfg.HonchoDir} {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", path)
		}
	}
	if _, err := os.Stat(cfg.ProgressJSON); err != nil {
		return err
	}
	if _, err := exec.LookPath(cfg.Backend); err != nil {
		return fmt.Errorf("backend %q not found on PATH: %w", cfg.Backend, err)
	}
	_, err := fmt.Fprintln(commandStdout, "doctor: ok")
	return err
}
