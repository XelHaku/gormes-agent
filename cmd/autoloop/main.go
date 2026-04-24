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
var serviceRunner autoloop.Runner = autoloop.ExecRunner{}

const usage = "usage: autoloop run [--dry-run] | audit | digest | service install | service install-audit | service disable legacy-timers"

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
	case len(args) >= 1 && args[0] == "run":
		runOpts, err := parseRunOptions(args[1:])
		if err != nil {
			return err
		}
		if runOpts.help {
			_, err := fmt.Fprintln(commandStdout, usage)
			return err
		}
		env := autoloopEnv()
		if runOpts.backend != "" {
			env["BACKEND"] = runOpts.backend
		}
		cfg, err := autoloop.ConfigFromEnv(root, env)
		if err != nil {
			return err
		}
		return runAutoloop(cfg, runOpts.dryRun)
	case len(args) >= 1 && args[0] == "digest":
		outputPath, err := digestOutputPath(args[1:])
		if err != nil {
			return err
		}
		digest, err := autoloop.DigestLedger(filepath.Join(digestRunRoot(root), "state", "runs.jsonl"))
		if err != nil {
			return err
		}
		if outputPath != "" {
			return os.WriteFile(outputPath, []byte(digest), 0o644)
		}
		_, err = fmt.Fprint(commandStdout, digest)
		return err
	case len(args) == 1 && args[0] == "audit":
		auditDir, err := auditReportDir()
		if err != nil {
			return err
		}
		summary, err := autoloop.WriteAuditReport(autoloop.AuditReportOptions{
			LedgerPath: filepath.Join(digestRunRoot(root), "state", "runs.jsonl"),
			AuditDir:   auditDir,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(commandStdout, summary)
		return err
	case len(args) >= 2 && args[0] == "service" && args[1] == "install":
		force, err := serviceForce(args[2:])
		if err != nil {
			return err
		}
		return installService(root, force)
	case len(args) >= 2 && args[0] == "service" && args[1] == "install-audit":
		force, err := serviceForce(args[2:])
		if err != nil {
			return err
		}
		return installAuditService(root, force)
	case len(args) == 3 && args[0] == "service" && args[1] == "disable" && args[2] == "legacy-timers":
		return autoloop.DisableLegacyTimers(context.Background(), serviceRunner)
	default:
		return fmt.Errorf(usage)
	}
}

type runOptions struct {
	dryRun  bool
	backend string
	help    bool
}

func parseRunOptions(args []string) (runOptions, error) {
	opts := runOptions{}
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			opts.dryRun = true
		case "--codexu":
			opts.backend = "codexu"
		case "--claudeu":
			opts.backend = "claudeu"
		case "--opencode":
			opts.backend = "opencode"
		case "--help", "-h":
			opts.help = true
		default:
			return runOptions{}, fmt.Errorf(usage)
		}
	}

	return opts, nil
}

func digestOutputPath(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) == 2 && args[0] == "--output" && args[1] != "" {
		return args[1], nil
	}
	return "", fmt.Errorf(usage)
}

func serviceForce(args []string) (bool, error) {
	force := os.Getenv("FORCE") == "1"
	for _, arg := range args {
		if arg != "--force" {
			return false, fmt.Errorf(usage)
		}
		force = true
	}

	return force, nil
}

func installService(root string, force bool) error {
	unitDir, err := systemdUserUnitDir()
	if err != nil {
		return err
	}

	return autoloop.InstallService(context.Background(), autoloop.ServiceInstallOptions{
		Runner:       serviceRunner,
		UnitDir:      unitDir,
		UnitName:     "gormes-orchestrator.service",
		AutoloopPath: orchestratorWrapperPath(root),
		WorkDir:      root,
		ExecArgs:     []string{},
		AutoStart:    autoStart(),
		Force:        force,
	})
}

func installAuditService(root string, force bool) error {
	unitDir, err := systemdUserUnitDir()
	if err != nil {
		return err
	}

	return autoloop.InstallAuditService(context.Background(), autoloop.AuditServiceInstallOptions{
		Runner:    serviceRunner,
		UnitDir:   unitDir,
		UnitName:  "gormes-orchestrator-audit.service",
		TimerName: "gormes-orchestrator-audit.timer",
		AuditPath: auditWrapperPath(root),
		WorkDir:   root,
		AutoStart: autoStart(),
		Force:     force,
	})
}

func autoStart() bool {
	return os.Getenv("AUTO_START") != "0"
}

func orchestratorWrapperPath(root string) string {
	if path := os.Getenv("ORCHESTRATOR_PATH"); path != "" {
		return path
	}
	return filepath.Join(root, "scripts", "gormes-auto-codexu-orchestrator.sh")
}

func auditWrapperPath(root string) string {
	if path := os.Getenv("AUDIT_PATH"); path != "" {
		return path
	}
	return filepath.Join(root, "scripts", "orchestrator", "audit.sh")
}

func auditReportDir() (string, error) {
	if auditDir := os.Getenv("AUDIT_DIR"); auditDir != "" {
		return auditDir, nil
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".cache", "gormes-orchestrator-audit"), nil
	}
	return "", fmt.Errorf("cannot determine audit directory: set AUDIT_DIR or HOME")
}

func systemdUserUnitDir() (string, error) {
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "systemd", "user"), nil
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", "systemd", "user"), nil
	}

	return "", fmt.Errorf("cannot determine systemd user unit directory: set XDG_CONFIG_HOME or HOME")
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
