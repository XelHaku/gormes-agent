package plannerloop

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
	"github.com/TrebuchetDynamics/gormes-agent/internal/cmdrunner"
	"github.com/TrebuchetDynamics/gormes-agent/internal/plannertriggers"
	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

type RunOptions struct {
	Config         Config
	Runner         cmdrunner.Runner
	DryRun         bool
	SkipValidation bool
	Now            time.Time
	// Keywords narrows the planner's row-level context (QuarantinedRows,
	// future PreviousReshapes) to only rows that mechanically match any of
	// these substrings. Empty means broad/full-context run. See L6 topical
	// focus mode in docs/superpowers/specs/2026-04-24-planner-self-healing-design.md.
	Keywords []string
}

type RunSummary struct {
	RunID         string
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

var ErrPlannerRunInProgress = errors.New("planner run already in progress")

type plannerRunLock struct {
	file *os.File
}

func acquirePlannerRunLock(runRoot string, now time.Time) (*plannerRunLock, error) {
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(runRoot, "run.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: %s", ErrPlannerRunInProgress, lockPath)
		}
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_ = file.Truncate(0)
	_, _ = file.WriteString(fmt.Sprintf("pid=%d\nstarted_utc=%s\n", os.Getpid(), now.UTC().Format(time.RFC3339)))
	return &plannerRunLock{file: file}, nil
}

func (lock *plannerRunLock) release() {
	if lock == nil || lock.file == nil {
		return
	}
	_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	_ = lock.file.Close()
}

func RunOnce(ctx context.Context, opts RunOptions) (RunSummary, error) {
	cfg := opts.Config
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := now.UTC().Format("20060102T150405Z")
	runner := opts.Runner
	if runner == nil {
		runner = cmdrunner.ExecRunner{}
	}

	runLock, err := acquirePlannerRunLock(cfg.RunRoot, now)
	if err != nil {
		return RunSummary{}, err
	}
	defer runLock.release()

	if cfg.MergeOpenPullRequests && !opts.DryRun {
		if _, err := builderloop.MergeOpenPullRequests(ctx, builderloop.PullRequestIntakeOptions{
			Runner:         runner,
			RepoRoot:       cfg.RepoRoot,
			RunRoot:        cfg.RunRoot,
			RunID:          runID,
			ConflictAction: cfg.PRConflictAction,
		}); err != nil {
			return RunSummary{}, err
		}
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
	if len(opts.Keywords) > 0 {
		bundle = FilterContextByKeywords(bundle, opts.Keywords)
	}

	// Consume any autoloop trigger events queued since the last planner
	// run. The cursor advances in a deferred call below so a regen
	// rejection or backend failure still moves past these events — they
	// represent state transitions, not work to retry. LoadCursor and
	// ReadTriggersSinceCursor soft-fail to empty so a missing/corrupt
	// ledger never blocks the run.
	cursor, _ := plannertriggers.LoadCursor(cfg.TriggersCursorPath)
	triggerEvents, _ := plannertriggers.ReadTriggersSinceCursor(cfg.PlannerTriggersPath, cursor)
	// Phase D priority: event (quarantine triggers from autoloop) wins over
	// impl_change (impl-tree path unit, Phase D Task 4) which wins over
	// scheduled (timer). cfg.TriggerReason is set from PLANNER_TRIGGER_REASON
	// so the impl-path unit can stamp the run via env without dragging an
	// extra arg through the wrapper script.
	trigger := "scheduled"
	if len(triggerEvents) > 0 {
		trigger = "event"
	} else if cfg.TriggerReason == "impl_change" {
		trigger = "impl_change"
	}
	bundle.TriggerEvents = triggerEvents
	defer func() {
		// Dry-run is purely observational — leave the cursor where it is
		// so the operator can re-run with --dry-run as many times as they
		// want without burning trigger events. Real runs (success OR
		// failure) advance the cursor: trigger events represent state
		// transitions, not work to retry.
		if opts.DryRun {
			return
		}
		if len(triggerEvents) > 0 {
			newCursor := plannertriggers.TriggerCursor{
				LastConsumedID: triggerEvents[len(triggerEvents)-1].ID,
				LastReadAt:     now.UTC().Format(time.RFC3339),
			}
			_ = plannertriggers.SaveCursor(cfg.TriggersCursorPath, newCursor)
		}
	}()
	triggerEventIDs := func() []string {
		if len(triggerEvents) == 0 {
			return nil
		}
		ids := make([]string, 0, len(triggerEvents))
		for _, ev := range triggerEvents {
			ids = append(ids, ev.ID)
		}
		return ids
	}()

	contextPath := filepath.Join(cfg.RunRoot, "context.json")
	promptPath := filepath.Join(cfg.RunRoot, "latest_prompt.txt")
	reportPath := filepath.Join(cfg.RunRoot, "latest_planner_report.md")
	rawReportPath := filepath.Join(cfg.RunRoot, "latest_planner_report.raw.md")
	statePath := filepath.Join(cfg.RunRoot, "planner_state.json")
	validationLogPath := filepath.Join(cfg.RunRoot, "validation.log")

	// L4 self-evaluation: correlate the planner's own ledger with autoloop's
	// to surface whether previous reshapes unstuck the row, are still
	// failing, or haven't been retried yet. Errors are swallowed so a missing
	// or corrupt ledger never blocks the run; the planner just gets an empty
	// PreviousReshapes section in that case (handled by formatPreviousReshapes).
	ledgerPath := filepath.Join(cfg.RunRoot, "state", "runs.jsonl")
	bundle.PreviousReshapes, _ = Evaluate(ledgerPath, autoloopLedgerPath(cfg), cfg.EvaluationWindow, now)

	if err := writeContext(contextPath, bundle); err != nil {
		return RunSummary{}, err
	}
	prompt := BuildPrompt(bundle, opts.Keywords)
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return RunSummary{}, err
	}

	summary := RunSummary{
		RunID:         runID,
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

	// Snapshot progress.json BEFORE the LLM backend runs so we can verify
	// the backend's edits preserved every existing Health block. Health
	// metadata is owned by the autoloop runtime; the planner is only
	// allowed to update spec fields. A missing file is fine — it means
	// there is nothing to preserve yet.
	beforeDoc, err := loadProgressForValidation(cfg.ProgressJSON)
	if err != nil {
		return RunSummary{}, fmt.Errorf("planner: load before-doc: %w", err)
	}
	if err := validateRuntimeSourcePreflightClean(cfg.RepoRoot); err != nil {
		appendPlannerLedger(ledgerPath, LedgerEvent{
			TS:            now.UTC().Format(time.RFC3339),
			RunID:         runID,
			Trigger:       trigger,
			TriggerEvents: triggerEventIDs,
			Backend:       cfg.Backend,
			Mode:          cfg.Mode,
			Status:        "validation_rejected",
			Detail:        err.Error(),
			BeforeStats:   computeStats(beforeDoc),
			Keywords:      opts.Keywords,
		})
		return RunSummary{}, fmt.Errorf("planner: runtime source preflight rejected: %w", err)
	}
	beforeRuntime, err := snapshotRuntimeSources(cfg.RepoRoot)
	if err != nil {
		return RunSummary{}, fmt.Errorf("planner: snapshot runtime sources: %w", err)
	}

	argv, err := plannerBackendCommand(cfg.Backend, cfg.Mode, rawReportPath)
	if err != nil {
		return RunSummary{}, err
	}

	// L3 retry-with-feedback loop. The initial attempt uses the prompt built
	// above; any retry appends RetryFeedback() naming the rows whose Health
	// blocks were dropped, so the same LLM session can self-correct without
	// re-doing the upstream sync analysis. Backend failures are NEVER
	// retried — only validation rejections trigger another attempt. The
	// before-doc is captured ONCE outside the loop so it is not reloaded
	// against in-flight autoloop writes between attempts.
	maxRetries := cfg.MaxRetries
	currentPrompt := prompt
	attempts := make([]retryAttempt, 0, maxRetries+1)
	var (
		afterDoc   *progress.Progress
		lastResult cmdrunner.Result
	)
	for i := 0; i <= maxRetries; i++ {
		attempt := retryAttempt{Index: i}
		if err := clearRawReport(rawReportPath); err != nil {
			return RunSummary{}, fmt.Errorf("planner: clear raw report: %w", err)
		}
		backendCtx := ctx
		cancelBackend := func() {}
		if cfg.BackendTimeout > 0 {
			backendCtx, cancelBackend = context.WithTimeout(ctx, cfg.BackendTimeout)
		}
		attemptIndex := i
		startTime := time.Now()
		progressTicker := time.NewTicker(30 * time.Second)
		progressDone := make(chan struct{}, 1)
		go func() {
			for {
				select {
				case <-progressTicker.C:
					elapsed := time.Since(startTime).Round(time.Second)
					appendPlannerLedger(ledgerPath, LedgerEvent{
						TS:           time.Now().UTC().Format(time.RFC3339),
						RunID:        runID,
						Event:        "backend_progress",
						Status:       "running",
						Detail:       fmt.Sprintf("elapsed=%s attempt=%d", elapsed, attemptIndex),
						Backend:      cfg.Backend,
						Mode:         cfg.Mode,
						RetryAttempt: attemptIndex,
					})
				case <-progressDone:
					return
				}
			}
		}()
		watchdogTicker := time.NewTicker(2 * time.Minute)
		watchdogDone := make(chan struct{}, 1)
		go func() {
			for {
				select {
				case <-watchdogTicker.C:
					elapsed := time.Since(startTime).Round(time.Second)
					status := "backend_slow"
					if elapsed > 5*time.Minute {
						status = "backend_stuck"
					}
					appendPlannerLedger(ledgerPath, LedgerEvent{
						TS:           time.Now().UTC().Format(time.RFC3339),
						RunID:        runID,
						Event:        status,
						Status:       "warning",
						Detail:       fmt.Sprintf("no_output_for=%s attempt=%d", elapsed, attemptIndex),
						Backend:      cfg.Backend,
						Mode:         cfg.Mode,
						RetryAttempt: attemptIndex,
					})
				case <-watchdogDone:
					return
				}
			}
		}()
		result := runner.Run(backendCtx, cmdrunner.Command{
			Name: argv[0],
			Args: append(append([]string(nil), argv[1:]...), currentPrompt),
			Dir:  cfg.RepoRoot,
		})
		progressTicker.Stop()
		watchdogTicker.Stop()
		close(progressDone)
		close(watchdogDone)
		cancelBackend()
		lastResult = result
		if result.Err != nil {
			// Backend failure short-circuits the retry loop: this is
			// infrastructure, not an LLM-correctable mistake.
			failure := plannerBackendFailure(result)
			detail := failure.Detail
			attempt.Status = "backend_failed"
			attempt.Detail = detail
			attempts = append(attempts, attempt)
			appendPlannerLedger(ledgerPath, LedgerEvent{
				TS:            now.UTC().Format(time.RFC3339),
				RunID:         runID,
				Event:         failure.Event,
				Trigger:       trigger,
				TriggerEvents: triggerEventIDs,
				Backend:       cfg.Backend,
				Mode:          cfg.Mode,
				Status:        "backend_failed",
				Detail:        detail,
				BeforeStats:   computeStats(beforeDoc),
				Keywords:      opts.Keywords,
				RetryAttempt:  attempt.Index,
				Attempts:      attempts,
			})
			return RunSummary{}, commandError(argv[0], result)
		}
		if vErr := validateRuntimeSourceScope(beforeRuntime, cfg.RepoRoot); vErr != nil {
			attempt.Status = "validation_rejected"
			attempt.Detail = vErr.Error()
			attempts = append(attempts, attempt)
			appendPlannerLedger(ledgerPath, LedgerEvent{
				TS:            now.UTC().Format(time.RFC3339),
				RunID:         runID,
				Trigger:       trigger,
				TriggerEvents: triggerEventIDs,
				Backend:       cfg.Backend,
				Mode:          cfg.Mode,
				Status:        "validation_rejected",
				Detail:        vErr.Error(),
				BeforeStats:   computeStats(beforeDoc),
				Keywords:      opts.Keywords,
				RetryAttempt:  attempt.Index,
				Attempts:      attempts,
			})
			return RunSummary{}, fmt.Errorf("planner: regeneration rejected: %w", vErr)
		}

		// Reload progress.json after the backend's edits and check Health
		// preservation. Skipped when there was no before-doc (fresh
		// checkout) or when the after-doc cannot be parsed.
		afterDoc = nil
		if beforeDoc != nil {
			loaded, loadErr := loadProgressForValidation(cfg.ProgressJSON)
			if loadErr != nil {
				return RunSummary{}, fmt.Errorf("planner: load after-doc: %w", loadErr)
			}
			afterDoc = loaded
		}
		if beforeDoc != nil && afterDoc != nil {
			if vErr := validateHealthPreservation(beforeDoc, afterDoc); vErr != nil {
				attempt.Status = "validation_rejected"
				attempt.Detail = vErr.Error()
				attempt.DroppedRows = extractDroppedRows(beforeDoc, afterDoc)
				attempts = append(attempts, attempt)
				if i == maxRetries {
					appendPlannerLedger(ledgerPath, LedgerEvent{
						TS:            now.UTC().Format(time.RFC3339),
						RunID:         runID,
						Trigger:       trigger,
						TriggerEvents: triggerEventIDs,
						Backend:       cfg.Backend,
						Mode:          cfg.Mode,
						Status:        "validation_rejected",
						Detail:        vErr.Error(),
						BeforeStats:   computeStats(beforeDoc),
						AfterStats:    computeStats(afterDoc),
						RowsChanged:   diffRows(beforeDoc, afterDoc),
						Keywords:      opts.Keywords,
						RetryAttempt:  attempt.Index,
						Attempts:      attempts,
					})
					return RunSummary{}, fmt.Errorf("planner: regeneration rejected: %w", vErr)
				}
				// Build a corrective follow-up prompt naming the dropped
				// rows. The original prompt is preserved as the prefix so
				// the LLM keeps full context (sync results, quarantine
				// priorities, etc.).
				currentPrompt = prompt + RetryFeedback(vErr, beforeDoc, afterDoc)
				continue
			}
		}
		attempt.Status = "ok"
		attempts = append(attempts, attempt)
		break
	}
	// Result used by writeReport below comes from the last successful attempt.
	result := lastResult

	if err := writeReport(reportPath, rawReportPath, result, bundle, now); err != nil {
		return RunSummary{}, err
	}

	// Spec rows that changed in this run feed the verdict-stamping pass
	// (Phase C Task 11) and every terminal ledger event, including
	// post-backend validation failures.
	rowsChanged := diffRows(beforeDoc, afterDoc)
	finalAttempt := 0
	if len(attempts) > 0 {
		finalAttempt = attempts[len(attempts)-1].Index
	}

	if cfg.Validate && !opts.SkipValidation {
		if err := runValidation(ctx, runner, cfg.RepoRoot, validationLogPath); err != nil {
			appendPlannerLedger(ledgerPath, LedgerEvent{
				TS:            now.UTC().Format(time.RFC3339),
				RunID:         runID,
				Trigger:       trigger,
				TriggerEvents: triggerEventIDs,
				Backend:       cfg.Backend,
				Mode:          cfg.Mode,
				Status:        "validation_failed",
				Detail:        err.Error(),
				BeforeStats:   computeStats(beforeDoc),
				AfterStats:    computeStats(afterDoc),
				RowsChanged:   rowsChanged,
				Keywords:      opts.Keywords,
				RetryAttempt:  finalAttempt,
				Attempts:      attempts,
			})
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

	// L5 verdict stamping: deterministic post-processing that increments
	// PlannerVerdict.ReshapeCount on reshaped rows and sets sticky
	// NeedsHuman when L4 outcomes show persistent failure past the
	// escalation threshold. Must happen AFTER validateHealthPreservation
	// passes (so we trust the LLM's regen) and BEFORE the final ledger
	// append (so verdict_set RowChanges land in the same entry).
	var verdictChanges []RowChange
	if afterDoc != nil {
		verdictChanges = StampVerdicts(afterDoc, rowsChanged, bundle.PreviousReshapes, cfg.EscalationThreshold, now)
		if len(verdictChanges) > 0 {
			if err := progress.SaveProgress(cfg.ProgressJSON, afterDoc); err != nil {
				return RunSummary{}, fmt.Errorf("planner: save verdicts: %w", err)
			}
		}
	}

	runStatus := "ok"
	if beforeDoc == nil || afterDoc == nil {
		runStatus = "no_changes"
	} else if len(verdictChanges) > 0 {
		// At least one row's PlannerVerdict materially changed. If any of
		// those changes set NeedsHuman, surface that as the run status so
		// operators can spot escalations in the ledger at a glance.
		for _, rc := range verdictChanges {
			if rc.Detail == "needs_human=true" {
				runStatus = "needs_human_set"
				break
			}
		}
	}
	combinedRows := append([]RowChange(nil), rowsChanged...)
	combinedRows = append(combinedRows, verdictChanges...)

	// Phase D Task 5: forensics for subphase DriftState transitions. Only
	// forward edges are emitted as DriftPromotions; backward edges are logged
	// (planner runtime never demotes — humans demote via direct edit).
	driftPromotions := diffSubphaseStates(beforeDoc, afterDoc)

	appendPlannerLedger(ledgerPath, LedgerEvent{
		TS:              now.UTC().Format(time.RFC3339),
		RunID:           runID,
		Trigger:         trigger,
		TriggerEvents:   triggerEventIDs,
		Backend:         cfg.Backend,
		Mode:            cfg.Mode,
		Status:          runStatus,
		BeforeStats:     computeStats(beforeDoc),
		AfterStats:      computeStats(afterDoc),
		RowsChanged:     combinedRows,
		Keywords:        opts.Keywords,
		RetryAttempt:    finalAttempt,
		Attempts:        attempts,
		DriftPromotions: driftPromotions,
	})

	return summary, nil
}

// appendPlannerLedger writes one LedgerEvent and soft-fails on error: the
// ledger is observability, not the planner run's success criterion. Errors
// are logged via the standard log package so operators see them, but they
// do not fail the run.
func appendPlannerLedger(path string, event LedgerEvent) {
	if err := AppendLedgerEvent(path, event); err != nil {
		log.Printf("planner: append ledger failed: %v", err)
	}
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

	argv, err := builderloop.BuildBackendCommand(backend, mode)
	if err != nil {
		return nil, err
	}
	return append(argv, "--output-last-message", rawReportPath), nil
}

func runValidation(ctx context.Context, runner cmdrunner.Runner, repoRoot, logPath string) error {
	env := validationCommandEnv()
	commands := []cmdrunner.Command{
		{Name: "go", Args: []string{"run", "./cmd/builder-loop", "progress", "write"}, Dir: repoRoot, Env: env},
		{Name: "go", Args: []string{"run", "./cmd/builder-loop", "progress", "validate"}, Dir: repoRoot, Env: env},
		{Name: "go", Args: []string{"test", "./internal/progress", "-count=1"}, Dir: repoRoot, Env: env},
		{Name: "go", Args: []string{"test", "./docs", "-count=1"}, Dir: repoRoot, Env: env},
		{Name: "go", Args: []string{"test", "./...", "-count=1"}, Dir: filepath.Join(repoRoot, "www.gormes.ai"), Env: env},
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

func validationCommandEnv() []string {
	path := os.Getenv("PATH")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		goBin := filepath.Join(home, "go", "bin")
		if !pathContains(path, goBin) {
			if path == "" {
				path = goBin
			} else {
				path = goBin + string(os.PathListSeparator) + path
			}
		}
	}
	if path == "" {
		return nil
	}
	return []string{"PATH=" + path}
}

func pathContains(pathValue, dir string) bool {
	for _, entry := range filepath.SplitList(pathValue) {
		if filepath.Clean(entry) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

func writeReport(path, rawPath string, result cmdrunner.Result, bundle ContextBundle, now time.Time) error {
	raw := backendReportText(rawPath, result)
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

func backendReportText(rawPath string, result cmdrunner.Result) string {
	if data, err := os.ReadFile(rawPath); err == nil && strings.TrimSpace(string(data)) != "" {
		return strings.TrimSpace(string(data))
	}
	stdout := strings.TrimSpace(result.Stdout)
	if text, ok := finalAgentMessageFromJSONStream(stdout); ok {
		return strings.TrimSpace(text)
	}
	return stdout
}

func finalAgentMessageFromJSONStream(output string) (string, bool) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", false
	}

	var lastAgentMessage string
	sawJSON := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event struct {
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return "", false
		}
		sawJSON = true
		if event.Item.Type == "agent_message" && strings.TrimSpace(event.Item.Text) != "" {
			lastAgentMessage = event.Item.Text
		}
	}
	return strings.TrimSpace(lastAgentMessage), sawJSON
}

func clearRawReport(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeState(path string, state stateFile) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func commandError(name string, result cmdrunner.Result) error {
	detail := plannerFailureDetail(result)
	if detail == "" {
		return fmt.Errorf("%s failed: %w", name, result.Err)
	}
	return fmt.Errorf("%s failed: %w: %s", name, result.Err, detail)
}

func plannerFailureDetail(result cmdrunner.Result) string {
	return plannerBackendFailure(result).Detail
}

func plannerBackendFailure(result cmdrunner.Result) plannerBackendFailureClassification {
	failure := builderloop.ClassifyBackendFailure(result.Err, result.Stdout, result.Stderr)
	event := ""
	if failure.Status != "backend_failed" {
		event = failure.Status
	}
	return plannerBackendFailureClassification{Event: event, Detail: failure.Detail}
}

type plannerBackendFailureClassification struct {
	Event  string
	Detail string
}

type runtimeSourceSnapshot map[string][sha256.Size]byte

func snapshotRuntimeSources(repoRoot string) (runtimeSourceSnapshot, error) {
	snapshot := make(runtimeSourceSnapshot)
	for _, rootRel := range []string{"cmd", "internal"} {
		root := filepath.Join(repoRoot, rootRel)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			snapshot[filepath.ToSlash(rel)] = sha256.Sum256(data)
			return nil
		})
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
	}
	return snapshot, nil
}

func validateRuntimeSourceScope(before runtimeSourceSnapshot, repoRoot string) error {
	after, err := snapshotRuntimeSources(repoRoot)
	if err != nil {
		return fmt.Errorf("snapshot runtime sources: %w", err)
	}
	changes := diffRuntimeSources(before, after)
	if len(changes) == 0 {
		return nil
	}
	return fmt.Errorf("planner output modified runtime source files: %s", strings.Join(changes, ", "))
}

func validateRuntimeSourcePreflightClean(repoRoot string) error {
	changes, err := dirtyRuntimeSourceChanges(repoRoot)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return nil
	}
	return fmt.Errorf("runtime source files are already dirty: %s", strings.Join(changes, ", "))
}

func dirtyRuntimeSourceChanges(repoRoot string) ([]string, error) {
	if repoRoot == "" {
		return nil, nil
	}
	if err := exec.Command("git", "-C", repoRoot, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return nil, nil
	}
	topOutput, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, nil
	}
	top := filepath.Clean(strings.TrimSpace(string(topOutput)))
	root := filepath.Clean(repoRoot)
	if absTop, err := filepath.Abs(top); err == nil {
		top = absTop
	}
	if absRoot, err := filepath.Abs(root); err == nil {
		root = absRoot
	}
	if top != root {
		return nil, nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "status", "--porcelain=v1", "--untracked-files=all", "--", "cmd", "internal")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git status runtime sources: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git status runtime sources: %w", err)
	}
	var changes []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" || len(line) < 4 {
			continue
		}
		status := line[:2]
		path := strings.TrimSpace(line[2:])
		if idx := strings.LastIndex(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		path = strings.Trim(path, `"`)
		if filepath.Ext(path) != ".go" {
			continue
		}
		changes = append(changes, runtimeGitStatusLabel(status)+" "+filepath.ToSlash(path))
	}
	sort.Strings(changes)
	return changes, nil
}

func runtimeGitStatusLabel(status string) string {
	switch {
	case strings.Contains(status, "?"):
		return "untracked"
	case strings.Contains(status, "D"):
		return "deleted"
	case strings.Contains(status, "R"):
		return "renamed"
	case strings.Contains(status, "A"):
		return "added"
	default:
		return "modified"
	}
}

func diffRuntimeSources(before, after runtimeSourceSnapshot) []string {
	var changes []string
	for path, afterHash := range after {
		beforeHash, ok := before[path]
		switch {
		case !ok:
			changes = append(changes, "added "+path)
		case beforeHash != afterHash:
			changes = append(changes, "modified "+path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changes = append(changes, "deleted "+path)
		}
	}
	sort.Strings(changes)
	return changes
}

// loadProgressForValidation reads progress.json for the health-preservation
// gate. Returns (nil, nil) when the file does not exist so the gate skips
// gracefully on a fresh checkout (there is no prior state to preserve).
// Other read/parse errors propagate so the planner refuses to silently
// proceed against a corrupted progress.json.
func loadProgressForValidation(path string) (*progress.Progress, error) {
	prog, err := progress.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return prog, nil
}

// validateHealthPreservation rejects planner regenerations that drop or
// modify any existing Health block. Rows missing from the after-doc are
// considered intentional deletions (planner removed them) and pass.
// Spec hash mismatch is NOT validated here — that triggers stale-clear
// in autoloop's selection layer (L3), not a planner-side rejection.
func validateHealthPreservation(before, after *progress.Progress) error {
	beforeIndex := indexItems(before)
	afterIndex := indexItems(after)

	for key, beforeItem := range beforeIndex {
		afterItem, exists := afterIndex[key]
		if !exists {
			continue // intentional deletion
		}
		if !healthEqual(beforeItem.Health, afterItem.Health) {
			return fmt.Errorf("planner output dropped or modified health block for %s/%s/%s",
				key.phaseID, key.subphaseID, key.itemName)
		}
	}
	return nil
}

type itemKey struct{ phaseID, subphaseID, itemName string }

// indexItems flattens a Progress document into a map keyed by
// (phaseID, subphaseID, itemName). Returns an empty map when prog is nil.
// Item pointers are taken from the underlying slice so callers can read
// fields without copying the whole row.
func indexItems(prog *progress.Progress) map[itemKey]*progress.Item {
	out := map[itemKey]*progress.Item{}
	if prog == nil {
		return out
	}
	for phaseID, phase := range prog.Phases {
		for subID, sub := range phase.Subphases {
			for i := range sub.Items {
				it := &sub.Items[i]
				out[itemKey{phaseID, subID, it.Name}] = it
			}
		}
	}
	return out
}

// healthEqual compares two RowHealth pointers for deep equality, treating
// (nil, nil) as equal but (nil, non-nil) or (non-nil, nil) as different.
func healthEqual(a, b *progress.RowHealth) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}

// computeStats walks a Progress doc and counts rows by status, including
// the new Phase C buckets (Quarantined, NeedsHuman) which aren't in the
// existing Progress.Stats() function. Returns a zero ProgressStats when
// prog is nil so the helper is safe on the dry-run / no-before-doc paths.
func computeStats(prog *progress.Progress) ProgressStats {
	if prog == nil {
		return ProgressStats{}
	}
	var stats ProgressStats
	for _, phase := range prog.Phases {
		for _, sub := range phase.Subphases {
			for i := range sub.Items {
				it := &sub.Items[i]
				switch it.Status {
				case progress.StatusComplete:
					stats.Shipped++
				case progress.StatusInProgress:
					stats.InProgress++
				default:
					stats.Planned++
				}
				if it.Health != nil && it.Health.Quarantine != nil {
					stats.Quarantined++
				}
				if it.PlannerVerdict != nil && it.PlannerVerdict.NeedsHuman {
					stats.NeedsHuman++
				}
			}
		}
	}
	return stats
}

// diffRows compares before/after docs and returns RowChange records for
// added/deleted/spec_changed rows. Spec change is detected via
// progress.ItemSpecHash so cosmetic edits don't show up as changes. Returns
// nil when both inputs are nil/empty.
func diffRows(before, after *progress.Progress) []RowChange {
	var out []RowChange
	beforeIndex := indexItems(before)
	afterIndex := indexItems(after)

	for key, beforeItem := range beforeIndex {
		afterItem, exists := afterIndex[key]
		if !exists {
			out = append(out, RowChange{
				PhaseID:    key.phaseID,
				SubphaseID: key.subphaseID,
				ItemName:   key.itemName,
				Kind:       "deleted",
			})
			continue
		}
		if progress.ItemSpecHash(beforeItem) != progress.ItemSpecHash(afterItem) {
			out = append(out, RowChange{
				PhaseID:    key.phaseID,
				SubphaseID: key.subphaseID,
				ItemName:   key.itemName,
				Kind:       "spec_changed",
			})
		}
	}
	for key := range afterIndex {
		if _, existed := beforeIndex[key]; !existed {
			out = append(out, RowChange{
				PhaseID:    key.phaseID,
				SubphaseID: key.subphaseID,
				ItemName:   key.itemName,
				Kind:       "added",
			})
		}
	}
	return out
}
