package autoloop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

type RunOptions struct {
	Config Config
	Runner Runner
	DryRun bool
	Now    time.Time
}

type RunSummary struct {
	Candidates int
	Selected   []Candidate
	RunID      string
}

type workerRun struct {
	ID           int
	Candidate    Candidate
	Task         string
	Branch       string
	BaseCommit   string
	RepoRoot     string
	WorktreePath string
	Result       Result
}

func RunOnce(ctx context.Context, opts RunOptions) (RunSummary, error) {
	candidates, err := NormalizeCandidates(opts.Config.ProgressJSON, CandidateOptions{
		ActiveFirst:   true,
		PriorityBoost: opts.Config.PriorityBoost,
		MaxPhase:      opts.Config.MaxPhase,
	})
	if err != nil {
		return RunSummary{}, err
	}

	selected := selectAcrossSubphases(candidates, opts.Config.MaxAgents)
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := now.UTC().Format("20060102T150405Z")

	summary := RunSummary{
		Candidates: len(candidates),
		Selected:   append([]Candidate(nil), selected...),
		RunID:      runID,
	}
	if opts.DryRun {
		return summary, nil
	}
	if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "run_started",
		Status: "started",
	}); err != nil {
		return RunSummary{}, err
	}
	if err := preflightCleanWorktree(opts.Config, runID); err != nil {
		return RunSummary{}, err
	}

	// Reactive autoloop wiring (Tasks 3-4): the per-run accumulator captures
	// success / failure outcomes for each candidate; Flush at the end of the
	// run mutates progress.json in one batched write.
	acc := newHealthAccumulator(runID, time.Now, opts.Config.QuarantineThreshold)
	chain := opts.Config.BackendFallback
	if len(chain) == 0 {
		chain = []string{opts.Config.Backend}
	}
	degrader := newBackendDegrader(chain, opts.Config.BackendDegradeThreshold)

	// Forward Task-5 stale-quarantine signals into the accumulator now so the
	// wiring is in place. Today this is a no-op because StaleQuarantine is
	// always false; Task 5 will set it during selection.
	for _, c := range selected {
		if c.StaleQuarantine {
			acc.MarkStaleQuarantine(c)
		}
	}

	// flushHealth runs at every return path that might have recorded outcomes.
	// Errors from Flush are best-effort: we surface them in the ledger but do
	// not fail the run, since worker outcomes are already promoted at this
	// point.
	//
	// If the live progress.json is in a legacy or test format that the
	// accumulator cannot resolve (e.g. items keyed by "item_name" instead of
	// the canonical "name"), Flush would fail with "item not found" for every
	// row. To avoid noisy ledger churn in that case we probe before flushing
	// and emit no health event when no recorded row resolves to a real item.
	flushHealth := func() {
		hashOf := func(phaseID, subphaseID, itemName string) string {
			prog, err := progress.Load(opts.Config.ProgressJSON)
			if err != nil {
				return ""
			}
			phase, ok := prog.Phases[phaseID]
			if !ok {
				return ""
			}
			sub, ok := phase.Subphases[subphaseID]
			if !ok {
				return ""
			}
			for i := range sub.Items {
				if sub.Items[i].Name == itemName {
					return progress.ItemSpecHash(&sub.Items[i])
				}
			}
			return ""
		}

		if !accumulatorRowsResolve(acc, opts.Config.ProgressJSON) {
			return
		}

		if err := acc.Flush(opts.Config.ProgressJSON, hashOf); err != nil {
			_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "health_update_failed",
				Status: "failed",
				Detail: err.Error(),
			})
			return
		}
		_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "health_updated",
			Status: "ok",
		})
	}

	// observeOutcome feeds the degrader and emits backend_degraded ledger
	// events on switch. Called sequentially after each finishWorker so it is
	// safe with no synchronization.
	observeOutcome := func(out workerOutcome) {
		switched, from, to := degrader.ObserveOutcome(out)
		if !switched {
			return
		}
		_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "backend_degraded",
			Status: "degraded",
			Detail: fmt.Sprintf("from=%s to=%s threshold=%d", from, to, opts.Config.BackendDegradeThreshold),
		})
	}

	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	argv, err := BuildBackendCommand(opts.Config.Backend, opts.Config.Mode)
	if err != nil {
		return RunSummary{}, err
	}

	hasGit := repoHasGit(opts.Config.RepoRoot)
	var baseBranch string
	if hasGit {
		baseBranch, err = gitCurrentBranch(opts.Config.RepoRoot)
		if err != nil {
			return RunSummary{}, err
		}
	}

	if hasGit && len(selected) > 1 {
		workers, skipped, err := prepareGitWorkers(opts.Config, runID, selected)
		if err != nil {
			return RunSummary{}, err
		}
		for _, sk := range skipped {
			acc.RecordFailure(sk.Candidate, progress.FailureProgressSummary, degrader.Current(), sk.Reason)
		}
		runBackendWorkers(ctx, runner, argv, workers)
		for _, worker := range workers {
			finishErr := finishWorker(ctx, opts.Config, runner, argv[0], runID, baseBranch, hasGit, worker)
			recordWorkerOutcome(acc, observeOutcome, degrader.Current(), worker, finishErr)
			if finishErr != nil {
				flushHealth()
				return RunSummary{}, finishErr
			}
		}
		if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "run_completed",
			Status: "completed",
		}); err != nil {
			flushHealth()
			return RunSummary{}, err
		}
		flushHealth()

		return summary, nil
	}

	for i, candidate := range selected {
		worker := workerRun{
			ID:        i + 1,
			Candidate: candidate,
			Task:      candidateTaskName(candidate),
			RepoRoot:  opts.Config.RepoRoot,
		}
		if hasGit {
			worker.Branch = WorkerBranchName(runID, worker.ID, candidate)
			worker.WorktreePath = WorkerWorktreePath(opts.Config, runID, worker.ID)
		}
		if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_claimed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "claimed",
		}); err != nil {
			flushHealth()
			return RunSummary{}, err
		}

		if hasGit {
			if err := gitCreateWorkerWorktree(opts.Config.RepoRoot, worker.WorktreePath, worker.Branch); err != nil {
				// Soft-skip per-candidate setup failure: emit a candidate_skipped
				// ledger event, record the failure to the accumulator, and move
				// on to the next candidate so a single broken row cannot stall
				// an otherwise-healthy run.
				_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
					TS:     time.Now().UTC(),
					RunID:  runID,
					Event:  "candidate_skipped",
					Worker: worker.ID,
					Task:   worker.Task,
					Branch: worker.Branch,
					Status: "worktree_create_failed",
					Detail: err.Error(),
				})
				acc.RecordFailure(candidate, progress.FailureProgressSummary, degrader.Current(), err.Error())
				continue
			}
			worker.RepoRoot = worker.WorktreePath
			worker.BaseCommit, err = gitHeadSha(worker.RepoRoot)
			if err != nil {
				_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
					TS:     time.Now().UTC(),
					RunID:  runID,
					Event:  "candidate_skipped",
					Worker: worker.ID,
					Task:   worker.Task,
					Branch: worker.Branch,
					Status: "head_sha_failed",
					Detail: err.Error(),
				})
				acc.RecordFailure(candidate, progress.FailureProgressSummary, degrader.Current(), err.Error())
				continue
			}
		}

		args := append([]string(nil), argv[1:]...)
		args = append(args, BuildWorkerPromptWithBranch(candidate, worker.Branch))
		worker.Result = runner.Run(ctx, Command{
			Name: argv[0],
			Args: args,
			Dir:  worker.RepoRoot,
		})
		finishErr := finishWorker(ctx, opts.Config, runner, argv[0], runID, baseBranch, hasGit, worker)
		recordWorkerOutcome(acc, observeOutcome, degrader.Current(), worker, finishErr)
		if finishErr != nil {
			flushHealth()
			return RunSummary{}, finishErr
		}
	}
	if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "run_completed",
		Status: "completed",
	}); err != nil {
		flushHealth()
		return RunSummary{}, err
	}
	flushHealth()

	return summary, nil
}

// recordWorkerOutcome translates a finishWorker result into accumulator +
// degrader observations. Called sequentially after each worker drains, so it
// is safe to mutate the accumulator without a mutex.
func recordWorkerOutcome(
	acc *healthAccumulator,
	observe func(workerOutcome),
	currentBackend string,
	worker workerRun,
	finishErr error,
) {
	if finishErr == nil {
		acc.RecordSuccess(worker.Candidate)
		observe(workerOutcome{IsSuccessFlag: true, Backend: currentBackend})
		return
	}
	cat := mapFinishErrorToCategory(worker, finishErr)
	stderrTail := worker.Result.Stderr
	if stderrTail == "" {
		stderrTail = finishErr.Error()
	}
	acc.RecordFailure(worker.Candidate, cat, currentBackend, stderrTail)

	out := workerOutcome{
		Backend:  currentBackend,
		Commit:   "",
		Category: string(cat),
	}
	// Only flag as a backend error when the worker exited non-zero AND
	// produced no commit. This protects against treating row-level failures
	// (dirty worktree, scope leak, etc.) as backend infra failures.
	if worker.Result.Err != nil && cat == progress.FailureWorkerError {
		out.IsBackendErrorFlag = true
	}
	observe(out)
}

// accumulatorRowsResolve returns true when at least one row currently held
// by the accumulator can be resolved against the progress.json on disk. Used
// to silence health ledger events when running against a legacy/test progress
// file that uses non-canonical item keys.
func accumulatorRowsResolve(acc *healthAccumulator, path string) bool {
	if acc == nil || len(acc.rows) == 0 {
		return false
	}
	prog, err := progress.Load(path)
	if err != nil {
		return false
	}
	for key := range acc.rows {
		phase, ok := prog.Phases[key.phaseID]
		if !ok {
			continue
		}
		sub, ok := phase.Subphases[key.subphaseID]
		if !ok {
			continue
		}
		for i := range sub.Items {
			if sub.Items[i].Name == key.itemName {
				return true
			}
		}
	}
	return false
}

// mapFinishErrorToCategory classifies a finishWorker error into the closed
// FailureCategory set. Errors not unambiguously classifiable default to
// FailureWorkerError so quarantine math still progresses.
func mapFinishErrorToCategory(worker workerRun, err error) progress.FailureCategory {
	if err == nil {
		return ""
	}
	// A backend exit error (worker.Result.Err != nil) is always worker_error.
	if worker.Result.Err != nil {
		return progress.FailureWorkerError
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "outside declared write scope"):
		return progress.FailureReportValidation
	case strings.Contains(msg, "worker branch changed"):
		return progress.FailureWorkerError
	case strings.Contains(msg, "uncommitted changes"):
		return progress.FailureWorkerError
	case strings.Contains(msg, "unresolved merge conflicts"):
		return progress.FailureWorkerError
	}
	return progress.FailureWorkerError
}

// prepareGitWorkers builds workerRun entries for each selected candidate in
// the concurrent multi-worker path. Per-candidate setup failures (worktree
// create, head-sha lookup) are soft-skipped: a candidate_skipped ledger event
// is emitted and the failure is reported via the skipped slice so the caller
// can record it in the health accumulator. The returned worker slice contains
// only successfully-prepared workers (it may be shorter than selected).
func prepareGitWorkers(cfg Config, runID string, selected []Candidate) (workers []workerRun, skipped []skippedCandidate, err error) {
	workers = make([]workerRun, 0, len(selected))
	for i, candidate := range selected {
		worker := workerRun{
			ID:           i + 1,
			Candidate:    candidate,
			Task:         candidateTaskName(candidate),
			Branch:       WorkerBranchName(runID, i+1, candidate),
			RepoRoot:     WorkerWorktreePath(cfg, runID, i+1),
			WorktreePath: WorkerWorktreePath(cfg, runID, i+1),
		}
		if claimErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_claimed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "claimed",
		}); claimErr != nil {
			return nil, nil, claimErr
		}
		if createErr := gitCreateWorkerWorktree(cfg.RepoRoot, worker.WorktreePath, worker.Branch); createErr != nil {
			_ = appendRunLedgerEvent(cfg, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "candidate_skipped",
				Worker: worker.ID,
				Task:   worker.Task,
				Branch: worker.Branch,
				Status: "worktree_create_failed",
				Detail: createErr.Error(),
			})
			skipped = append(skipped, skippedCandidate{Candidate: candidate, Reason: createErr.Error()})
			continue
		}
		baseCommit, headErr := gitHeadSha(worker.RepoRoot)
		if headErr != nil {
			_ = appendRunLedgerEvent(cfg, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "candidate_skipped",
				Worker: worker.ID,
				Task:   worker.Task,
				Branch: worker.Branch,
				Status: "head_sha_failed",
				Detail: headErr.Error(),
			})
			skipped = append(skipped, skippedCandidate{Candidate: candidate, Reason: headErr.Error()})
			continue
		}
		worker.BaseCommit = baseCommit
		workers = append(workers, worker)
	}
	return workers, skipped, nil
}

// skippedCandidate carries enough context for the run loop to record a
// per-candidate setup failure in the accumulator without re-deriving paths.
type skippedCandidate struct {
	Candidate Candidate
	Reason    string
}

func runBackendWorkers(ctx context.Context, runner Runner, argv []string, workers []workerRun) {
	var wg sync.WaitGroup
	for i := range workers {
		worker := &workers[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			args := append([]string(nil), argv[1:]...)
			args = append(args, BuildWorkerPromptWithBranch(worker.Candidate, worker.Branch))
			worker.Result = runner.Run(ctx, Command{
				Name: argv[0],
				Args: args,
				Dir:  worker.RepoRoot,
			})
		}()
	}
	wg.Wait()
}

func finishWorker(ctx context.Context, cfg Config, runner Runner, backendName string, runID string, baseBranch string, hasGit bool, worker workerRun) error {
	if worker.Result.Err != nil {
		if err := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "backend_failed",
		}); err != nil {
			return err
		}
		return backendRunError(backendName, worker.Result)
	}
	if hasGit {
		if err := ensureCurrentBranch(worker.RepoRoot, worker.Branch); err != nil {
			if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "worker_failed",
				Worker: worker.ID,
				Task:   worker.Task,
				Branch: worker.Branch,
				Status: "branch_changed",
				Detail: err.Error(),
			}); ledgerErr != nil {
				return ledgerErr
			}
			return err
		}
	}
	if err := ensureNoMergeConflicts(worker.RepoRoot); err != nil {
		if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "worktree_unmerged",
			Detail: err.Error(),
		}); ledgerErr != nil {
			return ledgerErr
		}
		return err
	}
	if err := ensureWorktreeClean(worker.RepoRoot); err != nil {
		if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "worktree_dirty",
			Detail: err.Error(),
		}); ledgerErr != nil {
			return ledgerErr
		}
		return err
	}
	var commitSha string
	if hasGit {
		var err error
		commitSha, err = gitHeadSha(worker.RepoRoot)
		if err != nil {
			return err
		}
		if commitSha != worker.BaseCommit {
			if err := ensureChangedPathsWithinWriteScope(worker.RepoRoot, worker.BaseCommit, commitSha, worker.Candidate); err != nil {
				if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
					TS:     time.Now().UTC(),
					RunID:  runID,
					Event:  "worker_failed",
					Worker: worker.ID,
					Task:   worker.Task,
					Branch: worker.Branch,
					Commit: commitSha,
					Status: "write_scope_violation",
					Detail: err.Error(),
				}); ledgerErr != nil {
					return ledgerErr
				}
				return err
			}
			if err := ensureCurrentBranch(cfg.RepoRoot, baseBranch); err != nil {
				return err
			}
			if err := promoteWorkerCommit(ctx, cfg, runner, runID, worker.ID, worker.Task, worker.Branch, commitSha); err != nil {
				return err
			}
			removeCleanWorkerWorktree(cfg.RepoRoot, worker.WorktreePath)
		} else {
			commitSha = ""
			if err := appendRunLedgerEvent(cfg, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "worker_no_changes",
				Worker: worker.ID,
				Task:   worker.Task,
				Branch: worker.Branch,
				Status: "no_changes",
			}); err != nil {
				return err
			}
			removeCleanWorkerWorktree(cfg.RepoRoot, worker.WorktreePath)
		}
	}

	return appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "worker_success",
		Worker: worker.ID,
		Task:   worker.Task,
		Branch: worker.Branch,
		Commit: commitSha,
		Status: "success",
	})
}

func appendRunLedgerEvent(cfg Config, event LedgerEvent) error {
	if cfg.RunRoot == "" {
		return nil
	}
	return AppendLedgerEvent(filepath.Join(cfg.RunRoot, "state", "runs.jsonl"), event)
}

func preflightCleanWorktree(cfg Config, runID string) error {
	if err := ensureNoMergeConflicts(cfg.RepoRoot); err != nil {
		if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "run_failed",
			Status: "worktree_unmerged",
			Detail: err.Error(),
		}); ledgerErr != nil {
			return ledgerErr
		}
		return err
	}
	if err := ensureWorktreeClean(cfg.RepoRoot); err != nil {
		if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "run_failed",
			Status: "worktree_dirty",
			Detail: err.Error(),
		}); ledgerErr != nil {
			return ledgerErr
		}
		return err
	}

	return nil
}

func promoteWorkerCommit(ctx context.Context, cfg Config, runner Runner, runID string, workerID int, task string, workerBranch string, commitSha string) error {
	err := PromoteWorker(ctx, PromoteOptions{
		Runner:       runner,
		RepoRoot:     cfg.RepoRoot,
		WorkerBranch: workerBranch,
		WorkerCommit: commitSha,
	})
	if err != nil {
		if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_promotion_failed",
			Worker: workerID,
			Task:   task,
			Branch: workerBranch,
			Commit: commitSha,
			Status: "promotion_failed",
			Detail: err.Error(),
		}); ledgerErr != nil {
			return ledgerErr
		}
		return err
	}

	return appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "worker_promoted",
		Worker: workerID,
		Task:   task,
		Branch: workerBranch,
		Commit: commitSha,
		Status: "promoted",
	})
}

func removeCleanWorkerWorktree(repoRoot, worktreePath string) {
	_ = gitRemoveWorkerWorktree(repoRoot, worktreePath)
}

func ensureNoMergeConflicts(repoRoot string) error {
	if repoRoot == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("check git repository: %w", err)
	}

	cmd := exec.Command("git", "-C", repoRoot, "diff", "--name-only", "--diff-filter=U")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("check git merge conflicts: %w", err)
	}
	if conflicts := strings.TrimSpace(string(output)); conflicts != "" {
		return fmt.Errorf("repository has unresolved merge conflicts:\n%s", conflicts)
	}

	return nil
}

func ensureWorktreeClean(repoRoot string) error {
	if repoRoot == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("check git repository: %w", err)
	}

	cmd := exec.Command("git", "-C", repoRoot, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("check git worktree status: %w", err)
	}
	if dirty := strings.TrimSpace(string(output)); dirty != "" {
		return fmt.Errorf("repository has uncommitted changes:\n%s", dirty)
	}

	return nil
}

func ensureCurrentBranch(repoRoot string, expected string) error {
	if expected == "" {
		return nil
	}
	current, err := gitCurrentBranch(repoRoot)
	if err != nil {
		return err
	}
	if current != expected {
		return fmt.Errorf("worker branch changed: current %s, want %s", current, expected)
	}
	return nil
}

func ensureChangedPathsWithinWriteScope(repoRoot string, baseCommit string, headCommit string, candidate Candidate) error {
	paths, err := gitChangedPaths(repoRoot, baseCommit, headCommit)
	if err != nil {
		return err
	}

	var violations []string
	for _, path := range paths {
		if !candidateAllowsPath(candidate, path) {
			violations = append(violations, path)
		}
	}
	if len(violations) == 0 {
		return nil
	}

	allowed := strings.Join(candidate.WriteScope, ", ")
	if strings.TrimSpace(allowed) == "" {
		allowed = "(none declared)"
	}
	return fmt.Errorf("worker changed paths outside declared write scope: %s (allowed: %s)", strings.Join(violations, ", "), allowed)
}

func candidateAllowsPath(candidate Candidate, path string) bool {
	path = cleanRepoPath(path)
	if path == "" {
		return false
	}
	for _, scope := range candidate.WriteScope {
		scope = cleanRepoPath(scope)
		if scope == "" {
			continue
		}
		if path == scope || strings.HasPrefix(path, scope+"/") {
			return true
		}
	}
	return false
}

func cleanRepoPath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	for strings.HasPrefix(path, "./") {
		path = strings.TrimPrefix(path, "./")
	}
	path = strings.Trim(path, "/")
	if path == "." {
		return ""
	}
	return path
}

func candidateTaskName(candidate Candidate) string {
	return candidate.PhaseID + "/" + candidate.SubphaseID + "/" + candidate.ItemName
}

// selectAcrossSubphases caps the candidate list at maxAgents but prefers
// distributing the first slots across distinct subphases. With MaxAgents==1
// or fewer candidates than slots the behaviour matches a plain prefix slice.
func selectAcrossSubphases(candidates []Candidate, maxAgents int) []Candidate {
	if maxAgents <= 0 || maxAgents >= len(candidates) {
		return candidates
	}
	if maxAgents == 1 {
		return candidates[:1]
	}

	picked := make([]Candidate, 0, maxAgents)
	pickedIdx := make(map[int]struct{}, maxAgents)
	seenSubphase := make(map[string]struct{}, maxAgents)
	for i, candidate := range candidates {
		if len(picked) >= maxAgents {
			break
		}
		key := strings.ToLower(strings.TrimSpace(candidate.SubphaseID))
		if _, ok := seenSubphase[key]; ok && key != "" {
			continue
		}
		seenSubphase[key] = struct{}{}
		picked = append(picked, candidate)
		pickedIdx[i] = struct{}{}
	}
	for i, candidate := range candidates {
		if len(picked) >= maxAgents {
			break
		}
		if _, ok := pickedIdx[i]; ok {
			continue
		}
		picked = append(picked, candidate)
	}

	return picked
}

func BuildWorkerPrompt(candidate Candidate) string {
	return BuildWorkerPromptWithBranch(candidate, "")
}

func BuildWorkerPromptWithBranch(candidate Candidate, branch string) string {
	var prompt strings.Builder

	prompt.WriteString("Mission:\n")
	prompt.WriteString("Complete the selected Gormes progress task with strict Test-Driven Development (TDD).\n\n")

	prompt.WriteString("Selected task:\n")
	fmt.Fprintf(&prompt, "- Phase/Subphase/Item: %s / %s / %s\n", candidate.PhaseID, candidate.SubphaseID, candidate.ItemName)
	fmt.Fprintf(&prompt, "- Current status: %s\n", valueOrDash(candidate.Status))
	fmt.Fprintf(&prompt, "- Priority: %s\n", valueOrDash(candidate.Priority))
	fmt.Fprintf(&prompt, "- Execution owner: %s\n", valueOrDash(candidate.ExecutionOwner))
	fmt.Fprintf(&prompt, "- Slice size: %s\n", valueOrDash(candidate.SliceSize))
	fmt.Fprintf(&prompt, "- Selection reason: %s\n\n", valueOrDash(candidate.SelectionReason()))

	prompt.WriteString("Execution contract:\n")
	fmt.Fprintf(&prompt, "- Contract: %s\n", valueOrDash(candidate.Contract))
	fmt.Fprintf(&prompt, "- Contract status: %s\n", valueOrDash(candidate.ContractStatus))
	fmt.Fprintf(&prompt, "- Fixture: %s\n", valueOrDash(candidate.Fixture))
	fmt.Fprintf(&prompt, "- Degraded mode: %s\n", valueOrDash(candidate.DegradedMode))
	prompt.WriteString("- Trust class:\n")
	writePromptList(&prompt, candidate.TrustClass)
	prompt.WriteString("\n")

	prompt.WriteString("Readiness:\n")
	prompt.WriteString("- Ready when:\n")
	writePromptList(&prompt, candidate.ReadyWhen)
	prompt.WriteString("- Not ready when:\n")
	writePromptList(&prompt, candidate.NotReadyWhen)
	prompt.WriteString("- Blocked by:\n")
	writePromptList(&prompt, candidate.BlockedBy)
	prompt.WriteString("- Unblocks:\n")
	writePromptList(&prompt, candidate.Unblocks)
	prompt.WriteString("\n")

	prompt.WriteString("Worker boundaries:\n")
	prompt.WriteString("- Allowed write scope:\n")
	writePromptList(&prompt, candidate.WriteScope)
	prompt.WriteString("- Required test commands:\n")
	writePromptList(&prompt, candidate.TestCommands)
	prompt.WriteString("\n")

	prompt.WriteString("Completion evidence:\n")
	prompt.WriteString("- Done signal:\n")
	writePromptList(&prompt, candidate.DoneSignal)
	prompt.WriteString("- Acceptance:\n")
	writePromptList(&prompt, candidate.Acceptance)
	prompt.WriteString("\n")

	prompt.WriteString("Source references:\n")
	writePromptList(&prompt, candidate.SourceRefs)
	prompt.WriteString("\n")

	fmt.Fprintf(&prompt, "Note: %s\n\n", valueOrDash(candidate.Note))

	prompt.WriteString("Requirements:\n")
	prompt.WriteString("- Read the repository context before editing.\n")
	prompt.WriteString("- Keep changes scoped to the selected task and its allowed write scope.\n")
	prompt.WriteString("- Run the required test commands before reporting completion.\n")
	prompt.WriteString("- Report against the done signal and acceptance criteria.\n")

	if strings.TrimSpace(branch) != "" {
		prompt.WriteString("\nWorker branch:\n")
		fmt.Fprintf(&prompt, "- You are isolated on git branch %s.\n", branch)
		prompt.WriteString("- After tests pass, stage and commit your changes on this branch (`git add -A && git commit -m \"...\"`).\n")
		prompt.WriteString("- Leave the working tree clean. Autoloop refuses dirty worker output so scope leaks are visible instead of silently promoted.\n")
		prompt.WriteString("- Do not switch branches, rebase, or push yourself.\n")
	}

	return prompt.String()
}

func writePromptList(prompt *strings.Builder, values []string) {
	if len(values) == 0 {
		prompt.WriteString("- (none declared)\n")
		return
	}

	for _, value := range values {
		fmt.Fprintf(prompt, "- %s\n", value)
	}
}

func valueOrDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}

	return value
}

func backendRunError(name string, result Result) error {
	output := strings.TrimSpace(result.Stderr)
	if output == "" {
		output = strings.TrimSpace(result.Stdout)
	}
	if output == "" {
		return fmt.Errorf("%s failed: %w", name, result.Err)
	}

	return fmt.Errorf("%s failed: %w: %s", name, result.Err, output)
}
