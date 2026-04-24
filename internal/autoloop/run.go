package autoloop

import (
	"context"
	"fmt"
	"strings"
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
		ActiveFirst: true,
		MaxPhase:    opts.Config.MaxPhase,
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

	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	argv, err := BuildBackendCommand(opts.Config.Backend, opts.Config.Mode)
	if err != nil {
		return RunSummary{}, err
	}

	for _, candidate := range selected {
		args := append([]string(nil), argv[1:]...)
		args = append(args, BuildWorkerPrompt(candidate))
		result := runner.Run(ctx, Command{
			Name: argv[0],
			Args: args,
			Dir:  opts.Config.RepoRoot,
		})
		if result.Err != nil {
			return RunSummary{}, backendRunError(argv[0], result)
		}
	}

	return summary, nil
}

func BuildWorkerPrompt(candidate Candidate) string {
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
	prompt.WriteString("- Keep changes scoped to the selected task.\n")
	prompt.WriteString("- Run the relevant tests before reporting completion.\n")

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
