package builderloop

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plannertriggers"
	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// ErrPostPromotionVerifyFailed marks a RunOnce error as caused by the
// post-promotion verify gate failing (and exhausting any configured repair
// attempts). cmd/ binaries use errors.Is against this sentinel to map the
// outcome to a distinct exit code so systemd / CI can react differently
// from a generic internal error.
var ErrPostPromotionVerifyFailed = errors.New("post-promotion verify failed")

// ErrControlPlaneRunInProgress marks a builder run blocked by another
// control-plane loop holding the shared planner-loop run lock. Builder
// mutates the same checkout/progress surfaces as planner during claims,
// promotions, and health writes, so non-dry-run builder cycles must not
// overlap planner regeneration.
var ErrControlPlaneRunInProgress = errors.New("control-plane run already in progress")

type RunOptions struct {
	Config Config
	Runner Runner
	DryRun bool
	Now    time.Time
}

type RunSummary struct {
	Candidates           int
	Selected             []Candidate
	RunID                string
	MaxPhaseFiltered     int
	NextFilteredMaxPhase int
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
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := runIDFromTime(now)
	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	if !opts.DryRun {
		controlLock, err := acquireControlPlaneRunLock(opts.Config, now)
		if err != nil {
			_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "run_blocked",
				Status: "control_plane_locked",
				Detail: err.Error(),
			})
			return RunSummary{}, err
		}
		defer controlLock.release()

		if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "run_started",
			Status: "started",
		}); err != nil {
			return RunSummary{}, err
		}
		if opts.Config.AutoCommitDirtyWorktree {
			if err := checkpointDirtyWorktree(ctx, opts.Config, runID); err != nil {
				return RunSummary{}, err
			}
		}
		if err := preflightCleanWorktree(opts.Config, runID); err != nil {
			return RunSummary{}, err
		}
		if opts.Config.MergeOpenPullRequests {
			if _, err := MergeOpenPullRequests(ctx, PullRequestIntakeOptions{
				Runner:         runner,
				RepoRoot:       opts.Config.RepoRoot,
				RunRoot:        opts.Config.RunRoot,
				RunID:          runID,
				ConflictAction: opts.Config.PRConflictAction,
			}); err != nil {
				return RunSummary{}, err
			}
		}
	}

	candidates, err := NormalizeCandidates(opts.Config.ProgressJSON, CandidateOptions{
		ActiveFirst:        true,
		PriorityBoost:      opts.Config.PriorityBoost,
		MaxPhase:           opts.Config.MaxPhase,
		IncludeQuarantined: opts.Config.IncludeQuarantined,
		IncludeNeedsHuman:  opts.Config.IncludeNeedsHuman,
	})
	if err != nil {
		return RunSummary{}, err
	}

	selected := selectAcrossSubphases(candidates, opts.Config.MaxAgents)
	candidateCount := len(candidates)

	// Speculative execution: if enabled and slots remain, select candidates
	// whose blocked_by isn't complete but ready_when is satisfied.
	if opts.Config.SpeculativeExecutionEnabled && len(selected) < opts.Config.MaxAgents {
		remainingSlots := opts.Config.MaxAgents - len(selected)
		maxSpeculative := opts.Config.MaxSpeculativeWorkers
		if maxSpeculative > remainingSlots {
			maxSpeculative = remainingSlots
		}

		speculativePool, err := NormalizeCandidates(opts.Config.ProgressJSON, CandidateOptions{
			ActiveFirst:        true,
			PriorityBoost:      opts.Config.PriorityBoost,
			MaxPhase:           opts.Config.MaxPhase,
			IncludeBlocked:     true,
			IncludeQuarantined: opts.Config.IncludeQuarantined,
			IncludeNeedsHuman:  opts.Config.IncludeNeedsHuman,
		})
		if err != nil {
			return RunSummary{}, err
		}
		candidateCount = len(speculativePool)

		progressDoc, err := progress.Load(opts.Config.ProgressJSON)
		if err != nil {
			return RunSummary{}, err
		}
		completed := completedItemSetFromProgress(progressDoc)

		speculative := selectSpeculativeCandidates(speculativePool, completed, func(c Candidate) bool {
			// ready_when satisfied if no not_ready_when conditions exist
			// and ready_when conditions are implied by being a candidate
			return len(c.NotReadyWhen) == 0
		}, maxSpeculative)

		if len(speculative) > 0 {
			// Enrich with spec hashes for staleness detection
			speculative = enrichCandidatesWithSpecHash(speculative, func(c Candidate) string {
				return itemSpecHashFromProgress(progressDoc, c)
			})
			selected = append(selected, speculative...)
		}
	}

	summary := RunSummary{
		Candidates: candidateCount,
		Selected:   append([]Candidate(nil), selected...),
		RunID:      runID,
	}
	if opts.Config.MaxPhase > 0 {
		count, next, err := maxPhaseFilteredSummary(opts.Config.ProgressJSON, CandidateOptions{
			ActiveFirst:        true,
			PriorityBoost:      opts.Config.PriorityBoost,
			IncludeQuarantined: opts.Config.IncludeQuarantined,
			IncludeNeedsHuman:  opts.Config.IncludeNeedsHuman,
		}, opts.Config.MaxPhase)
		if err != nil {
			return RunSummary{}, err
		}
		summary.MaxPhaseFiltered = count
		summary.NextFilteredMaxPhase = next
	}
	if opts.DryRun {
		return summary, nil
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
	// A flush failure is fatal to the run: the operator must see the error so
	// they can investigate why the on-disk progress.json could not be updated
	// after worker outcomes were promoted. The failure is also recorded in the
	// run ledger so post-mortem tooling can correlate it with the run id.
	flushHealth := func() error {
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

		triggerEvents, err := acc.FlushWithTriggers(opts.Config.ProgressJSON, hashOf)
		if err != nil {
			_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "health_update_failed",
				Status: "failed",
				Detail: err.Error(),
			})
			return fmt.Errorf("flush health: %w", err)
		}
		if err := commitRunHealth(ctx, opts.Config, runner); err != nil {
			_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "health_update_failed",
				Status: "failed",
				Detail: err.Error(),
			})
			return fmt.Errorf("commit health: %w", err)
		}
		_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "health_updated",
			Status: "ok",
		})

		// Emit planner trigger events. Soft-fail: a missing or unwritable
		// triggers ledger must not break a successful autoloop run; the
		// planner will still pick up state changes on its periodic timer.
		emitPlannerTriggers(opts.Config.PlannerTriggersPath, triggerEvents)
		return nil
	}

	// observeOutcome feeds the degrader and emits backend_degraded ledger
	// events on switch. Called sequentially after each finishWorker so it is
	// safe with no synchronization.
	completedWork := false
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

	completeRun := func() error {
		if err := runPostPromotionGate(ctx, opts.Config, runner, runID, completedWork); err != nil {
			return err
		}
		if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "run_completed",
			Status: "completed",
		}); err != nil {
			return errors.Join(err, flushHealth())
		}
		return flushHealth()
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
		runBackendWorkers(ctx, opts.Config, runner, argv, workers)
		for _, worker := range workers {
			finishErr := finishWorker(ctx, opts.Config, runner, argv[0], runID, baseBranch, hasGit, worker)
			recordWorkerOutcome(acc, observeOutcome, degrader.Current(), worker, finishErr)
			if finishErr != nil {
				return RunSummary{}, errors.Join(finishErr, flushHealth())
			}
			completedWork = true
		}
		if err := completeRun(); err != nil {
			return summary, err
		}

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
			TS:              time.Now().UTC(),
			RunID:           runID,
			Event:           "worker_claimed",
			Worker:          worker.ID,
			Task:            worker.Task,
			Branch:          worker.Branch,
			Status:          "claimed",
			Speculative:     candidate.Speculative,
			SpecHashAtClaim: candidate.SpecHashAtClaim,
		}); err != nil {
			return RunSummary{}, errors.Join(err, flushHealth())
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
		worker.Result = runBackendCommand(ctx, opts.Config, runner, Command{
			Name: argv[0],
			Args: args,
			Dir:  worker.RepoRoot,
		})
		finishErr := finishWorker(ctx, opts.Config, runner, argv[0], runID, baseBranch, hasGit, worker)
		recordWorkerOutcome(acc, observeOutcome, degrader.Current(), worker, finishErr)
		if finishErr != nil {
			return RunSummary{}, errors.Join(finishErr, flushHealth())
		}
		completedWork = true
	}
	if err := completeRun(); err != nil {
		return summary, err
	}

	return summary, nil
}

type controlPlaneRunLock struct {
	file *os.File
}

func acquireControlPlaneRunLock(cfg Config, now time.Time) (*controlPlaneRunLock, error) {
	lockPath := controlPlaneRunLockPath(cfg)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: %s", ErrControlPlaneRunInProgress, lockPath)
		}
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_ = file.Truncate(0)
	_, _ = file.WriteString(fmt.Sprintf("holder=builder-loop\npid=%d\nstarted_utc=%s\n", os.Getpid(), now.UTC().Format(time.RFC3339)))
	return &controlPlaneRunLock{file: file}, nil
}

func controlPlaneRunLockPath(cfg Config) string {
	if strings.TrimSpace(cfg.PlannerTriggersPath) != "" {
		return filepath.Join(filepath.Dir(cfg.PlannerTriggersPath), "run.lock")
	}
	if strings.TrimSpace(cfg.RunRoot) != "" {
		return filepath.Join(filepath.Dir(filepath.Clean(cfg.RunRoot)), "planner-loop", "run.lock")
	}
	if strings.TrimSpace(cfg.RepoRoot) != "" {
		return filepath.Join(cfg.RepoRoot, ".codex", "planner-loop", "run.lock")
	}
	return filepath.Join(".codex", "planner-loop", "run.lock")
}

func (lock *controlPlaneRunLock) release() {
	if lock == nil || lock.file == nil {
		return
	}
	_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	_ = lock.file.Close()
}

func runIDFromTime(t time.Time) string {
	t = t.UTC()
	runID := t.Format("20060102T150405Z")
	if t.Nanosecond() == 0 {
		return runID
	}
	return fmt.Sprintf("%s-%09d", runID, t.Nanosecond())
}

func maxPhaseFilteredSummary(path string, opts CandidateOptions, maxPhase int) (count int, nextMaxPhase int, err error) {
	if maxPhase < 1 {
		return 0, 0, nil
	}
	opts.MaxPhase = 0
	candidates, err := NormalizeCandidates(path, opts)
	if err != nil {
		return 0, 0, err
	}
	for _, candidate := range candidates {
		phase, ok := phaseNumber(candidate.PhaseID)
		if !ok || phase <= maxPhase {
			continue
		}
		count++
		if nextMaxPhase == 0 || phase < nextMaxPhase {
			nextMaxPhase = phase
		}
	}
	return count, nextMaxPhase, nil
}

func runPostPromotionGate(ctx context.Context, cfg Config, runner Runner, runID string, promotedWork bool) error {
	if !promotedWork || len(cfg.PostPromotionVerifyCommands) == 0 {
		return nil
	}

	verifyErr := runPostPromotionVerification(ctx, cfg, runner, runID, 1)
	if verifyErr == nil {
		return nil
	}

	attempts := cfg.PostPromotionRepairAttempts
	if !cfg.PostPromotionRepairEnabled || attempts <= 0 {
		_ = appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "run_failed",
			Status: "post_promotion_verify_failed",
			Detail: truncateLedgerDetail(verifyErr.Error()),
		})
		return errors.Join(ErrPostPromotionVerifyFailed, verifyErr)
	}

	lastErr := verifyErr
	for attempt := 1; attempt <= attempts; attempt++ {
		if repairErr := runPostPromotionRepair(ctx, cfg, runner, runID, attempt, lastErr); repairErr != nil {
			_ = appendRunLedgerEvent(cfg, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "run_failed",
				Status: "post_promotion_repair_failed",
				Detail: truncateLedgerDetail(repairErr.Error()),
			})
			return errors.Join(lastErr, repairErr)
		}
		lastErr = runPostPromotionVerification(ctx, cfg, runner, runID, attempt+1)
		if lastErr == nil {
			return nil
		}
	}

	_ = appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "run_failed",
		Status: "post_promotion_verify_failed",
		Detail: truncateLedgerDetail(lastErr.Error()),
	})
	return errors.Join(ErrPostPromotionVerifyFailed, lastErr)
}

// runPrePromotionGate orchestrates the pre-promotion verify-repair-verify
// loop. Empty PrePromotionVerifyCommands → no-op. On verify failure: when
// PrePromotionRepairEnabled and PrePromotionRepairAttempts > 0, dispatch the
// LLM-driven repair against the worker's worktree, then re-verify. Loop up
// to PrePromotionRepairAttempts times. If the gate ultimately fails, the
// final ledger event is a worker_failed/pre_promotion_verify_failed (or
// pre_promotion_repair_failed if repair itself errored), main is never
// touched, and the error propagates up so finishWorker bails before
// promoteWorkerCommit.
func runPrePromotionGate(ctx context.Context, cfg Config, runner Runner, runID string, worker workerRun) error {
	if len(cfg.PrePromotionVerifyCommands) == 0 {
		return nil
	}

	verifyErr := runPrePromotionVerify(ctx, cfg, runner, runID, worker, 1)
	if verifyErr == nil {
		return nil
	}

	attempts := cfg.PrePromotionRepairAttempts
	if !cfg.PrePromotionRepairEnabled || attempts <= 0 {
		// Repair disabled — the verify_failed event was already emitted by
		// runPrePromotionVerify itself, so just propagate the error.
		return verifyErr
	}

	lastErr := verifyErr
	for attempt := 1; attempt <= attempts; attempt++ {
		if repairErr := runPrePromotionRepair(ctx, cfg, runner, runID, attempt, worker, lastErr); repairErr != nil {
			return errors.Join(lastErr, repairErr)
		}
		lastErr = runPrePromotionVerify(ctx, cfg, runner, runID, worker, attempt+1)
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

// runPrePromotionRepair dispatches the LLM backend against the worker's
// worktree with a prompt that names the failing verify commands and the
// captured error. The repair runs ON the worker's branch, so any commits
// the LLM produces accumulate on the worker branch (not main). After the
// backend exits, the worktree must be clean — uncommitted changes or merge
// conflicts mark the repair as failed.
func runPrePromotionRepair(ctx context.Context, cfg Config, runner Runner, runID string, attempt int, worker workerRun, cause error) error {
	if err := appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "pre_promotion_repair_started",
		Worker: worker.ID,
		Task:   worker.Task,
		Branch: worker.Branch,
		Status: "started",
		Detail: fmt.Sprintf("attempt=%d", attempt),
	}); err != nil {
		return err
	}

	argv, err := BuildBackendCommand(cfg.Backend, cfg.Mode)
	if err != nil {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "pre_promotion_repair_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "failed",
			Detail: truncateLedgerDetail(err.Error()),
		})
		return errors.Join(err, ledgerErr)
	}

	args := append([]string(nil), argv[1:]...)
	args = append(args, BuildPrePromotionRepairPrompt(cfg.PrePromotionVerifyCommands, worker, cause))
	result := runBackendCommand(ctx, cfg, runner, Command{
		Name: argv[0],
		Args: args,
		Dir:  worker.RepoRoot,
		Env:  postPromotionCommandEnv(cfg),
	})
	if result.Err != nil {
		err := postPromotionCommandError("pre-promotion repair", argv[0], result)
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "pre_promotion_repair_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d: %s", attempt, commandFailureDetail(result))),
		})
		return errors.Join(err, ledgerErr)
	}
	if err := ensureNoMergeConflicts(worker.RepoRoot); err != nil {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "pre_promotion_repair_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d clean-check: %s", attempt, err.Error())),
		})
		return errors.Join(err, ledgerErr)
	}
	if err := ensureWorktreeClean(worker.RepoRoot); err != nil {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "pre_promotion_repair_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d clean-check: %s", attempt, err.Error())),
		})
		return errors.Join(err, ledgerErr)
	}

	return appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "pre_promotion_repair_succeeded",
		Worker: worker.ID,
		Task:   worker.Task,
		Branch: worker.Branch,
		Status: "ok",
		Detail: fmt.Sprintf("attempt=%d", attempt),
	})
}

// runPrePromotionVerify runs cfg.PrePromotionVerifyCommands inside the
// worker's worktree before the worker's commit is cherry-picked onto main.
// Empty command list is a no-op (default — preserves the post-promotion-only
// behavior). On failure, emits a worker_failed ledger event with status
// pre_promotion_verify_failed and returns the error so finishWorker bails
// before promoteWorkerCommit runs. Main is therefore never touched when a
// worker's branch fails its own verify gate.
//
// All commands run regardless of which one fails first, mirroring the
// post-promotion gate's behavior so the operator (and any future repair
// hook) sees a complete failure picture in one ledger entry.
func runPrePromotionVerify(ctx context.Context, cfg Config, runner Runner, runID string, worker workerRun, attempt int) error {
	if len(cfg.PrePromotionVerifyCommands) == 0 {
		return nil
	}
	if err := appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "pre_promotion_verify_started",
		Worker: worker.ID,
		Task:   worker.Task,
		Branch: worker.Branch,
		Status: "started",
		Detail: fmt.Sprintf("attempt=%d commands=%d", attempt, len(cfg.PrePromotionVerifyCommands)),
	}); err != nil {
		return err
	}

	var (
		commandErrs    []error
		failureDetails []string
	)
	for i, shellCommand := range cfg.PrePromotionVerifyCommands {
		result := runner.Run(ctx, Command{
			Name: "sh",
			Args: []string{"-lc", shellCommand},
			Dir:  worker.RepoRoot,
			Env:  postPromotionCommandEnv(cfg),
		})
		if result.Err == nil {
			continue
		}
		commandErrs = append(commandErrs, postPromotionCommandError("pre-promotion verification", shellCommand, result))
		failureDetails = append(failureDetails,
			fmt.Sprintf("command=%d/%d %q:\n%s", i+1, len(cfg.PrePromotionVerifyCommands), shellCommand, commandFailureDetail(result)),
		)
	}
	if len(commandErrs) > 0 {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "pre_promotion_verify_failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d failed=%d/%d\n\n%s",
				attempt, len(commandErrs), len(cfg.PrePromotionVerifyCommands),
				strings.Join(failureDetails, "\n\n---\n\n"))),
		})
		return errors.Join(append(commandErrs, ledgerErr)...)
	}

	return appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "pre_promotion_verify_succeeded",
		Worker: worker.ID,
		Task:   worker.Task,
		Branch: worker.Branch,
		Status: "ok",
		Detail: fmt.Sprintf("attempt=%d commands=%d", attempt, len(cfg.PrePromotionVerifyCommands)),
	})
}

func runRowEvaluator(ctx context.Context, cfg Config, runner Runner, runID string, worker workerRun) error {
	commands := append([]string(nil), worker.Candidate.TestCommands...)
	if len(commands) == 0 {
		return nil
	}

	if err := appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "row_evaluation_started",
		Worker: worker.ID,
		Task:   worker.Task,
		Branch: worker.Branch,
		Status: "started",
		Detail: fmt.Sprintf("commands=%d", len(commands)),
	}); err != nil {
		return err
	}

	var (
		commandErrs    []error
		failureDetails []string
	)
	for i, shellCommand := range commands {
		result := runner.Run(ctx, Command{
			Name: "sh",
			Args: []string{"-lc", shellCommand},
			Dir:  worker.RepoRoot,
			Env:  postPromotionCommandEnv(cfg),
		})
		if result.Err == nil {
			continue
		}
		commandErrs = append(commandErrs, fmt.Errorf("command %d failed: %w", i+1, result.Err))
		failureDetails = append(failureDetails,
			fmt.Sprintf("command=%d/%d %q:\n%s", i+1, len(commands), shellCommand, commandFailureDetail(result)),
		)
	}
	if len(commandErrs) > 0 {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: "row_evaluation_failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("failed=%d/%d\n\n%s",
				len(commandErrs), len(commands),
				strings.Join(failureDetails, "\n\n---\n\n"))),
		})
		return errors.Join(append(commandErrs, ledgerErr)...)
	}

	return appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "row_evaluation_succeeded",
		Worker: worker.ID,
		Task:   worker.Task,
		Branch: worker.Branch,
		Status: "ok",
		Detail: fmt.Sprintf("commands=%d", len(commands)),
	})
}

func runPostPromotionVerification(ctx context.Context, cfg Config, runner Runner, runID string, attempt int) error {
	if err := appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "post_promotion_verify_started",
		Status: "started",
		Detail: fmt.Sprintf("attempt=%d commands=%d", attempt, len(cfg.PostPromotionVerifyCommands)),
	}); err != nil {
		return err
	}

	// Run ALL verify commands and collect failures, even if an early one
	// fails. The repair agent benefits from a complete failure picture
	// (e.g. both `go test` and `go vet` errors) rather than discovering
	// one issue at a time across multiple repair cycles.
	var (
		commandErrs    []error
		failureDetails []string
	)
	for i, shellCommand := range cfg.PostPromotionVerifyCommands {
		result := runner.Run(ctx, Command{
			Name: "sh",
			Args: []string{"-lc", shellCommand},
			Dir:  cfg.RepoRoot,
			Env:  postPromotionCommandEnv(cfg),
		})
		if result.Err == nil {
			continue
		}
		commandErrs = append(commandErrs, postPromotionCommandError("verification", shellCommand, result))
		failureDetails = append(failureDetails,
			fmt.Sprintf("command=%d/%d %q:\n%s", i+1, len(cfg.PostPromotionVerifyCommands), shellCommand, commandFailureDetail(result)),
		)
	}
	if len(commandErrs) > 0 {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "post_promotion_verify_failed",
			Status: "failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d failed=%d/%d\n\n%s",
				attempt, len(commandErrs), len(cfg.PostPromotionVerifyCommands),
				strings.Join(failureDetails, "\n\n---\n\n"))),
		})
		return errors.Join(append(commandErrs, ledgerErr)...)
	}

	if err := ensureNoMergeConflicts(cfg.RepoRoot); err != nil {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "post_promotion_verify_failed",
			Status: "failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d clean-check: %s", attempt, err.Error())),
		})
		return errors.Join(err, ledgerErr)
	}
	if err := ensureWorktreeClean(cfg.RepoRoot); err != nil {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "post_promotion_verify_failed",
			Status: "failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d clean-check: %s", attempt, err.Error())),
		})
		return errors.Join(err, ledgerErr)
	}

	return appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "post_promotion_verify_succeeded",
		Status: "ok",
		Detail: fmt.Sprintf("attempt=%d commands=%d", attempt, len(cfg.PostPromotionVerifyCommands)),
	})
}

func runPostPromotionRepair(ctx context.Context, cfg Config, runner Runner, runID string, attempt int, cause error) error {
	if err := appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "post_promotion_repair_started",
		Status: "started",
		Detail: fmt.Sprintf("attempt=%d", attempt),
	}); err != nil {
		return err
	}

	argv, err := BuildBackendCommand(cfg.Backend, cfg.Mode)
	if err != nil {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "post_promotion_repair_failed",
			Status: "failed",
			Detail: truncateLedgerDetail(err.Error()),
		})
		return errors.Join(err, ledgerErr)
	}

	args := append([]string(nil), argv[1:]...)
	args = append(args, BuildPostPromotionRepairPrompt(cfg.PostPromotionVerifyCommands, cause))
	result := runBackendCommand(ctx, cfg, runner, Command{
		Name: argv[0],
		Args: args,
		Dir:  cfg.RepoRoot,
		Env:  postPromotionCommandEnv(cfg),
	})
	if result.Err != nil {
		err := postPromotionCommandError("repair", argv[0], result)
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "post_promotion_repair_failed",
			Status: "failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d: %s", attempt, commandFailureDetail(result))),
		})
		return errors.Join(err, ledgerErr)
	}
	if err := ensureNoMergeConflicts(cfg.RepoRoot); err != nil {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "post_promotion_repair_failed",
			Status: "failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d clean-check: %s", attempt, err.Error())),
		})
		return errors.Join(err, ledgerErr)
	}
	if err := ensureWorktreeClean(cfg.RepoRoot); err != nil {
		ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "post_promotion_repair_failed",
			Status: "failed",
			Detail: truncateLedgerDetail(fmt.Sprintf("attempt=%d clean-check: %s", attempt, err.Error())),
		})
		return errors.Join(err, ledgerErr)
	}

	return appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "post_promotion_repair_succeeded",
		Status: "ok",
		Detail: fmt.Sprintf("attempt=%d", attempt),
	})
}

func postPromotionCommandError(kind, command string, result Result) error {
	output := commandFailureDetail(result)
	if output == "" {
		return fmt.Errorf("post-promotion %s command %q failed: %w", kind, command, result.Err)
	}
	return fmt.Errorf("post-promotion %s command %q failed: %w: %s", kind, command, result.Err, output)
}

func commandFailureDetail(result Result) string {
	var parts []string
	if result.Err != nil {
		parts = append(parts, result.Err.Error())
	}
	output := strings.TrimSpace(result.Stderr)
	if output == "" {
		output = strings.TrimSpace(result.Stdout)
	}
	if output != "" {
		parts = append(parts, output)
	}
	return strings.Join(parts, ": ")
}

func postPromotionCommandEnv(cfg Config) []string {
	env := []string{
		"PROGRESS_JSON=" + cfg.ProgressJSON,
		"RUN_ROOT=" + cfg.RunRoot,
		"BACKEND=" + cfg.Backend,
		"MODE=" + cfg.Mode,
		fmt.Sprintf("MAX_AGENTS=%d", cfg.MaxAgents),
		fmt.Sprintf("MAX_PHASE=%d", cfg.MaxPhase),
	}
	if len(cfg.PriorityBoost) > 0 {
		env = append(env, "PRIORITY_BOOST="+strings.Join(cfg.PriorityBoost, ","))
	}
	return env
}

func truncateLedgerDetail(value string) string {
	value = strings.TrimSpace(value)
	const maxDetail = 2000
	if len(value) <= maxDetail {
		return value
	}
	// Preserve the TAIL of the value: the actual failure summary in
	// `go test ./...` output (--- FAIL, FAIL package, exit status) and the
	// last test that broke live near the end of stdout, while the prefix is
	// long lists of passing packages. A head-truncated detail strips
	// exactly the diagnostic information and leaves the operator staring
	// at "ok    package/foo 0.04s ... ok    package/bar 0.05s ...".
	//
	// Truncate at a line boundary inside the tail so we don't cut a log
	// line in half. Keep a small head context too (so the operator can see
	// the run signature) separated by an explicit elision marker.
	const headBudget = 200
	tailBudget := maxDetail - headBudget - len("...[N bytes elided]...\n\n")
	if tailBudget <= 0 {
		return "..." + value[len(value)-maxDetail:]
	}
	head := value[:headBudget]
	tail := value[len(value)-tailBudget:]
	if idx := strings.IndexByte(tail, '\n'); idx >= 0 && idx < len(tail)-1 {
		// Trim the partial first line of the tail so logs read cleanly.
		tail = tail[idx+1:]
	}
	elided := len(value) - headBudget - len(tail)
	return fmt.Sprintf("%s\n...[%d bytes elided]...\n\n%s", head, elided, tail)
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

func runBackendWorkers(ctx context.Context, cfg Config, runner Runner, argv []string, workers []workerRun) {
	var wg sync.WaitGroup
	for i := range workers {
		worker := &workers[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			args := append([]string(nil), argv[1:]...)
			args = append(args, BuildWorkerPromptWithBranch(worker.Candidate, worker.Branch))
			worker.Result = runBackendCommand(ctx, cfg, runner, Command{
				Name: argv[0],
				Args: args,
				Dir:  worker.RepoRoot,
			})
		}()
	}
	wg.Wait()
}

func runBackendCommand(ctx context.Context, cfg Config, runner Runner, command Command) Result {
	backendCtx := ctx
	cancel := func() {}
	if cfg.BackendTimeout > 0 {
		backendCtx, cancel = context.WithTimeout(ctx, cfg.BackendTimeout)
	}
	result := runner.Run(backendCtx, command)
	cancel()
	return result
}

func finishWorker(ctx context.Context, cfg Config, runner Runner, backendName string, runID string, baseBranch string, hasGit bool, worker workerRun) error {
	if worker.Result.Err != nil {
		failure := ClassifyBackendFailure(worker.Result.Err, worker.Result.Stdout, worker.Result.Stderr)
		if err := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_failed",
			Worker: worker.ID,
			Task:   worker.Task,
			Branch: worker.Branch,
			Status: failure.Status,
			Detail: truncateLedgerDetail(failure.Detail),
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
			if err := ensureWorkerCommitWithinWriteScope(cfg, runID, worker, commitSha); err != nil {
				return err
			}
			if err := ensureCurrentBranch(cfg.RepoRoot, baseBranch); err != nil {
				return err
			}
			// Speculative execution verification: before promoting a speculative
			// worker, verify that (1) the spec hasn't changed since claim, and
			// (2) all blocked_by dependencies completed successfully.
			if worker.Candidate.Speculative {
				if err := verifySpeculativeWorker(ctx, cfg, worker, runID); err != nil {
					if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
						TS:     time.Now().UTC(),
						RunID:  runID,
						Event:  "worker_failed",
						Worker: worker.ID,
						Task:   worker.Task,
						Branch: worker.Branch,
						Commit: commitSha,
						Status: "speculative_verify_failed",
						Detail: err.Error(),
					}); ledgerErr != nil {
						return ledgerErr
					}
					return err
				}
			}
			// Pre-promotion gate: verify the worker's worktree, optionally
			// repair on failure, re-verify, and only promote if the gate
			// passes. A failing gate aborts here as a worker_failed
			// outcome so main never enters a briefly-broken state. Empty
			// PrePromotionVerifyCommands is a no-op (current behavior).
			if err := runPrePromotionGate(ctx, cfg, runner, runID, worker); err != nil {
				return err
			}
			commitSha, err = gitHeadSha(worker.RepoRoot)
			if err != nil {
				return err
			}
			if commitSha != worker.BaseCommit {
				if err := ensureWorkerCommitWithinWriteScope(cfg, runID, worker, commitSha); err != nil {
					return err
				}
				if err := runRowEvaluator(ctx, cfg, runner, runID, worker); err != nil {
					return err
				}
				if err := promoteWorkerCommit(ctx, cfg, runner, runID, worker.ID, worker.Task, worker.Branch, commitSha); err != nil {
					return err
				}
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

func ensureWorkerCommitWithinWriteScope(cfg Config, runID string, worker workerRun, commitSha string) error {
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
	return nil
}

// verifySpeculativeWorker checks that a speculative worker can be promoted:
// (1) the spec hash hasn't changed since claim, and (2) all blocked_by
// dependencies completed successfully. Returns an error if either check fails.
func verifySpeculativeWorker(ctx context.Context, cfg Config, worker workerRun, runID string) error {
	c := worker.Candidate

	// Check 1: Spec hash unchanged
	if c.SpecHashAtClaim != "" {
		currentHash := func() string {
			prog, err := progress.Load(cfg.ProgressJSON)
			if err != nil {
				return ""
			}
			phase, ok := prog.Phases[c.PhaseID]
			if !ok {
				return ""
			}
			sub, ok := phase.Subphases[c.SubphaseID]
			if !ok {
				return ""
			}
			for i := range sub.Items {
				if sub.Items[i].Name == c.ItemName {
					return progress.ItemSpecHash(&sub.Items[i])
				}
			}
			return ""
		}()

		if currentHash != c.SpecHashAtClaim {
			return fmt.Errorf("spec changed since claim: was %s, now %s", c.SpecHashAtClaim, currentHash)
		}
	}

	// Check 2: All blocked_by dependencies completed successfully
	if len(c.BlockedByPending) > 0 {
		// Reload progress to check current status of blockers
		prog, err := progress.Load(cfg.ProgressJSON)
		if err != nil {
			return fmt.Errorf("cannot load progress to verify blockers: %w", err)
		}

		// Build completed set
		completed := make(map[string]struct{})
		for phaseID, phase := range prog.Phases {
			for subID, sub := range phase.Subphases {
				for i := range sub.Items {
					if sub.Items[i].Status == progress.StatusComplete {
						for _, key := range []string{
							strings.ToLower(sub.Items[i].Name),
							strings.ToLower(subID + "/" + sub.Items[i].Name),
							strings.ToLower(phaseID + "/" + subID + "/" + sub.Items[i].Name),
						} {
							completed[key] = struct{}{}
						}
					}
				}
			}
		}

		var stillPending []string
		for _, blocker := range c.BlockedByPending {
			if _, ok := completed[blocker]; !ok {
				stillPending = append(stillPending, blocker)
			}
		}

		if len(stillPending) > 0 {
			return fmt.Errorf("blocked_by dependencies still pending: %v", stillPending)
		}
	}

	return nil
}

func completedItemSetFromProgress(progressDoc *progress.Progress) map[string]struct{} {
	completed := make(map[string]struct{})
	if progressDoc == nil {
		return completed
	}
	for phaseID, phase := range progressDoc.Phases {
		for subphaseID, subphase := range phase.Subphases {
			for i := range subphase.Items {
				if subphase.Items[i].Status != progress.StatusComplete {
					continue
				}
				for _, key := range blockerKeys(phaseID, subphaseID, subphase.Items[i].Name) {
					completed[key] = struct{}{}
				}
			}
		}
	}
	return completed
}

func itemSpecHashFromProgress(progressDoc *progress.Progress, candidate Candidate) string {
	if progressDoc == nil {
		return ""
	}
	phase, ok := progressDoc.Phases[candidate.PhaseID]
	if !ok {
		return ""
	}
	subphase, ok := phase.Subphases[candidate.SubphaseID]
	if !ok {
		return ""
	}
	for i := range subphase.Items {
		if subphase.Items[i].Name == candidate.ItemName {
			return progress.ItemSpecHash(&subphase.Items[i])
		}
	}
	return ""
}

// emitPlannerTriggers writes one plannertriggers ledger entry per
// FlushedTriggerEvent. An empty path or empty event slice is a no-op.
// Each append is independent: if one entry fails to land, the rest still
// get a chance, and failures are logged rather than returned because the
// trigger ledger is a best-effort signal channel — the planner's own
// timer is the durable fallback.
func emitPlannerTriggers(path string, events []FlushedTriggerEvent) {
	if path == "" || len(events) == 0 {
		return
	}
	for _, ev := range events {
		entry := plannertriggers.TriggerEvent{
			Source:        "builder-loop",
			Kind:          ev.Kind,
			PhaseID:       ev.PhaseID,
			SubphaseID:    ev.SubphaseID,
			ItemName:      ev.ItemName,
			Reason:        ev.Reason,
			AutoloopRunID: ev.AutoloopRunID,
		}
		if err := plannertriggers.AppendTriggerEvent(path, entry); err != nil {
			log.Printf("builder-loop: append planner trigger failed: %v", err)
		}
	}
}

func appendRunLedgerEvent(cfg Config, event LedgerEvent) error {
	if cfg.RunRoot == "" {
		return nil
	}
	return AppendLedgerEvent(filepath.Join(cfg.RunRoot, "state", "runs.jsonl"), event)
}

func checkpointDirtyWorktree(ctx context.Context, cfg Config, runID string) error {
	if cfg.RepoRoot == "" || !repoHasGit(cfg.RepoRoot) {
		return nil
	}
	if err := ensureNoMergeConflicts(cfg.RepoRoot); err != nil {
		return recordCheckpointFailure(cfg, runID, "worktree_unmerged", err)
	}

	status, err := runGitCheckpointCommand(ctx, cfg.RepoRoot, "status", "--porcelain")
	if err != nil {
		return recordCheckpointFailure(cfg, runID, "status_failed", err)
	}
	dirty := strings.TrimSpace(status)
	if dirty == "" {
		return nil
	}

	if err := appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "worktree_checkpoint_started",
		Status: "started",
		Detail: summarizeDirtyStatus(dirty),
	}); err != nil {
		return err
	}
	if _, err := runGitCheckpointCommand(ctx, cfg.RepoRoot, "add", "-A"); err != nil {
		return recordCheckpointFailure(cfg, runID, "stage_failed", err)
	}

	message := fmt.Sprintf("builder-loop: checkpoint dirty worktree %s", runID)
	if _, err := runGitCheckpointCommand(ctx, cfg.RepoRoot,
		"-c", "user.name=Gormes Builder Loop",
		"-c", "user.email=builder-loop@gormes.local",
		"-c", "commit.gpgsign=false",
		"commit", "-m", message,
	); err != nil {
		return recordCheckpointFailure(cfg, runID, "commit_failed", err)
	}

	sha, err := gitHeadSha(cfg.RepoRoot)
	if err != nil {
		return recordCheckpointFailure(cfg, runID, "commit_sha_failed", err)
	}
	return appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "worktree_checkpoint_committed",
		Status: "committed",
		Commit: sha,
	})
}

func recordCheckpointFailure(cfg Config, runID string, status string, err error) error {
	if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "worktree_checkpoint_failed",
		Status: status,
		Detail: err.Error(),
	}); ledgerErr != nil {
		return ledgerErr
	}
	return err
}

func runGitCheckpointCommand(ctx context.Context, repoRoot string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", repoRoot}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, trimmed)
	}
	return trimmed, nil
}

func summarizeDirtyStatus(status string) string {
	lines := strings.Split(strings.TrimSpace(status), "\n")
	const maxLines = 20
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], fmt.Sprintf("... %d more paths", len(lines)-maxLines))
	}
	out := strings.Join(lines, "\n")
	const maxChars = 2000
	if len(out) > maxChars {
		return out[:maxChars] + "\n... truncated"
	}
	return out
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
	if err := ensureUpstreamNotBehind(cfg.RepoRoot); err != nil {
		if ledgerErr := appendRunLedgerEvent(cfg, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "run_failed",
			Status: "branch_behind_upstream",
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

func commitRunHealth(ctx context.Context, cfg Config, runner Runner) error {
	if cfg.RepoRoot == "" || cfg.ProgressJSON == "" || !repoHasGit(cfg.RepoRoot) {
		return nil
	}
	rel, err := filepath.Rel(cfg.RepoRoot, cfg.ProgressJSON)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil
	}

	status := runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"status", "--short", "--", rel},
		Dir:  cfg.RepoRoot,
	})
	if status.Err != nil {
		return fmt.Errorf("check progress health status: %w", status.Err)
	}
	if strings.TrimSpace(status.Stdout) == "" {
		return nil
	}

	add := runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"add", "--", rel},
		Dir:  cfg.RepoRoot,
	})
	if add.Err != nil {
		return fmt.Errorf("stage progress health: %w", add.Err)
	}

	commit := runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"commit", "-m", "builder-loop: record run health", "--", rel},
		Dir:  cfg.RepoRoot,
	})
	if commit.Err != nil {
		return fmt.Errorf("commit progress health: %w", commit.Err)
	}
	return nil
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

// BuildPrePromotionRepairPrompt produces the LLM prompt the pre-promotion
// repair gate hands to the backend when verify fails. Critically, the
// repair runs ON THE WORKER'S BRANCH inside the worker's worktree, so any
// commits the LLM produces accumulate on that branch (they will be
// cherry-picked onto main only if the post-repair re-verify passes).
//
// The prompt names the failing commands and the truncated cause, and tells
// the LLM to scope its edits to the worker's task — not to widen scope to
// unrelated parts of the tree.
func BuildPrePromotionRepairPrompt(verifyCommands []string, worker workerRun, cause error) string {
	var prompt strings.Builder

	prompt.WriteString("Mission:\n")
	prompt.WriteString("Repair this autoloop worker's branch in its worktree so the pre-promotion verify gate passes BEFORE the commit is cherry-picked onto main.\n\n")

	prompt.WriteString("Context:\n")
	fmt.Fprintf(&prompt, "- You are working in the worker's isolated worktree (branch %q, task %q).\n", worker.Branch, worker.Task)
	prompt.WriteString("- The worker's commit is NOT yet on main. Failed verifies here keep main clean; only after this gate passes does promote run.\n")
	prompt.WriteString("- Keep edits minimal and tightly scoped to making the failing commands pass. Do not widen scope to unrelated phases.\n")
	prompt.WriteString("- The worker has already produced one commit on this branch; your repair commits stack on top of it.\n\n")

	prompt.WriteString("Failing verification:\n")
	if cause == nil {
		prompt.WriteString("- (no error detail available)\n\n")
	} else {
		fmt.Fprintf(&prompt, "- %s\n\n", truncateLedgerDetail(cause.Error()))
	}

	prompt.WriteString("Verify commands to restore:\n")
	writePromptList(&prompt, verifyCommands)
	prompt.WriteString("\n")

	prompt.WriteString("Requirements:\n")
	prompt.WriteString("- Inspect the failure before editing.\n")
	prompt.WriteString("- Fix code, tests, docs, or progress metadata required for the failing verify commands to pass.\n")
	prompt.WriteString("- Run the verify commands above after repair to confirm they pass locally.\n")
	prompt.WriteString("- Stage and commit repair changes on this branch with a clear message before exiting.\n")
	prompt.WriteString("- Leave the worktree with no uncommitted changes or unresolved merge conflicts.\n")

	return prompt.String()
}

func BuildPostPromotionRepairPrompt(verifyCommands []string, cause error) string {
	var prompt strings.Builder

	prompt.WriteString("Mission:\n")
	prompt.WriteString("Repair the integrated Gormes control checkout after promoted autoloop worker changes failed the mandatory post-promotion verification gate.\n\n")

	prompt.WriteString("Context:\n")
	prompt.WriteString("- You are repairing the already-promoted integration state, not selecting new roadmap work.\n")
	prompt.WriteString("- Keep edits minimal and directly tied to the failing verification output.\n")
	prompt.WriteString("- The final run health must not be recorded until this full-suite gate passes.\n\n")

	prompt.WriteString("Failing verification:\n")
	if cause == nil {
		prompt.WriteString("- (no error detail available)\n\n")
	} else {
		fmt.Fprintf(&prompt, "- %s\n\n", truncateLedgerDetail(cause.Error()))
	}

	prompt.WriteString("Full-suite commands to restore:\n")
	writePromptList(&prompt, verifyCommands)
	prompt.WriteString("\n")

	prompt.WriteString("Requirements:\n")
	prompt.WriteString("- Inspect the failure before editing.\n")
	prompt.WriteString("- Fix code, tests, docs, or progress metadata needed for the promoted integration to pass.\n")
	prompt.WriteString("- Run the full-suite commands above after repair.\n")
	prompt.WriteString("- Stage and commit repair changes with a clear message before exiting.\n")
	prompt.WriteString("- Leave the repository with no uncommitted changes or unresolved merge conflicts.\n")

	return prompt.String()
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
	failure := ClassifyBackendFailure(result.Err, result.Stdout, result.Stderr)
	if failure.Detail == "" {
		return fmt.Errorf("%s failed: %w", name, result.Err)
	}

	return fmt.Errorf("%s failed: %w: %s", name, result.Err, failure.Detail)
}
