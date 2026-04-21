package subagent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

func TestDelegateTool_ExecuteReturnsChildResult(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	seenSpec := make(chan Spec, 1)

	mgr := NewManager(config.DelegationCfg{
		DefaultMaxIterations: 8,
		DefaultTimeout:       45 * time.Second,
		MaxChildDepth:        1,
	}, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		seenSpec <- spec
		close(started)
		<-release
		return Result{Status: StatusCompleted, Summary: "child summary"}, nil
	}), t.TempDir()+"/runs.jsonl")

	tool := NewDelegateTool(mgr)

	done := make(chan struct{})
	var out json.RawMessage
	var err error
	go func() {
		out, err = tool.Execute(context.Background(), json.RawMessage(`{
			"goal":"  investigate ",
			"context":"  scoped notes ",
			"model":"child-model",
			"max_iterations":3,
			"timeout_seconds":7,
			"allowed_tools":["echo","now"]
		}`))
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("child run did not start")
	}

	select {
	case spec := <-seenSpec:
		if spec.Goal != "investigate" {
			t.Fatalf("Goal = %q, want investigate", spec.Goal)
		}
		if spec.Context != "scoped notes" {
			t.Fatalf("Context = %q, want scoped notes", spec.Context)
		}
		if spec.Model != "child-model" {
			t.Fatalf("Model = %q, want child-model", spec.Model)
		}
		if spec.MaxIterations != 3 {
			t.Fatalf("MaxIterations = %d, want 3", spec.MaxIterations)
		}
		if spec.Timeout != 7*time.Second {
			t.Fatalf("Timeout = %v, want 7s", spec.Timeout)
		}
		if len(spec.AllowedTools) != 2 || spec.AllowedTools[0] != "echo" || spec.AllowedTools[1] != "now" {
			t.Fatalf("AllowedTools = %#v, want [echo now]", spec.AllowedTools)
		}
	default:
		t.Fatal("runner did not receive spec")
	}

	select {
	case <-done:
		t.Fatal("Execute returned before child run completed")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Execute did not return")
	}
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got struct {
		RunID   string `json:"run_id"`
		Status  string `json:"status"`
		Summary string `json:"summary,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got.RunID == "" {
		t.Fatal("RunID must be populated")
	}
	if got.Status != string(StatusCompleted) {
		t.Fatalf("Status = %q, want completed", got.Status)
	}
	if got.Summary != "child summary" {
		t.Fatalf("Summary = %q, want child summary", got.Summary)
	}
	if got.Error != "" {
		t.Fatalf("Error = %q, want empty", got.Error)
	}
}

func TestDelegateTool_TimeoutIsTwoMinutes(t *testing.T) {
	if got := NewDelegateTool(nil).Timeout(); got != 2*time.Minute {
		t.Fatalf("Timeout() = %v, want 2m", got)
	}
}

func TestDelegateTool_ExecuteReturnsErrorWhenWaitFails(t *testing.T) {
	mgr := NewManager(config.DelegationCfg{
		DefaultMaxIterations: 8,
		DefaultTimeout:       45 * time.Second,
		MaxChildDepth:        1,
	}, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		return Result{Status: StatusCompleted, Summary: "child summary"}, nil
	}), t.TempDir())

	tool := NewDelegateTool(mgr)

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":" investigate "}`))
	if err == nil {
		t.Fatal("Execute error = nil, want wait/log failure")
	}

	var got struct {
		RunID   string `json:"run_id"`
		Status  string `json:"status"`
		Summary string `json:"summary,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	if jerr := json.Unmarshal(out, &got); jerr != nil {
		t.Fatalf("unmarshal output: %v", jerr)
	}
	if got.RunID == "" {
		t.Fatal("RunID must be populated")
	}
	if got.Status != string(StatusCompleted) {
		t.Fatalf("Status = %q, want completed", got.Status)
	}
	if got.Error == "" {
		t.Fatal("Error field must be populated when wait fails")
	}
}
