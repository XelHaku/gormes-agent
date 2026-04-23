package telemetry

import (
	"testing"
	"time"
)

func TestSnapshotAfterTicks(t *testing.T) {
	tm := New()
	tm.StartTurn()
	tm.Tick(1)
	tm.Tick(3)
	tm.FinishTurn(12*time.Millisecond, TurnStatusCompleted)
	s := tm.Snapshot()
	if s.TokensOutTotal != 3 {
		t.Errorf("out_total = %d, want 3", s.TokensOutTotal)
	}
	if s.LatencyMsLast != 12 {
		t.Errorf("latency = %d, want 12", s.LatencyMsLast)
	}
}

func TestModelPlumbing(t *testing.T) {
	tm := New()
	tm.SetModel("hermes-agent")
	if got := tm.Snapshot().Model; got != "hermes-agent" {
		t.Errorf("Model = %q", got)
	}
}

func TestSetTokensIn(t *testing.T) {
	tm := New()
	tm.SetTokensIn(5)
	tm.SetTokensIn(3)
	if got := tm.Snapshot().TokensInTotal; got != 8 {
		t.Errorf("in_total = %d, want 8 (accumulated)", got)
	}
}

func TestSnapshotTracksTurnAndToolOutcomes(t *testing.T) {
	tm := New()
	tm.SetModel("gormes-agent")

	tm.StartTurn()
	tm.Tick(2)
	tm.RecordToolCall(ToolStatusCompleted)
	tm.RecordToolCall(ToolStatusFailed)
	tm.FinishTurn(12*time.Millisecond, TurnStatusCompleted)

	tm.StartTurn()
	tm.RecordToolCall(ToolStatusCancelled)
	tm.FinishTurn(5*time.Millisecond, TurnStatusCancelled)

	got := tm.Snapshot()
	if got.TurnsTotal != 2 {
		t.Fatalf("turns_total = %d, want 2", got.TurnsTotal)
	}
	if got.TurnsCompleted != 1 {
		t.Fatalf("turns_completed = %d, want 1", got.TurnsCompleted)
	}
	if got.TurnsCancelled != 1 {
		t.Fatalf("turns_cancelled = %d, want 1", got.TurnsCancelled)
	}
	if got.ToolCallsTotal != 3 {
		t.Fatalf("tool_calls_total = %d, want 3", got.ToolCallsTotal)
	}
	if got.ToolCallsFailed != 1 {
		t.Fatalf("tool_calls_failed = %d, want 1", got.ToolCallsFailed)
	}
	if got.ToolCallsCancelled != 1 {
		t.Fatalf("tool_calls_cancelled = %d, want 1", got.ToolCallsCancelled)
	}
	if got.LastTurnStatus != TurnStatusCancelled {
		t.Fatalf("last_turn_status = %q, want %q", got.LastTurnStatus, TurnStatusCancelled)
	}
}
