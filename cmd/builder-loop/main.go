package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
	"github.com/TrebuchetDynamics/gormes-agent/internal/cmdrunner"
	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// cliDeps carries the writers and process runner that callers can swap
// in tests. Production main() builds defaultDeps() from os.Stdout/Stderr
// and the real cmdrunner.ExecRunner. Tests construct their own deps and
// pass them to run() — no mutable package-level globals to coordinate
// across t.Parallel cases.
type cliDeps struct {
	stdout io.Writer
	stderr io.Writer
	runner cmdrunner.Runner
}

func defaultDeps() cliDeps {
	return cliDeps{stdout: os.Stdout, stderr: os.Stderr, runner: cmdrunner.ExecRunner{}}
}

const usage = "usage: builder-loop [--repo-root <path>] run [--loop] [--dry-run] [--backend codexu|claudeu|opencode] | progress validate | progress write | repo benchmark record | repo readme update | audit | digest [--output <path>] [--force] | doctor | service install | service install-audit | service disable legacy-timers"

// subUsage maps each subcommand to its own help text. --help/-h on a
// subcommand prints the matching entry instead of the giant top-level usage,
// so an operator typing `builder-loop digest --help` sees just the digest
// flags rather than the full builder-loop surface.
var subUsage = map[string]string{
	"run":                           "usage: builder-loop run [--loop] [--dry-run] [--backend codexu|claudeu|opencode]",
	"progress":                      "usage: builder-loop progress {validate|write}",
	"progress write":                "usage: builder-loop progress write",
	"repo":                          "usage: builder-loop repo {benchmark record|readme update}",
	"repo benchmark":                "usage: builder-loop repo benchmark record",
	"repo readme":                   "usage: builder-loop repo readme update",
	"audit":                         "usage: builder-loop audit [--format text|json]",
	"digest":                        "usage: builder-loop digest [--output <path>] [--force] [--format text|json]",
	"doctor":                        "usage: builder-loop doctor [--format text|json]",
	"progress validate":             "usage: builder-loop progress validate [--format text|json]",
	"service":                       "usage: builder-loop service {install|install-audit|disable legacy-timers} [--force]",
	"service install":               "usage: builder-loop service install [--force]",
	"service install-audit":         "usage: builder-loop service install-audit [--force]",
	"service disable":               "usage: builder-loop service disable legacy-timers",
	"service disable legacy-timers": "usage: builder-loop service disable legacy-timers",
}

// supportedBuilderBackends lists the backends the run subcommand accepts via
// --backend. The same names are accepted via the BACKEND environment
// variable downstream.
var supportedBuilderBackends = []string{"codexu", "claudeu", "opencode"}

// errParse marks parser-level failures so main() can map them to exit code 2.
var errParse = errors.New("parse error")

// Exit codes used by the binary. Operators (and systemd Restart= /
// OnFailure= rules) can branch on these without parsing stderr.
//
//	2 — config / parse error: don't retry, fix the invocation
//	20 — backend timeout (context deadline / cancel): retry with backoff
//	30 — post-promotion verify gate failed: page operator
//	1 — anything else (internal error)
const (
	exitInternal       = 1
	exitParseError     = 2
	exitBackendTimeout = 20
	exitVerifyFailed   = 30
)

// classifyExit picks an exit code from err. The order matters: parse errors
// take precedence over context-cancel because a malformed flag should still
// exit 2 even if the surrounding context happens to have been canceled.
func classifyExit(err error) int {
	switch {
	case errors.Is(err, errParse):
		return exitParseError
	case errors.Is(err, builderloop.ErrPostPromotionVerifyFailed):
		return exitVerifyFailed
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return exitBackendTimeout
	default:
		return exitInternal
	}
}

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
// missing from subUsage) to deps.stdout. Help is intentionally exit-0
// output, so callers return nil after invoking this.
func printHelp(deps cliDeps, key string) error {
	help, ok := subUsage[key]
	if !ok {
		help = usage
	}
	_, err := fmt.Fprintln(deps.stdout, help)
	return err
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	deps := defaultDeps()
	if err := run(ctx, deps, os.Args[1:]); err != nil {
		fmt.Fprintln(deps.stderr, err)
		os.Exit(classifyExit(err))
	}
}

func run(ctx context.Context, deps cliDeps, args []string) error {
	args, root, err := resolveRepoRoot(args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("%w\n%s", errParse, usage)
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		return printHelp(deps, "")
	}

	switch {
	case args[0] == "run":
		if wantsHelp(args[1:]) {
			return printHelp(deps, "run")
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
		return runAutoloop(ctx, deps, root, cfg, runOpts)
	case args[0] == "progress":
		if wantsHelp(args[1:]) {
			key := "progress"
			if len(args) >= 2 && !strings.HasPrefix(args[1], "-") {
				key = "progress " + args[1]
			}
			return printHelp(deps, key)
		}
		return runProgress(deps, root, args[1:])
	case args[0] == "repo":
		if wantsHelp(args[1:]) {
			key := "repo"
			if len(args) >= 2 && !strings.HasPrefix(args[1], "-") {
				key = "repo " + args[1]
			}
			return printHelp(deps, key)
		}
		return runRepo(root, args[1:])
	case args[0] == "digest":
		if wantsHelp(args[1:]) {
			return printHelp(deps, "digest")
		}
		opts, err := parseDigestOptions(args[1:])
		if err != nil {
			return err
		}
		ledgerPath := filepath.Join(digestRunRoot(root), "state", "runs.jsonl")
		if opts.format == "json" {
			counts, err := builderloop.DigestLedgerCounts(ledgerPath)
			if err != nil {
				return err
			}
			if opts.outputPath != "" {
				body, _ := json.Marshal(counts)
				return writeDigestOutput(opts.outputPath, string(body)+"\n", opts.force)
			}
			return json.NewEncoder(deps.stdout).Encode(counts)
		}
		digest, err := builderloop.DigestLedger(ledgerPath)
		if err != nil {
			return err
		}
		if opts.outputPath != "" {
			return writeDigestOutput(opts.outputPath, digest, opts.force)
		}
		_, err = fmt.Fprint(deps.stdout, digest)
		return err
	case args[0] == "audit":
		if wantsHelp(args[1:]) {
			return printHelp(deps, "audit")
		}
		format, err := parseFormat(args[1:], "audit")
		if err != nil {
			return err
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
		if format == "json" {
			// summary is a multi-line text block; the audit ledger itself
			// (audit.AuditDir/report.ndjson) carries the structured line.
			// Wrap the text so callers have a stable JSON object to parse
			// while still reading the same content.
			return json.NewEncoder(deps.stdout).Encode(struct {
				Summary string `json:"summary"`
			}{Summary: summary})
		}
		_, err = fmt.Fprint(deps.stdout, summary)
		return err
	case args[0] == "doctor":
		if wantsHelp(args[1:]) {
			return printHelp(deps, "doctor")
		}
		format, err := parseFormat(args[1:], "doctor")
		if err != nil {
			return err
		}
		cfg, err := builderloop.ConfigFromEnv(root, os.LookupEnv)
		if err != nil {
			return err
		}
		return doctor(deps, cfg, format)
	case args[0] == "service":
		return runService(ctx, deps, root, args[1:])
	default:
		return fmt.Errorf("%w\n%s", errParse, usage)
	}
}

// runService dispatches the `service` subcommand family with --help routing.
func runService(ctx context.Context, deps cliDeps, root string, args []string) error {
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
		return printHelp(deps, key)
	}

	switch {
	case len(args) >= 1 && args[0] == "install":
		force, err := serviceForce(args[1:])
		if err != nil {
			return err
		}
		return installService(ctx, deps, root, force)
	case len(args) >= 1 && args[0] == "install-audit":
		force, err := serviceForce(args[1:])
		if err != nil {
			return err
		}
		return installAuditService(ctx, deps, root, force)
	case len(args) == 2 && args[0] == "disable" && args[1] == "legacy-timers":
		return builderloop.DisableLegacyTimers(ctx, deps.runner)
	default:
		return fmt.Errorf("%w\n%s", errParse, subUsage["service"])
	}
}

type runOptions struct {
	dryRun  bool
	loop    bool
	backend string
}

func parseRunOptions(args []string) (runOptions, error) {
	opts := runOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--dry-run":
			opts.dryRun = true
		case "--loop":
			opts.loop = true
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
	if opts.loop && opts.dryRun {
		return runOptions{}, fmt.Errorf("%w: --loop cannot be combined with --dry-run\n%s", errParse, subUsage["run"])
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
	format     string
}

func parseDigestOptions(args []string) (digestOptions, error) {
	opts := digestOptions{format: "text"}
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
		case "--format":
			if i+1 >= len(args) {
				return digestOptions{}, fmt.Errorf("%w: --format requires a value\n%s", errParse, subUsage["digest"])
			}
			switch args[i+1] {
			case "text", "json":
				opts.format = args[i+1]
			default:
				return digestOptions{}, fmt.Errorf("%w: --format must be text or json (got %q)\n%s",
					errParse, args[i+1], subUsage["digest"])
			}
			i++
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

func installService(ctx context.Context, deps cliDeps, root string, force bool) error {
	unitDir, err := systemdUserUnitDir()
	if err != nil {
		return err
	}

	return builderloop.InstallService(ctx, builderloop.ServiceInstallOptions{
		Runner:       deps.runner,
		UnitDir:      unitDir,
		UnitName:     "gormes-orchestrator.service",
		AutoloopPath: orchestratorWrapperPath(root),
		WorkDir:      root,
		ExecArgs:     []string{"run", "--loop"},
		AutoStart:    autoStart(),
		Force:        force,
	})
}

func installAuditService(ctx context.Context, deps cliDeps, root string, force bool) error {
	unitDir, err := systemdUserUnitDir()
	if err != nil {
		return err
	}

	return builderloop.InstallAuditService(ctx, builderloop.AuditServiceInstallOptions{
		Runner:    deps.runner,
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

type autoloopRuntime struct {
	runBuilder func(context.Context, builderloop.Config, bool) (builderloop.RunSummary, error)
	runPlanner func(context.Context) error
	sleep      func(context.Context, time.Duration) error
}

func defaultAutoloopRuntime(deps cliDeps, root string) autoloopRuntime {
	runner := deps.runner
	if runner == nil {
		runner = cmdrunner.ExecRunner{}
	}
	return autoloopRuntime{
		runBuilder: func(ctx context.Context, cfg builderloop.Config, dryRun bool) (builderloop.RunSummary, error) {
			return builderloop.RunOnce(ctx, builderloop.RunOptions{
				Config: cfg,
				Runner: runner,
				DryRun: dryRun,
			})
		},
		runPlanner: func(ctx context.Context) error {
			result := runner.Run(ctx, cmdrunner.Command{
				Name: "go",
				Args: []string{"run", "./cmd/planner-loop", "run"},
				Dir:  root,
			})
			if result.Err != nil {
				detail := strings.TrimSpace(result.Stderr)
				if detail != "" {
					return fmt.Errorf("planner command go run ./cmd/planner-loop run failed: %w: %s", result.Err, detail)
				}
				return fmt.Errorf("planner command go run ./cmd/planner-loop run failed: %w", result.Err)
			}
			return nil
		},
		sleep: sleepContext,
	}
}

func runAutoloop(ctx context.Context, deps cliDeps, root string, cfg builderloop.Config, opts runOptions) error {
	interval := time.Duration(0)
	if opts.loop {
		var err error
		interval, err = builderLoopSleep(os.LookupEnv)
		if err != nil {
			return err
		}
	}
	return runAutoloopWithRuntime(ctx, deps, cfg, opts, interval, defaultAutoloopRuntime(deps, root))
}

func runAutoloopWithRuntime(ctx context.Context, deps cliDeps, cfg builderloop.Config, opts runOptions, interval time.Duration, runtime autoloopRuntime) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		summary, err := runtime.runBuilder(ctx, cfg, opts.dryRun)
		if err != nil {
			return err
		}
		printAutoloopSummary(deps.stdout, summary)
		if !opts.loop {
			return nil
		}
		if err := runtime.runPlanner(ctx); err != nil {
			return err
		}
		if err := runtime.sleep(ctx, interval); err != nil {
			return err
		}
	}
}

func printAutoloopSummary(w io.Writer, summary builderloop.RunSummary) {
	fmt.Fprintf(w, "candidates: %d\nselected: %d\n", summary.Candidates, len(summary.Selected))
	if summary.MaxPhaseFiltered > 0 {
		fmt.Fprintf(w, "max_phase_filtered: %d\nnext_max_phase: %d\nhint: rerun with MAX_PHASE=%d to include the next queued phase\n",
			summary.MaxPhaseFiltered,
			summary.NextFilteredMaxPhase,
			summary.NextFilteredMaxPhase,
		)
	}
	for _, candidate := range summary.Selected {
		fmt.Fprintf(w, "- %s/%s %s [%s] owner=%s size=%s reason=%s\n",
			candidate.PhaseID,
			candidate.SubphaseID,
			candidate.ItemName,
			candidate.Status,
			dashIfEmpty(candidate.ExecutionOwner),
			dashIfEmpty(candidate.SliceSize),
			dashIfEmpty(candidate.SelectionReason()),
		)
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func builderLoopSleep(lookup func(string) (string, bool)) (time.Duration, error) {
	value, ok := lookup("BUILDER_LOOP_SLEEP")
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return 30 * time.Second, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%w: BUILDER_LOOP_SLEEP must be a non-negative Go duration (got %q)", errParse, value)
	}
	return d, nil
}

func dashIfEmpty(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}

	return value
}

// doctor runs operator-facing health checks for the builder loop.
//   - progress.json exists, parses, and validates
//   - the planner triggers path is writable (so the builder can append to it)
//   - the latest health_updated event in the builder ledger is fresher than
//     1 hour; warning-only since a fresh checkout has no history
//
// Each warning is advisory; the command exits 0 unless a hard precondition
// fails. systemd timer status is intentionally not probed: the binary itself
// running implies the unit fired.
func doctor(deps cliDeps, cfg builderloop.Config, format string) error {
	if _, err := os.Stat(cfg.ProgressJSON); err != nil {
		return err
	}
	if p, err := progress.Load(cfg.ProgressJSON); err != nil {
		return fmt.Errorf("progress.json: %w", err)
	} else if err := progress.Validate(p); err != nil {
		return fmt.Errorf("progress.json validation: %w", err)
	}
	if err := triggerPathWritable(cfg.PlannerTriggersPath); err != nil {
		return fmt.Errorf("planner triggers path: %w", err)
	}
	var warnings []string
	ledgerPath := filepath.Join(cfg.RunRoot, "state", "runs.jsonl")
	if msg := driftWarning("builder-loop", ledgerPath, "health_updated", time.Hour); msg != "" {
		warnings = append(warnings, msg)
	}
	staleClaims, err := staleWorkerClaimWarnings(ledgerPath, time.Now(), time.Hour)
	if err != nil {
		if !os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("doctor: warning: builder-loop ledger unreadable at %s: %v", ledgerPath, err))
		}
	} else {
		warnings = append(warnings, staleClaims...)
	}
	if format == "json" {
		return json.NewEncoder(deps.stdout).Encode(struct {
			OK       bool     `json:"ok"`
			Warnings []string `json:"warnings"`
		}{OK: true, Warnings: warnings})
	}
	for _, w := range warnings {
		fmt.Fprintln(deps.stdout, w)
	}
	_, err = fmt.Fprintln(deps.stdout, "doctor: ok")
	return err
}

// triggerPathWritable verifies the parent of path exists (creating it if
// missing) and that we can append to a JSONL file there. The file itself is
// created empty on first write so doctor on a fresh checkout passes.
func triggerPathWritable(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// driftWarning mirrors planner-loop's drift check: returns a non-empty
// advisory string when the latest event of `eventName` in the ledger at
// path is older than threshold (or absent entirely). A missing ledger file
// is treated as "no history", not a stall.
func driftWarning(loop, path, eventName string, threshold time.Duration) string {
	latest, err := latestLedgerEventTime(path, eventName)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return fmt.Sprintf("doctor: warning: %s ledger unreadable at %s: %v", loop, path, err)
	}
	if latest.IsZero() {
		return fmt.Sprintf("doctor: warning: %s ledger %s has no %s events yet", loop, path, eventName)
	}
	age := time.Since(latest)
	if age > threshold {
		return fmt.Sprintf("doctor: warning: %s last %s was %s ago (>%s); loop may be stalled", loop, eventName, age.Truncate(time.Second), threshold)
	}
	return ""
}

type openWorkerClaim struct {
	event builderloop.LedgerEvent
}

func staleWorkerClaimWarnings(path string, now time.Time, threshold time.Duration) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	open := make(map[string]openWorkerClaim)
	var order []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev builderloop.LedgerEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		key := workerClaimKey(ev)
		if key == "" {
			continue
		}
		switch ev.Event {
		case "worker_claimed":
			if _, ok := open[key]; !ok {
				order = append(order, key)
			}
			open[key] = openWorkerClaim{event: ev}
		case "worker_success", "worker_failed", "worker_promotion_failed", "candidate_skipped":
			delete(open, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var warnings []string
	for _, key := range order {
		claim, ok := open[key]
		if !ok {
			continue
		}
		age := now.Sub(claim.event.TS)
		if age <= threshold {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("doctor: warning: builder-loop worker claim stale for %s: run=%s worker=%d task=%s branch=%s",
			age.Truncate(time.Second),
			claim.event.RunID,
			claim.event.Worker,
			claim.event.Task,
			claim.event.Branch,
		))
	}
	return warnings, nil
}

func workerClaimKey(ev builderloop.LedgerEvent) string {
	if ev.RunID == "" || ev.Worker == 0 {
		return ""
	}
	return fmt.Sprintf("%s\x00%d\x00%s\x00%s", ev.RunID, ev.Worker, ev.Task, ev.Branch)
}

// latestLedgerEventTime scans the JSONL ledger at path and returns the
// timestamp of the most recent event with the given Event field. Returns a
// zero time and nil error if no matching event exists; bubbles os.IsNotExist
// if the file is missing entirely.
func latestLedgerEventTime(path, eventName string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

	var latest time.Time
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev struct {
			TS    time.Time `json:"ts"`
			Event string    `json:"event"`
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Event == eventName && ev.TS.After(latest) {
			latest = ev.TS
		}
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, err
	}
	return latest, nil
}

// parseFormat consumes a "--format text|json" pair from args (in any
// position) and returns the chosen format. Defaults to "text" when the flag
// is absent. Used by read-only subcommands so external tooling can
// consume their output as JSON without parsing prose.
func parseFormat(args []string, subcommand string) (string, error) {
	format := "text"
	for i := 0; i < len(args); i++ {
		if args[i] == "--format" {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%w: --format requires a value\n%s", errParse, subUsage[subcommand])
			}
			switch args[i+1] {
			case "text", "json":
				format = args[i+1]
			default:
				return "", fmt.Errorf("%w: --format must be text or json (got %q)\n%s",
					errParse, args[i+1], subUsage[subcommand])
			}
			i++
			continue
		}
		return "", fmt.Errorf("%w: unexpected argument %q\n%s", errParse, args[i], subUsage[subcommand])
	}
	return format, nil
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
