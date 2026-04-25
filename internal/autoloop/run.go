package autoloop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

	for i, candidate := range selected {
		workerID := i + 1
		task := candidateTaskName(candidate)
		var workerBranch string
		var workerBaseCommit string
		if hasGit {
			workerBranch = WorkerBranchName(runID, workerID, candidate)
		}
		if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_claimed",
			Worker: workerID,
			Task:   task,
			Branch: workerBranch,
			Status: "claimed",
		}); err != nil {
			return RunSummary{}, err
		}

		if hasGit {
			if err := gitCreateWorkerBranch(opts.Config.RepoRoot, workerBranch); err != nil {
				_ = appendRunLedgerEvent(opts.Config, LedgerEvent{
					TS:     time.Now().UTC(),
					RunID:  runID,
					Event:  "worker_failed",
					Worker: workerID,
					Task:   task,
					Branch: workerBranch,
					Status: "branch_create_failed",
					Detail: err.Error(),
				})
				return RunSummary{}, err
			}
			workerBaseCommit, err = gitHeadSha(opts.Config.RepoRoot)
			if err != nil {
				return RunSummary{}, err
			}
		}

		args := append([]string(nil), argv[1:]...)
		args = append(args, BuildWorkerPromptWithBranch(candidate, workerBranch))
		result := runner.Run(ctx, Command{
			Name: argv[0],
			Args: args,
			Dir:  opts.Config.RepoRoot,
		})
		if result.Err != nil {
			if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "worker_failed",
				Worker: workerID,
				Task:   task,
				Branch: workerBranch,
				Status: "backend_failed",
			}); err != nil {
				return RunSummary{}, err
			}
			if hasGit {
				restoreBranchIfClean(opts.Config.RepoRoot, baseBranch)
			}
			return RunSummary{}, backendRunError(argv[0], result)
		}
		if err := ensureNoMergeConflicts(opts.Config.RepoRoot); err != nil {
			if ledgerErr := appendRunLedgerEvent(opts.Config, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "worker_failed",
				Worker: workerID,
				Task:   task,
				Branch: workerBranch,
				Status: "worktree_unmerged",
				Detail: err.Error(),
			}); ledgerErr != nil {
				return RunSummary{}, ledgerErr
			}
			if hasGit {
				restoreBranchIfClean(opts.Config.RepoRoot, baseBranch)
			}
			return RunSummary{}, err
		}
		if err := ensureWorktreeClean(opts.Config.RepoRoot); err != nil {
			if ledgerErr := appendRunLedgerEvent(opts.Config, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  runID,
				Event:  "worker_failed",
				Worker: workerID,
				Task:   task,
				Branch: workerBranch,
				Status: "worktree_dirty",
				Detail: err.Error(),
			}); ledgerErr != nil {
				return RunSummary{}, ledgerErr
			}
			return RunSummary{}, err
		}
		var commitSha string
		if hasGit {
			commitSha, err = gitHeadSha(opts.Config.RepoRoot)
			if err != nil {
				return RunSummary{}, err
			}
			if commitSha != workerBaseCommit {
				if err := gitRestoreBranch(opts.Config.RepoRoot, baseBranch); err != nil {
					return RunSummary{}, err
				}
				if err := promoteWorkerCommit(ctx, opts.Config, runner, runID, workerID, task, workerBranch, commitSha); err != nil {
					return RunSummary{}, err
				}
			} else {
				commitSha = ""
				if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
					TS:     time.Now().UTC(),
					RunID:  runID,
					Event:  "worker_no_changes",
					Worker: workerID,
					Task:   task,
					Branch: workerBranch,
					Status: "no_changes",
				}); err != nil {
					return RunSummary{}, err
				}
				if err := gitRestoreBranch(opts.Config.RepoRoot, baseBranch); err != nil {
					return RunSummary{}, err
				}
			}
		}

		if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  runID,
			Event:  "worker_success",
			Worker: workerID,
			Task:   task,
			Branch: workerBranch,
			Commit: commitSha,
			Status: "success",
		}); err != nil {
			return RunSummary{}, err
		}
	}
	if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  runID,
		Event:  "run_completed",
		Status: "completed",
	}); err != nil {
		return RunSummary{}, err
	}

	return summary, nil
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
