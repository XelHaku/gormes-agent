package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
	"github.com/TrebuchetDynamics/gormes-agent/internal/cmdrunner"
)

var commandStdout io.Writer = os.Stdout
var serviceRunner cmdrunner.Runner = cmdrunner.ExecRunner{}

const usage = "usage: builder-loop [--repo-root <path>] run [--dry-run] [--backend codexu|claudeu|opencode] | progress validate | progress write | repo benchmark record | repo readme update | audit | digest [--output <path>] [--force] | service install | service install-audit | service disable legacy-timers"

// subUsage maps each subcommand to its own help text. --help/-h on a
// subcommand prints the matching entry instead of the giant top-level usage,
// so an operator typing `builder-loop digest --help` sees just the digest
// flags rather than the full builder-loop surface.
var subUsage = map[string]string{
	"run":                            "usage: builder-loop run [--dry-run] [--backend codexu|claudeu|opencode]",
	"progress":                       "usage: builder-loop progress {validate|write}",
	"progress validate":              "usage: builder-loop progress validate",
	"progress write":                 "usage: builder-loop progress write",
	"repo":                           "usage: builder-loop repo {benchmark record|readme update}",
	"repo benchmark":                 "usage: builder-loop repo benchmark record",
	"repo readme":                    "usage: builder-loop repo readme update",
	"audit":                          "usage: builder-loop audit",
	"digest":                         "usage: builder-loop digest [--output <path>] [--force]",
	"service":                        "usage: builder-loop service {install|install-audit|disable legacy-timers} [--force]",
	"service install":                "usage: builder-loop service install [--force]",
	"service install-audit":          "usage: builder-loop service install-audit [--force]",
	"service disable":                "usage: builder-loop service disable legacy-timers",
	"service disable legacy-timers":  "usage: builder-loop service disable legacy-timers",
}

// supportedBuilderBackends lists the backends the run subcommand accepts via
// --backend. The same names are accepted via the BACKEND environment
// variable downstream.
var supportedBuilderBackends = []string{"codexu", "claudeu", "opencode"}

// errParse marks parser-level failures so main() can map them to exit code 2.
var errParse = errors.New("parse error")

// wantsHelp returns true if any arg is --help or -h. Subcommand handlers
// short-circuit on this so help routing is consistent across the binary.
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// printHelp writes the help text for key (or the global usage if key is
// missing from subUsage) to stdout. Help is intentionally exit-0 output, so
// callers return nil after invoking this.
func printHelp(key string) error {
	help, ok := subUsage[key]
	if !ok {
		help = usage
	}
	_, err := fmt.Fprintln(commandStdout, help)
	return err
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, errParse) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	args, root, err := resolveRepoRoot(args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("%w\n%s", errParse, usage)
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		return printHelp("")
	}

	switch {
	case args[0] == "run":
		if wantsHelp(args[1:]) {
			return printHelp("run")
		}
		runOpts, err := parseRunOptions(args[1:])
		if err != nil {
			return err
		}
		// --backend takes precedence over $BACKEND when both are set; we
		// achieve this by overlaying the flag value on the lookup chain.
		lookup := overlayEnv(os.LookupEnv, "BACKEND", runOpts.backend)
		cfg, err := builderloop.ConfigFromEnv(root, lookup)
		if err != nil {
			return err
		}
		return runAutoloop(ctx, cfg, runOpts.dryRun)
	case args[0] == "progress":
		if wantsHelp(args[1:]) {
			key := "progress"
			if len(args) >= 2 && !strings.HasPrefix(args[1], "-") {
				key = "progress " + args[1]
			}
			return printHelp(key)
		}
		return runProgress(root, args[1:])
	case args[0] == "repo":
		if wantsHelp(args[1:]) {
			key := "repo"
			if len(args) >= 2 && !strings.HasPrefix(args[1], "-") {
				key = "repo " + args[1]
			}
			return printHelp(key)
		}
		return runRepo(root, args[1:])
	case args[0] == "digest":
		if wantsHelp(args[1:]) {
			return printHelp("digest")
		}
		opts, err := parseDigestOptions(args[1:])
		if err != nil {
			return err
		}
		digest, err := builderloop.DigestLedger(filepath.Join(digestRunRoot(root), "state", "runs.jsonl"))
		if err != nil {
			return err
		}
		if opts.outputPath != "" {
			return writeDigestOutput(opts.outputPath, digest, opts.force)
		}
		_, err = fmt.Fprint(commandStdout, digest)
		return err
	case args[0] == "audit":
		if wantsHelp(args[1:]) {
			return printHelp("audit")
		}
		if len(args) != 1 {
			return fmt.Errorf("%w\n%s", errParse, subUsage["audit"])
		}
		auditDir, err := auditReportDir()
		if err != nil {
			return err
		}
		summary, err := builderloop.WriteAuditReport(builderloop.AuditReportOptions{
			LedgerPath: filepath.Join(digestRunRoot(root), "state", "runs.jsonl"),
			AuditDir:   auditDir,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(commandStdout, summary)
		return err
	case args[0] == "service":
		return runService(ctx, root, args[1:])
	default:
		return fmt.Errorf("%w\n%s", errParse, usage)
	}
}

// runService dispatches the `service` subcommand family with --help routing.
// Splitting it out keeps run() readable now that each verb has its own help.
func runService(ctx context.Context, root string, args []string) error {
	if wantsHelp(args) {
		key := "service"
		switch {
		case len(args) >= 2 && args[0] == "disable":
			key = "service disable"
			if len(args) >= 2 && args[1] == "legacy-timers" {
				key = "service disable legacy-timers"
			}
		case len(args) >= 1 && args[0] == "install":
			key = "service install"
		case len(args) >= 1 && args[0] == "install-audit":
			key = "service install-audit"
		}
		return printHelp(key)
	}

	switch {
	case len(args) >= 1 && args[0] == "install":
		force, err := serviceForce(args[1:])
		if err != nil {
			return err
		}
		return installService(ctx, root, force)
	case len(args) >= 1 && args[0] == "install-audit":
		force, err := serviceForce(args[1:])
		if err != nil {
			return err
		}
		return installAuditService(ctx, root, force)
	case len(args) == 2 && args[0] == "disable" && args[1] == "legacy-timers":
		return builderloop.DisableLegacyTimers(ctx, serviceRunner)
	default:
		return fmt.Errorf("%w\n%s", errParse, subUsage["service"])
	}
}

type runOptions struct {
	dryRun  bool
	backend string
}

func parseRunOptions(args []string) (runOptions, error) {
	opts := runOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--dry-run":
			opts.dryRun = true
		case "--backend":
			if i+1 >= len(args) {
				return runOptions{}, fmt.Errorf("%w: --backend requires a value\n%s", errParse, subUsage["run"])
			}
			i++
			if !contains(supportedBuilderBackends, args[i]) {
				return runOptions{}, fmt.Errorf("%w: unsupported backend %q (want one of %s)\n%s",
					errParse, args[i], strings.Join(supportedBuilderBackends, ", "), subUsage["run"])
			}
			opts.backend = args[i]
		default:
			return runOptions{}, fmt.Errorf("%w\n%s", errParse, subUsage["run"])
		}
	}

	return opts, nil
}

// resolveRepoRoot consumes a --repo-root flag (if present) from anywhere in
// args and returns the cleaned arg list plus the resolved root. Falls back
// to REPO_ROOT then os.Getwd. Surfaces any os.Getwd error.
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

type digestOptions struct {
	outputPath string
	force      bool
}

func parseDigestOptions(args []string) (digestOptions, error) {
	opts := digestOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output":
			if i+1 >= len(args) || args[i+1] == "" {
				return digestOptions{}, fmt.Errorf("%w: --output requires a non-empty value\n%s", errParse, usage)
			}
			opts.outputPath = args[i+1]
			i++
		case "--force":
			opts.force = true
		default:
			return digestOptions{}, fmt.Errorf("%w\n%s", errParse, usage)
		}
	}
	return opts, nil
}

// writeDigestOutput writes digest to path, refusing to clobber an existing
// file unless force is true. The default no-clobber stance protects against
// fat-fingered paths (e.g. `--output README.md`) that the previous
// os.WriteFile() call would have silently overwritten.
func writeDigestOutput(path, digest string, force bool) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%s already exists; pass --force to overwrite", path)
		}
		return err
	}
	defer f.Close()
	_, err = io.WriteString(f, digest)
	return err
}

func contains(haystack []string, needle string) bool {
	for _, candidate := range haystack {
		if candidate == needle {
			return true
		}
	}
	return false
}

func serviceForce(args []string) (bool, error) {
	force := os.Getenv("FORCE") == "1"
	for _, arg := range args {
		if arg != "--force" {
			return false, fmt.Errorf("%w\n%s", errParse, usage)
		}
		force = true
	}

	return force, nil
}

func installService(ctx context.Context, root string, force bool) error {
	unitDir, err := systemdUserUnitDir()
	if err != nil {
		return err
	}

	return builderloop.InstallService(ctx, builderloop.ServiceInstallOptions{
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

func installAuditService(ctx context.Context, root string, force bool) error {
	unitDir, err := systemdUserUnitDir()
	if err != nil {
		return err
	}

	return builderloop.InstallAuditService(ctx, builderloop.AuditServiceInstallOptions{
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

func runAutoloop(ctx context.Context, cfg builderloop.Config, dryRun bool) error {
	summary, err := builderloop.RunOnce(ctx, builderloop.RunOptions{
		Config: cfg,
		DryRun: dryRun,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(commandStdout, "candidates: %d\nselected: %d\n", summary.Candidates, len(summary.Selected))
	for _, candidate := range summary.Selected {
		fmt.Fprintf(commandStdout, "- %s/%s %s [%s] owner=%s size=%s reason=%s\n",
			candidate.PhaseID,
			candidate.SubphaseID,
			candidate.ItemName,
			candidate.Status,
			dashIfEmpty(candidate.ExecutionOwner),
			dashIfEmpty(candidate.SliceSize),
			dashIfEmpty(candidate.SelectionReason()),
		)
	}

	return nil
}

func dashIfEmpty(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}

	return value
}

// overlayEnv returns an EnvLookup that returns override for key (when
// override is non-empty) and otherwise delegates to base. Used so a CLI
// flag wins over the matching environment variable without forcing
// callers to rebuild a map of every supported key.
func overlayEnv(base builderloop.EnvLookup, key, override string) builderloop.EnvLookup {
	if override == "" {
		return base
	}
	return func(k string) (string, bool) {
		if k == key {
			return override, true
		}
		return base(k)
	}
}
