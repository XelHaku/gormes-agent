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
	candidates, err := NormalizeCandidates(opts.Config.ProgressJSON, CandidateOptions{ActiveFirst: true})
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
	return fmt.Sprintf(`Mission:
Complete the selected Gormes progress task with strict Test-Driven Development (TDD).

Selected task:
- Phase/Subphase/Item: %s / %s / %s
- Current status: %s

Requirements:
- Read the repository context before editing.
- Keep changes scoped to the selected task.
- Run the relevant tests before reporting completion.
`, candidate.PhaseID, candidate.SubphaseID, candidate.ItemName, candidate.Status)
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
