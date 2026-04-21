// gormes/internal/subagent/runner.go
package subagent

import (
	"context"
	"time"
)

// Runner is the swappable inner loop of a subagent. This slice ships
// StubRunner; 2.E.7 will ship LLMRunner.
//
// Contracts (binding on every implementation):
//
//  1. Run MUST return promptly after ctx.Done() fires. "Promptly" means within
//     a small bounded time, not blocked forever.
//  2. Run MUST NOT close the events channel. The manager owns the channel
//     lifecycle.
//  3. Run MAY emit zero or more events.
//  4. Run MUST return a non-nil *SubagentResult.
type Runner interface {
	Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult
}

// StubRunner emits started → completed and returns immediately. ExitReason
// carries an explicit TODO marker so it is unmistakable in logs and tests.
type StubRunner struct{}

func (StubRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	start := time.Now()

	select {
	case events <- SubagentEvent{Type: EventStarted, Message: cfg.Goal}:
	case <-ctx.Done():
		return &SubagentResult{
			Status:     StatusInterrupted,
			ExitReason: "ctx_cancelled_before_start",
			Duration:   time.Since(start),
			Error:      ctx.Err().Error(),
		}
	}

	select {
	case events <- SubagentEvent{Type: EventCompleted, Message: "stub"}:
	case <-ctx.Done():
		return &SubagentResult{
			Status:     StatusInterrupted,
			ExitReason: "ctx_cancelled_during_stub",
			Duration:   time.Since(start),
			Error:      ctx.Err().Error(),
		}
	}

	return &SubagentResult{
		Status:     StatusCompleted,
		Summary:    cfg.Goal,
		ExitReason: "stub_runner_no_llm_yet",
		Duration:   time.Since(start),
		Iterations: 0,
	}
}
