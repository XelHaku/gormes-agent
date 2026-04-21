// gormes/internal/subagent/types_test.go
package subagent

import (
	"testing"
	"time"
)

func TestSubagentConfigZeroValue(t *testing.T) {
	var cfg SubagentConfig
	if cfg.Goal != "" {
		t.Errorf("Goal: want empty, got %q", cfg.Goal)
	}
	if cfg.MaxIterations != 0 {
		t.Errorf("MaxIterations: want 0, got %d", cfg.MaxIterations)
	}
	if cfg.Timeout != 0 {
		t.Errorf("Timeout: want 0, got %v", cfg.Timeout)
	}
	if cfg.EnabledTools != nil {
		t.Errorf("EnabledTools: want nil, got %v", cfg.EnabledTools)
	}
}

func TestEventTypeStringValues(t *testing.T) {
	cases := map[EventType]string{
		EventStarted:     "started",
		EventProgress:    "progress",
		EventToolCall:    "tool_call",
		EventOutput:      "output",
		EventCompleted:   "completed",
		EventFailed:      "failed",
		EventInterrupted: "interrupted",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("EventType: want %q, got %q", want, string(got))
		}
	}
}

func TestResultStatusStringValues(t *testing.T) {
	cases := map[ResultStatus]string{
		StatusCompleted:   "completed",
		StatusFailed:      "failed",
		StatusInterrupted: "interrupted",
		StatusError:       "error",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("ResultStatus: want %q, got %q", want, string(got))
		}
	}
}

func TestSubagentResultZeroValue(t *testing.T) {
	var r SubagentResult
	if r.ID != "" || r.Status != "" || r.Duration != time.Duration(0) {
		t.Errorf("zero-value SubagentResult: unexpected fields populated: %+v", r)
	}
}
