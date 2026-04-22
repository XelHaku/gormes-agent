// gormes/internal/subagent/runner_test.go
package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
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

func TestStubRunnerRejectsBlockedEnabledTools(t *testing.T) {
	cfg := SubagentConfig{
		Goal:         "do the thing",
		EnabledTools: []string{"echo", "delegate_task"},
	}
	events := make(chan SubagentEvent, 4)
	runner := StubRunner{}

	result := runner.Run(context.Background(), cfg, events)
	close(events)

	if result == nil {
		t.Fatal("Run returned nil result")
	}
	if result.Status != StatusFailed {
		t.Fatalf("Status: want %q, got %q", StatusFailed, result.Status)
	}
	if result.ExitReason != "blocked_tool_request" {
		t.Fatalf("ExitReason: want %q, got %q", "blocked_tool_request", result.ExitReason)
	}
	if result.Error == "" {
		t.Fatal("Error: want non-empty blocked-tool message")
	}

	got := drain(events)
	if len(got) != 1 {
		t.Fatalf("event count: want 1, got %d (%v)", len(got), got)
	}
	if got[0].Type != EventFailed {
		t.Fatalf("event type: want %q, got %q", EventFailed, got[0].Type)
	}
}

func TestExecuteChildToolRejectsNonAllowlistedTool(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})

	cfg := SubagentConfig{
		Goal:         "run echo",
		EnabledTools: []string{"now"},
		toolExecutor: tools.NewInProcessToolExecutor(reg),
	}

	_, info, err := executeChildTool(context.Background(), cfg, make(chan SubagentEvent, 4), tools.ToolRequest{
		ToolName: "echo",
		Input:    json.RawMessage(`{"text":"hi"}`),
	})
	if err == nil {
		t.Fatal("executeChildTool() error = nil, want allowlist failure")
	}
	if !strings.Contains(err.Error(), "tool not allowlisted for child run") {
		t.Fatalf("error = %q, want allowlist message", err.Error())
	}
	if info.Status != "blocked" {
		t.Fatalf("ToolCallInfo.Status = %q, want %q", info.Status, "blocked")
	}
}

func TestExecuteChildToolRejectsBlockedToolAtExecutionTime(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.MockTool{NameStr: "delegate_task"})

	cfg := SubagentConfig{
		Goal:         "run blocked tool",
		EnabledTools: []string{"delegate_task"},
		toolExecutor: tools.NewInProcessToolExecutor(reg),
	}

	_, info, err := executeChildTool(context.Background(), cfg, make(chan SubagentEvent, 4), tools.ToolRequest{
		ToolName: "delegate_task",
		Input:    json.RawMessage(`{}`),
	})
	if err == nil {
		t.Fatal("executeChildTool() error = nil, want blocked-tool failure")
	}
	if !strings.Contains(err.Error(), "blocked tool for child run") {
		t.Fatalf("error = %q, want blocked-tool message", err.Error())
	}
	if info.Status != "blocked" {
		t.Fatalf("ToolCallInfo.Status = %q, want %q", info.Status, "blocked")
	}
}

func TestExecuteChildToolRunsAllowlistedTool(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})

	cfg := SubagentConfig{
		Goal:         "run echo",
		EnabledTools: []string{"echo"},
		toolExecutor: tools.NewInProcessToolExecutor(reg),
	}
	events := make(chan SubagentEvent, 8)

	out, info, err := executeChildTool(context.Background(), cfg, events, tools.ToolRequest{
		ToolName: "echo",
		Input:    json.RawMessage(`{"text":"hi"}`),
	})
	if err != nil {
		t.Fatalf("executeChildTool() error = %v", err)
	}
	if !strings.Contains(string(out), `"hi"`) {
		t.Fatalf("output = %s, want echo payload", out)
	}
	if info.Status != "completed" {
		t.Fatalf("ToolCallInfo.Status = %q, want %q", info.Status, "completed")
	}
	if info.Name != "echo" {
		t.Fatalf("ToolCallInfo.Name = %q, want %q", info.Name, "echo")
	}
}

func TestHermesRunnerStreamsToCompletion(t *testing.T) {
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "hello "},
		{Kind: hermes.EventToken, Token: "world"},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sid_child")

	runner := NewHermesRunner(mc, "test-model", nil)
	cfg := SubagentConfig{Goal: "say hi", MaxIterations: 3}
	events := make(chan SubagentEvent, 16)

	res := runner.Run(context.Background(), cfg, events)
	close(events)

	if res.Status != StatusCompleted {
		t.Fatalf("Status: want %q, got %q", StatusCompleted, res.Status)
	}
	if res.Summary != "hello world" {
		t.Fatalf("Summary: want %q, got %q", "hello world", res.Summary)
	}
	if res.ExitReason != "stop" {
		t.Fatalf("ExitReason: want %q, got %q", "stop", res.ExitReason)
	}
	if res.Iterations != 1 {
		t.Fatalf("Iterations: want 1, got %d", res.Iterations)
	}

	got := drain(events)
	if len(got) < 4 {
		t.Fatalf("event count: want >=4, got %d (%v)", len(got), got)
	}
	if got[0].Type != EventStarted {
		t.Fatalf("first event: want %q, got %q", EventStarted, got[0].Type)
	}
	if got[len(got)-1].Type != EventCompleted {
		t.Fatalf("last event: want %q, got %q", EventCompleted, got[len(got)-1].Type)
	}
}

func TestHermesRunnerExecutesToolCallsAndContinues(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})

	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "tool_calls", ToolCalls: []hermes.ToolCall{{
			ID:        "call_1",
			Name:      "echo",
			Arguments: json.RawMessage(`{"text":"hi from tool"}`),
		}}},
	}, "sid_first")
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "done"},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sid_second")

	runner := NewHermesRunner(mc, "test-model", []hermes.ToolDescriptor{{Name: "echo", Description: "echo", Schema: json.RawMessage(`{"type":"object"}`)}})
	cfg := SubagentConfig{
		Goal:          "use tool",
		MaxIterations: 4,
		EnabledTools:  []string{"echo"},
		toolExecutor:  tools.NewInProcessToolExecutor(reg),
	}
	events := make(chan SubagentEvent, 32)

	res := runner.Run(context.Background(), cfg, events)
	close(events)

	if res.Status != StatusCompleted {
		t.Fatalf("Status: want %q, got %q", StatusCompleted, res.Status)
	}
	if res.Iterations != 2 {
		t.Fatalf("Iterations: want %d, got %d", 2, res.Iterations)
	}
	if len(res.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len: want 1, got %d", len(res.ToolCalls))
	}
	if res.ToolCalls[0].Name != "echo" || res.ToolCalls[0].Status != "completed" {
		t.Fatalf("ToolCalls[0]: got %+v", res.ToolCalls[0])
	}

	reqs := mc.Requests()
	if len(reqs) != 2 {
		t.Fatalf("OpenStream requests: want 2, got %d", len(reqs))
	}
	foundToolMsg := false
	for _, m := range reqs[1].Messages {
		if m.Role == "tool" && m.Name == "echo" {
			foundToolMsg = true
			break
		}
	}
	if !foundToolMsg {
		t.Fatalf("second request missing tool role message: %#v", reqs[1].Messages)
	}
}

func TestHermesRunnerFailsOnStreamOpenError(t *testing.T) {
	runner := NewHermesRunner(erringClient{openErr: errors.New("dial failed")}, "test-model", nil)
	cfg := SubagentConfig{Goal: "x", MaxIterations: 2}
	events := make(chan SubagentEvent, 8)

	res := runner.Run(context.Background(), cfg, events)
	close(events)

	if res.Status != StatusFailed {
		t.Fatalf("Status: want %q, got %q", StatusFailed, res.Status)
	}
	if res.ExitReason != "child_stream_open_failed" {
		t.Fatalf("ExitReason: want %q, got %q", "child_stream_open_failed", res.ExitReason)
	}
	if !strings.Contains(res.Error, "dial failed") {
		t.Fatalf("Error: want dial failure, got %q", res.Error)
	}
}

type erringClient struct{ openErr error }

func (c erringClient) OpenStream(context.Context, hermes.ChatRequest) (hermes.Stream, error) {
	return nil, c.openErr
}
func (c erringClient) OpenRunEvents(context.Context, string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}
func (c erringClient) Health(context.Context) error { return nil }

func drain(ch <-chan SubagentEvent) []SubagentEvent {
	var out []SubagentEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}
