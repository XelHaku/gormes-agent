package architectureplanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

type RunOptions struct {
	Config         Config
	Runner         autoloop.Runner
	DryRun         bool
	SkipValidation bool
	Now            time.Time
}

type RunSummary struct {
	Backend       string
	Mode          string
	RunRoot       string
	ProgressJSON  string
	ProgressItems int
	SourceRoots   []SourceRoot
	ReportPath    string
	StatePath     string
	ContextPath   string
	PromptPath    string
}

type stateFile struct {
	LastRunUTC   string           `json:"last_run_utc"`
	Backend      string           `json:"backend"`
	Mode         string           `json:"mode"`
	RepoRoot     string           `json:"repo_root"`
	ProgressJSON string           `json:"progress_json"`
	ContextPath  string           `json:"context_path"`
	PromptPath   string           `json:"prompt_path"`
	ReportPath   string           `json:"report_path"`
	SyncResults  []RepoSyncResult `json:"sync_results,omitempty"`
}

func RunOnce(ctx context.Context, opts RunOptions) (RunSummary, error) {
	cfg := opts.Config
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runner := opts.Runner
	if runner == nil {
		runner = autoloop.ExecRunner{}
	}

	if err := os.MkdirAll(cfg.RunRoot, 0o755); err != nil {
		return RunSummary{}, err
	}

	var syncResults []RepoSyncResult
	if cfg.SyncRepos && !opts.DryRun {
		var err error
		syncResults, err = SyncExternalRepos(ctx, cfg, runner)
		if err != nil {
			return RunSummary{}, err
		}
	}

	bundle, err := CollectContext(cfg, now)
	if err != nil {
		return RunSummary{}, err
	}
	bundle.SyncResults = syncResults

	contextPath := filepath.Join(cfg.RunRoot, "context.json")
	promptPath := filepath.Join(cfg.RunRoot, "latest_prompt.txt")
	reportPath := filepath.Join(cfg.RunRoot, "latest_planner_report.md")
	rawReportPath := filepath.Join(cfg.RunRoot, "latest_planner_report.raw.md")
	statePath := filepath.Join(cfg.RunRoot, "planner_state.json")
	validationLogPath := filepath.Join(cfg.RunRoot, "validation.log")

	if err := writeContext(contextPath, bundle); err != nil {
		return RunSummary{}, err
	}
	prompt := BuildPrompt(bundle)
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return RunSummary{}, err
	}

	summary := RunSummary{
		Backend:       cfg.Backend,
		Mode:          cfg.Mode,
		RunRoot:       cfg.RunRoot,
		ProgressJSON:  cfg.ProgressJSON,
		ProgressItems: bundle.ProgressStats.Items,
		SourceRoots:   bundle.SourceRoots,
		ReportPath:    reportPath,
		StatePath:     statePath,
		ContextPath:   contextPath,
		PromptPath:    promptPath,
	}

	if opts.DryRun {
		return summary, nil
	}

	argv, err := plannerBackendCommand(cfg.Backend, cfg.Mode, rawReportPath)
	if err != nil {
		return RunSummary{}, err
	}
	result := runner.Run(ctx, autoloop.Command{
		Name: argv[0],
		Args: append(append([]string(nil), argv[1:]...), prompt),
		Dir:  cfg.RepoRoot,
	})
	if result.Err != nil {
		return RunSummary{}, commandError(argv[0], result)
	}

	if err := writeReport(reportPath, rawReportPath, result, bundle, now); err != nil {
		return RunSummary{}, err
	}

	if cfg.Validate && !opts.SkipValidation {
		if err := runValidation(ctx, runner, cfg.RepoRoot, validationLogPath); err != nil {
			return RunSummary{}, err
		}
	}

	if err := writeState(statePath, stateFile{
		LastRunUTC:   now.UTC().Format(time.RFC3339),
		Backend:      cfg.Backend,
		Mode:         cfg.Mode,
		RepoRoot:     cfg.RepoRoot,
		ProgressJSON: cfg.ProgressJSON,
		ContextPath:  contextPath,
		PromptPath:   promptPath,
		ReportPath:   reportPath,
		SyncResults:  syncResults,
	}); err != nil {
		return RunSummary{}, err
	}

	return summary, nil
}

func plannerBackendCommand(backend, mode, rawReportPath string) ([]string, error) {
	if backend == "" {
		backend = "codexu"
	}
	switch backend {
	case "codexu", "claudeu":
	default:
		return nil, fmt.Errorf("invalid BACKEND %q: expected codexu or claudeu", backend)
	}

	argv, err := autoloop.BuildBackendCommand(backend, mode)
	if err != nil {
		return nil, err
	}
	return append(argv, "--output-last-message", rawReportPath), nil
}

func runValidation(ctx context.Context, runner autoloop.Runner, repoRoot, logPath string) error {
	commands := []autoloop.Command{
		{Name: "go", Args: []string{"run", "./cmd/progress-gen", "-write"}, Dir: repoRoot},
		{Name: "go", Args: []string{"run", "./cmd/progress-gen", "-validate"}, Dir: repoRoot},
		{Name: "go", Args: []string{"test", "./internal/progress", "-count=1"}, Dir: repoRoot},
		{Name: "go", Args: []string{"test", "./docs", "-count=1"}, Dir: repoRoot},
		{Name: "go", Args: []string{"test", "./...", "-count=1"}, Dir: filepath.Join(repoRoot, "www.gormes.ai")},
	}

	var log strings.Builder
	for _, command := range commands {
		log.WriteString("$ " + command.Name + " " + strings.Join(command.Args, " ") + "\n")
		result := runner.Run(ctx, command)
		log.WriteString(result.Stdout)
		log.WriteString(result.Stderr)
		if result.Err != nil {
			_ = os.WriteFile(logPath, []byte(log.String()), 0o644)
			return commandError(command.Name, result)
		}
	}
	return os.WriteFile(logPath, []byte(log.String()), 0o644)
}

func writeReport(path, rawPath string, result autoloop.Result, bundle ContextBundle, now time.Time) error {
	raw := strings.TrimSpace(result.Stdout)
	if data, err := os.ReadFile(rawPath); err == nil && strings.TrimSpace(string(data)) != "" {
		raw = strings.TrimSpace(string(data))
	}
	if raw == "" {
		raw = "Planner backend completed without a text report."
	}

	body := fmt.Sprintf(`# Architecture Planner Loop Run

- Run UTC: %s
- Repo root: %s
- Progress JSON: %s
- Progress items: %d

%s
`, now.UTC().Format(time.RFC3339), bundle.RepoRoot, bundle.ProgressJSON, bundle.ProgressStats.Items, raw)
	return os.WriteFile(path, []byte(body), 0o644)
}

func writeState(path string, state stateFile) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func commandError(name string, result autoloop.Result) error {
	output := strings.TrimSpace(result.Stderr)
	if output == "" {
		output = strings.TrimSpace(result.Stdout)
	}
	if output == "" {
		return fmt.Errorf("%s failed: %w", name, result.Err)
	}
	return fmt.Errorf("%s failed: %w: %s", name, result.Err, output)
}
