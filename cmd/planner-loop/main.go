package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/cmdrunner"
	"github.com/TrebuchetDynamics/gormes-agent/internal/plannerloop"
	"github.com/TrebuchetDynamics/gormes-agent/internal/plannertriggers"
	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

var commandStdout io.Writer = os.Stdout
var commandRunner cmdrunner.Runner = cmdrunner.ExecRunner{}

const usage = "usage: planner-loop [--repo-root <path>] run [--dry-run] [--backend codexu|claudeu] [--mode safe|full|unattended] [keyword ...] | status | show-report | doctor | trigger <reason> | service install [--force]"

// subUsage maps each subcommand to its own help text. --help/-h on a
// subcommand prints the matching entry instead of the full top-level usage.
var subUsage = map[string]string{
	"run":             "usage: planner-loop run [--dry-run] [--backend codexu|claudeu] [--mode safe|full|unattended] [keyword ...]",
	"status":          "usage: planner-loop status",
	"show-report":     "usage: planner-loop show-report",
	"doctor":          "usage: planner-loop doctor",
	"trigger":         "usage: planner-loop trigger <reason>",
	"service":         "usage: planner-loop service install [--force]",
	"service install": "usage: planner-loop service install [--force]",
}

// supportedPlannerBackends lists the backends the run subcommand accepts via
// --backend. opencode is intentionally absent: planner runs need the richer
// reasoning surface that codexu/claudeu provide.
var supportedPlannerBackends = []string{"codexu", "claudeu"}

// errParse marks parser-level failures so main() can map them to exit code 2.
var errParse = errors.New("parse error")

// wantsHelp returns true if any arg is --help or -h.
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
		args = []string{"run"}
	}

	switch args[0] {
	case "run":
		if wantsHelp(args[1:]) {
			return printHelp("run")
		}
		opts, err := parseRunOptions(args[1:])
		if err != nil {
			return err
		}
		cfg, err := plannerloop.ConfigFromEnv(root, plannerLookup(opts))
		if err != nil {
			return err
		}
		summary, err := plannerloop.RunOnce(ctx, plannerloop.RunOptions{
			Config:   cfg,
			Runner:   commandRunner,
			DryRun:   opts.dryRun,
			Keywords: opts.keywords,
		})
		if err != nil {
			return err
		}
		return printRunSummary(summary, opts.dryRun, opts.keywords)
	case "status":
		if wantsHelp(args[1:]) {
			return printHelp("status")
		}
		cfg, err := plannerloop.ConfigFromEnv(root, plannerLookup(runOptions{}))
		if err != nil {
			return err
		}
		return printStatus(cfg)
	case "show-report":
		if wantsHelp(args[1:]) {
			return printHelp("show-report")
		}
		cfg, err := plannerloop.ConfigFromEnv(root, plannerLookup(runOptions{}))
		if err != nil {
			return err
		}
		return printFile(filepath.Join(cfg.RunRoot, "latest_planner_report.md"))
	case "doctor":
		if wantsHelp(args[1:]) {
			return printHelp("doctor")
		}
		cfg, err := plannerloop.ConfigFromEnv(root, plannerLookup(runOptions{}))
		if err != nil {
			return err
		}
		return doctor(cfg)
	case "trigger":
		if wantsHelp(args[1:]) {
			return printHelp("trigger")
		}
		cfg, err := plannerloop.ConfigFromEnv(root, plannerLookup(runOptions{}))
		if err != nil {
			return err
		}
		return runTrigger(cfg, args[1:])
	case "service":
		if wantsHelp(args[1:]) {
			key := "service"
			if len(args) >= 2 && args[1] == "install" {
				key = "service install"
			}
			return printHelp(key)
		}
		if len(args) >= 2 && args[1] == "install" {
			force, err := plannerServiceForce(args[2:])
			if err != nil {
				return err
			}
			cfg, err := plannerloop.ConfigFromEnv(root, plannerLookup(runOptions{}))
			if err != nil {
				return err
			}
			return installPlannerService(ctx, root, force, cfg.PlannerTriggersPath)
		}
		return fmt.Errorf("%w\n%s", errParse, subUsage["service"])
	case "--help", "-h", "help":
		return printHelp("")
	default:
		return fmt.Errorf("%w\n%s", errParse, usage)
	}
}

type runOptions struct {
	dryRun   bool
	backend  string
	mode     string
	keywords []string
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
				return runOptions{}, fmt.Errorf("%w: --backend requires a value\n%s", errParse, usage)
			}
			i++
			if !plannerContains(supportedPlannerBackends, args[i]) {
				return runOptions{}, fmt.Errorf("%w: unsupported backend %q (want one of %s)\n%s",
					errParse, args[i], strings.Join(supportedPlannerBackends, ", "), usage)
			}
			opts.backend = args[i]
		case "--mode":
			if i+1 >= len(args) {
				return runOptions{}, fmt.Errorf("%w: --mode requires a value\n%s", errParse, usage)
			}
			i++
			opts.mode = args[i]
		default:
			// Treat as positional keyword argument (L6 topical focus mode).
			// Multi-word keywords (e.g. "skills tools") get split on
			// whitespace so a single quoted shell arg yields multiple
			// keywords. Reject obvious typos for unknown flags.
			if strings.HasPrefix(arg, "-") {
				return runOptions{}, fmt.Errorf("%w\n%s", errParse, usage)
			}
			for _, kw := range strings.Fields(arg) {
				opts.keywords = append(opts.keywords, kw)
			}
		}
	}
	return opts, nil
}

func plannerContains(haystack []string, needle string) bool {
	for _, candidate := range haystack {
		if candidate == needle {
			return true
		}
	}
	return false
}

// plannerLookup returns an EnvLookup that delegates to os.LookupEnv but
// overlays --backend / --mode CLI flags so they win over the matching
// env vars without forcing callers to enumerate every supported key.
func plannerLookup(opts runOptions) plannerloop.EnvLookup {
	overrides := map[string]string{}
	if opts.backend != "" {
		overrides["BACKEND"] = opts.backend
	}
	if opts.mode != "" {
		overrides["MODE"] = opts.mode
	}
	if len(overrides) == 0 {
		return os.LookupEnv
	}
	return func(key string) (string, bool) {
		if v, ok := overrides[key]; ok {
			return v, true
		}
		return os.LookupEnv(key)
	}
}

func printRunSummary(summary plannerloop.RunSummary, dryRun bool, keywords []string) error {
	label := "architecture planner run"
	if dryRun {
		label = "architecture planner dry-run"
	}
	fmt.Fprintf(commandStdout, "%s\n", label)
	fmt.Fprintf(commandStdout, "backend: %s\n", summary.Backend)
	fmt.Fprintf(commandStdout, "mode: %s\n", summary.Mode)
	if len(keywords) > 0 {
		// Echoing keywords back to the operator confirms topical-focus
		// mode actually engaged when the planner was invoked with
		// positional args (e.g. `planner-loop run hermes-issues`).
		fmt.Fprintf(commandStdout, "keywords: %s\n", strings.Join(keywords, " "))
	}
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

func printStatus(cfg plannerloop.Config) error {
	out, err := plannerloop.RenderStatus(plannerloop.RenderStatusOptions{
		StatePath:          filepath.Join(cfg.RunRoot, "planner_state.json"),
		PlannerLedgerPath:  filepath.Join(cfg.RunRoot, "state", "runs.jsonl"),
		AutoloopLedgerPath: filepath.Join(cfg.AutoloopRunRoot, "state", "runs.jsonl"),
		ProgressJSONPath:   cfg.ProgressJSON,
		EvaluationWindow:   cfg.EvaluationWindow,
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(commandStdout, out)
	return err
}

func printFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = commandStdout.Write(data)
	return err
}

func plannerServiceForce(args []string) (bool, error) {
	force := os.Getenv("FORCE") == "1"
	for _, arg := range args {
		if arg != "--force" {
			return false, fmt.Errorf("%w\n%s", errParse, usage)
		}
		force = true
	}
	return force, nil
}

// resolveRepoRoot consumes a --repo-root flag (if present) from anywhere in
// args and returns the cleaned arg list plus the resolved root. Falls back
// to REPO_ROOT then os.Getwd. Mirrors the builder-loop helper.
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

func installPlannerService(ctx context.Context, root string, force bool, pathToWatch string) error {
	unitDir, err := plannerUnitDir()
	if err != nil {
		return err
	}
	interval := os.Getenv("PLANNER_INTERVAL")
	autoStart := os.Getenv("AUTO_START") != "0"

	// Phase D Task 4: install the impl-tree .path unit by default, watching
	// cmd/ and internal/. PLANNER_IMPL_PATHS overrides as a CSV of absolute
	// directories; setting it to a single empty value disables the impl
	// .path unit so existing 3-unit installs can opt out.
	implPaths := defaultImplPathsToWatch(root)
	if value := os.Getenv("PLANNER_IMPL_PATHS"); value != "" {
		implPaths = splitAndTrim(value)
	}
	implPathName := "gormes-planner-loop-impl.path"
	if len(implPaths) == 0 {
		implPathName = ""
	}

	return plannerloop.InstallPlannerService(ctx, plannerloop.PlannerServiceInstallOptions{
		Runner:           commandRunner,
		UnitDir:          unitDir,
		UnitName:         "gormes-planner-loop.service",
		TimerName:        "gormes-planner-loop.timer",
		PathName:         "gormes-planner-loop.path",
		PathToWatch:      pathToWatch,
		ImplPathName:     implPathName,
		ImplPathsToWatch: implPaths,
		PlannerPath:      plannerWrapperPath(root),
		WorkDir:          root,
		Interval:         interval,
		AutoStart:        autoStart,
		Force:            force,
	})
}

// defaultImplPathsToWatch returns the impl-tree directories the impl .path
// unit watches by default: <root>/cmd and <root>/internal. Override via
// PLANNER_IMPL_PATHS (CSV of absolute paths).
func defaultImplPathsToWatch(root string) []string {
	return []string{
		filepath.Join(root, "cmd"),
		filepath.Join(root, "internal"),
	}
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func plannerUnitDir() (string, error) {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return filepath.Join(value, "systemd", "user"), nil
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", "systemd", "user"), nil
	}
	return "", fmt.Errorf("cannot determine systemd user unit directory: set XDG_CONFIG_HOME or HOME")
}

func plannerWrapperPath(root string) string {
	if path := os.Getenv("PLANNER_PATH"); path != "" {
		return path
	}
	return filepath.Join(root, "scripts", "planner-loop.sh")
}

func doctor(cfg plannerloop.Config) error {
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
	// progress.json must actually parse + validate, not just exist —
	// silent corruption was a pre-existing operator footgun.
	if p, err := progress.Load(cfg.ProgressJSON); err != nil {
		return fmt.Errorf("progress.json: %w", err)
	} else if err := progress.Validate(p); err != nil {
		return fmt.Errorf("progress.json validation: %w", err)
	}
	if _, err := exec.LookPath(cfg.Backend); err != nil {
		return fmt.Errorf("backend %q not found on PATH: %w", cfg.Backend, err)
	}
	if err := triggerPathWritable(cfg.PlannerTriggersPath); err != nil {
		return fmt.Errorf("planner triggers path: %w", err)
	}
	// Drift check: warn (do not fail) if the last planner health_updated
	// event is older than 2× the configured interval. Doctor's job here is
	// to surface "the loop runs but is not progressing" — historically the
	// most-missed failure mode.
	plannerLedger := filepath.Join(cfg.RunRoot, "state", "runs.jsonl")
	threshold := plannerDriftThreshold()
	if msg := driftWarning("planner", plannerLedger, "health_updated", threshold); msg != "" {
		fmt.Fprintln(commandStdout, msg)
	}
	builderLedger := filepath.Join(cfg.AutoloopRunRoot, "state", "runs.jsonl")
	if msg := driftWarning("builder-loop", builderLedger, "health_updated", time.Hour); msg != "" {
		fmt.Fprintln(commandStdout, msg)
	}
	_, err := fmt.Fprintln(commandStdout, "doctor: ok")
	return err
}

// plannerDriftThreshold returns 2× the planner timer interval, defaulting to
// 12h when PLANNER_INTERVAL is unset/unparseable. Used by doctor to decide
// whether the planner has fallen silent.
func plannerDriftThreshold() time.Duration {
	const defaultThreshold = 12 * time.Hour
	value := os.Getenv("PLANNER_INTERVAL")
	if value == "" {
		return defaultThreshold
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return defaultThreshold
	}
	return 2 * d
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

// driftWarning returns a non-empty warning string if the latest event of
// kind `eventName` in the ledger at path is older than threshold (or absent
// entirely). Returns "" when the ledger is fresh, missing entirely (a fresh
// checkout has no history to be stale against), or when reads fail
// transiently. Doctor surfaces these as advisory output, not hard failures.
func driftWarning(loop, path, eventName string, threshold time.Duration) string {
	latest, err := latestLedgerEventTime(path, eventName)
	if err != nil {
		// File missing on a fresh checkout is not a drift signal.
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

// runTrigger appends a manual trigger event to the planner's triggers.jsonl
// ledger. The .path systemd unit watches that file and fires a planner run
// shortly after. Reason is surfaced in the planner prompt as the trigger
// label so the next run can react to "operator-asked" vs scheduled vs
// impl_change inputs.
func runTrigger(cfg plannerloop.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: trigger requires a <reason>\n%s", errParse, subUsage["trigger"])
	}
	reason := strings.TrimSpace(strings.Join(args, " "))
	if reason == "" {
		return fmt.Errorf("%w: trigger reason cannot be empty\n%s", errParse, subUsage["trigger"])
	}

	if err := plannertriggers.AppendTriggerEvent(cfg.PlannerTriggersPath, plannertriggers.TriggerEvent{
		Source: "manual",
		Kind:   "manual",
		Reason: reason,
	}); err != nil {
		return fmt.Errorf("append trigger: %w", err)
	}

	_, err := fmt.Fprintf(commandStdout, "trigger: appended manual event reason=%q to %s\n", reason, cfg.PlannerTriggersPath)
	return err
}
