package autoloop

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type RunOptions struct {
	Config Config
	Runner Runner
	DryRun bool
}

type RunSummary struct {
	Candidates int
	Selected   []Candidate
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

	selected := candidates
	if opts.Config.MaxAgents > 0 && opts.Config.MaxAgents < len(selected) {
		selected = selected[:opts.Config.MaxAgents]
	}

	summary := RunSummary{
		Candidates: len(candidates),
		Selected:   append([]Candidate(nil), selected...),
	}
	if opts.DryRun {
		return summary, nil
	}
	if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
		TS:     time.Now().UTC(),
		Event:  "run_started",
		Status: "started",
	}); err != nil {
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

	for i, candidate := range selected {
		workerID := i + 1
		task := candidateTaskName(candidate)
		if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			Event:  "worker_claimed",
			Worker: workerID,
			Task:   task,
			Status: "claimed",
		}); err != nil {
			return RunSummary{}, err
		}

		args := append([]string(nil), argv[1:]...)
		args = append(args, BuildWorkerPrompt(candidate))
		result := runner.Run(ctx, Command{
			Name: argv[0],
			Args: args,
			Dir:  opts.Config.RepoRoot,
		})
		if result.Err != nil {
			if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
				TS:     time.Now().UTC(),
				Event:  "worker_failed",
				Worker: workerID,
				Task:   task,
				Status: "backend_failed",
			}); err != nil {
				return RunSummary{}, err
			}
			return RunSummary{}, backendRunError(argv[0], result)
		}
		if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
			TS:     time.Now().UTC(),
			Event:  "worker_success",
			Worker: workerID,
			Task:   task,
			Status: "success",
		}); err != nil {
			return RunSummary{}, err
		}
	}
	if err := appendRunLedgerEvent(opts.Config, LedgerEvent{
		TS:     time.Now().UTC(),
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

func candidateTaskName(candidate Candidate) string {
	return candidate.PhaseID + "/" + candidate.SubphaseID + "/" + candidate.ItemName
}

func BuildWorkerPrompt(candidate Candidate) string {
	var b strings.Builder
	fmt.Fprintln(&b, "Mission:")
	fmt.Fprintln(&b, "Complete the selected Gormes progress task with strict Test-Driven Development (TDD).")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Selected task:")
	fmt.Fprintf(&b, "- Phase/Subphase/Item: %s / %s / %s\n", candidate.PhaseID, candidate.SubphaseID, candidate.ItemName)
	fmt.Fprintf(&b, "- Current status: %s\n", valueOrDash(candidate.Status))
	fmt.Fprintf(&b, "- Priority: %s\n", valueOrDash(candidate.Priority))
	fmt.Fprintf(&b, "- Execution owner: %s\n", valueOrDash(candidate.ExecutionOwner))
	fmt.Fprintf(&b, "- Slice size: %s\n", valueOrDash(candidate.SliceSize))
	fmt.Fprintf(&b, "- Selection reason: %s\n", candidate.SelectionReason())
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Execution contract:")
	fmt.Fprintf(&b, "- Contract: %s\n", valueOrDash(candidate.Contract))
	fmt.Fprintf(&b, "- Contract status: %s\n", valueOrDash(candidate.ContractStatus))
	fmt.Fprintf(&b, "- Fixture: %s\n", valueOrDash(candidate.Fixture))
	fmt.Fprintf(&b, "- Degraded mode: %s\n", valueOrDash(candidate.DegradedMode))
	fmt.Fprintln(&b, "- Trust class:")
	writePromptList(&b, candidate.TrustClass)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Readiness:")
	fmt.Fprintln(&b, "- Ready when:")
	writePromptList(&b, candidate.ReadyWhen)
	fmt.Fprintln(&b, "- Not ready when:")
	writePromptList(&b, candidate.NotReadyWhen)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Worker boundaries:")
	fmt.Fprintln(&b, "- Allowed write scope:")
	writePromptList(&b, candidate.WriteScope)
	fmt.Fprintln(&b, "- Required test commands:")
	writePromptList(&b, candidate.TestCommands)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Completion evidence:")
	fmt.Fprintln(&b, "- Done signal:")
	writePromptList(&b, candidate.DoneSignal)
	fmt.Fprintln(&b, "- Acceptance:")
	writePromptList(&b, candidate.Acceptance)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Source references:")
	writePromptList(&b, candidate.SourceRefs)
	if candidate.Note != "" {
		fmt.Fprintf(&b, "\nNote: %s\n", candidate.Note)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Requirements:")
	fmt.Fprintln(&b, "- Read the repository context before editing.")
	fmt.Fprintln(&b, "- Keep changes scoped to the selected task and its allowed write scope.")
	fmt.Fprintln(&b, "- Run the required test commands before reporting completion.")
	fmt.Fprintln(&b, "- Report against the done signal and acceptance criteria.")
	return b.String()
}

func writePromptList(b *strings.Builder, values []string) {
	if len(values) == 0 {
		fmt.Fprintln(b, "- (none declared)")
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func valueOrDash(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}
	return trimmed
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
