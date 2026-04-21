// gormes/internal/subagent/runner_test.go
package subagent

import (
	"context"
	"testing"
	"time"
)

func TestStubRunnerHappyPath(t *testing.T) {
	cfg := SubagentConfig{Goal: "do the thing"}
	events := make(chan SubagentEvent, 4)
	runner := StubRunner{}

	result := runner.Run(context.Background(), cfg, events)
	close(events)

	if result == nil {
		t.Fatal("Run returned nil result")
	}
	if result.Status != StatusCompleted {
		t.Errorf("Status: want %q, got %q", StatusCompleted, result.Status)
	}
	if result.Summary != "do the thing" {
		t.Errorf("Summary: want %q, got %q", "do the thing", result.Summary)
	}
	if result.ExitReason != "stub_runner_no_llm_yet" {
		t.Errorf("ExitReason: want %q, got %q", "stub_runner_no_llm_yet", result.ExitReason)
	}

	got := drain(events)
	if len(got) != 2 {
		t.Fatalf("event count: want 2, got %d (%v)", len(got), got)
	}
	if got[0].Type != EventStarted || got[1].Type != EventCompleted {
		t.Errorf("event sequence: want started→completed, got %v→%v", got[0].Type, got[1].Type)
	}
}

func TestStubRunnerCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Unbuffered channel with no reader — first send would block. The runner
	// must observe ctx.Done() instead and return promptly.
	events := make(chan SubagentEvent)
	runner := StubRunner{}

	done := make(chan *SubagentResult, 1)
	go func() { done <- runner.Run(ctx, SubagentConfig{Goal: "x"}, events) }()

	select {
	case result := <-done:
		if result.Status != StatusInterrupted {
			t.Errorf("Status: want %q, got %q", StatusInterrupted, result.Status)
		}
		if result.ExitReason != "ctx_cancelled_before_start" {
			t.Errorf("ExitReason: want %q, got %q", "ctx_cancelled_before_start", result.ExitReason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StubRunner did not honour ctx cancellation within 2s")
	}
}

func TestStubRunnerCancelledDuringEmit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Buffered channel: first send (started) succeeds; reader never drains, so
	// second send (completed) blocks. Cancel after a moment to force the
	// "during" branch.
	events := make(chan SubagentEvent, 1)
	runner := StubRunner{}

	done := make(chan *SubagentResult, 1)
	go func() { done <- runner.Run(ctx, SubagentConfig{Goal: "x"}, events) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case result := <-done:
		if result.Status != StatusInterrupted {
			t.Errorf("Status: want %q, got %q", StatusInterrupted, result.Status)
		}
		if result.ExitReason != "ctx_cancelled_during_stub" {
			t.Errorf("ExitReason: want %q, got %q", "ctx_cancelled_during_stub", result.ExitReason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StubRunner did not honour ctx cancellation within 2s")
	}
}

func drain(ch <-chan SubagentEvent) []SubagentEvent {
	var out []SubagentEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}
