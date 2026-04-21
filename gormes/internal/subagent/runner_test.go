package subagent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

func TestChatRunner_StopFinishReturnsSummary(t *testing.T) {
	cli := hermes.NewMockClient()
	cli.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "child ", TokensOut: 1},
		{Kind: hermes.EventToken, Token: "done", TokensOut: 2},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 7, TokensOut: 2},
	}, "sess-child")

	reg := tools.NewRegistry()
	runner := NewChatRunner(cli, reg, ChatRunnerConfig{Model: "hermes-agent", MaxToolDuration: 2 * time.Second})

	var events []Event
	res, err := runner.Run(context.Background(), Spec{Goal: "summarize repo", MaxIterations: 4}, func(ev Event) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", res.Status)
	}
	if res.Summary != "child done" {
		t.Fatalf("Summary = %q, want child done", res.Summary)
	}
	if len(events) == 0 || events[0].Type != EventStarted {
		t.Fatalf("first event = %+v, want started", events)
	}
}

func TestChatRunner_ToolCallExecutesAllowedTool(t *testing.T) {
	cli := hermes.NewMockClient()
	cli.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "thinking aloud ", TokensOut: 2},
		{Kind: hermes.EventDone, FinishReason: "tool_calls", ToolCalls: []hermes.ToolCall{
			{ID: "call-1", Name: "echo", Arguments: json.RawMessage(`{"text":"hello"}`)},
		}},
	}, "sess-child")
	cli.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "tool ok", TokensOut: 2},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 11, TokensOut: 2},
	}, "sess-child")

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.MockTool{
		NameStr: "echo",
		ExecuteFn: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"text":"hello"}`), nil
		},
	})

	runner := NewChatRunner(cli, reg, ChatRunnerConfig{Model: "hermes-agent", MaxToolDuration: 2 * time.Second})
	res, err := runner.Run(context.Background(), Spec{
		Goal:          "call echo",
		MaxIterations: 4,
		AllowedTools:  []string{"echo"},
	}, func(Event) {})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0] != "echo" {
		t.Fatalf("ToolCalls = %v, want [echo]", res.ToolCalls)
	}
	if res.Summary != "tool ok" {
		t.Fatalf("Summary = %q, want tool ok", res.Summary)
	}
	if len(cli.Requests()) != 2 {
		t.Fatalf("OpenStream requests = %d, want 2", len(cli.Requests()))
	}
}

func TestChatRunner_ToolPanicReturnsErrorInsteadOfCrashing(t *testing.T) {
	cli := hermes.NewMockClient()
	cli.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "tool_calls", ToolCalls: []hermes.ToolCall{
			{ID: "call-1", Name: "explode", Arguments: json.RawMessage(`{}`)},
		}},
	}, "sess-child")
	cli.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "recovered", TokensOut: 2},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 9, TokensOut: 2},
	}, "sess-child")

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.MockTool{
		NameStr: "explode",
		ExecuteFn: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			panic("boom")
		},
	})

	runner := NewChatRunner(cli, reg, ChatRunnerConfig{Model: "hermes-agent", MaxToolDuration: 2 * time.Second})
	res, err := runner.Run(context.Background(), Spec{
		Goal:          "panic containment",
		MaxIterations: 4,
		AllowedTools:  []string{"explode"},
	}, func(Event) {})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", res.Status)
	}
	if len(cli.Requests()) != 2 {
		t.Fatalf("OpenStream requests = %d, want 2", len(cli.Requests()))
	}
}

func TestChatRunner_BlockedToolReturnsPolicyError(t *testing.T) {
	cli := hermes.NewMockClient()
	cli.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "tool_calls", ToolCalls: []hermes.ToolCall{
			{ID: "call-1", Name: "delegate_task", Arguments: json.RawMessage(`{"goal":"nested"}`)},
		}},
	}, "sess-child")
	cli.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "nested blocked", TokensOut: 2},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 9, TokensOut: 2},
	}, "sess-child")

	reg := tools.NewRegistry()
	runner := NewChatRunner(cli, reg, ChatRunnerConfig{Model: "hermes-agent", MaxToolDuration: 2 * time.Second})

	res, err := runner.Run(context.Background(), Spec{Goal: "try nested delegation", MaxIterations: 4}, func(Event) {})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", res.Status)
	}
	if len(cli.Requests()) != 2 {
		t.Fatalf("OpenStream requests = %d, want 2", len(cli.Requests()))
	}
}

func TestChatRunner_EOFWithoutDoneReturnsBufferedSummary(t *testing.T) {
	cli := hermes.NewMockClient()
	cli.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "partial ", TokensOut: 1},
		{Kind: hermes.EventToken, Token: "answer", TokensOut: 2},
	}, "sess-child")

	reg := tools.NewRegistry()
	runner := NewChatRunner(cli, reg, ChatRunnerConfig{Model: "hermes-agent", MaxToolDuration: 2 * time.Second})

	res, err := runner.Run(context.Background(), Spec{Goal: "finish via eof", MaxIterations: 2}, func(Event) {})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", res.Status)
	}
	if res.Summary != "partial answer" {
		t.Fatalf("Summary = %q, want partial answer", res.Summary)
	}
	if res.FinishReason != "" {
		t.Fatalf("FinishReason = %q, want empty", res.FinishReason)
	}
}
