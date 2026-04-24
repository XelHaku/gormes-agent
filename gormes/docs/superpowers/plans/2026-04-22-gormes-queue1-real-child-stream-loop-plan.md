# Queue 1 Real Child Hermes Stream Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the production `delegate_task` stub seam with a real Hermes-backed child stream loop that can complete normal child turns, execute child `tool_calls` through the existing allowlist/blocklist seam, and fail or stop deterministically.

**Architecture:** Keep `internal/subagent/manager.go` as the lifecycle owner and keep `executeChildTool()` as the only child-tool execution gate. Add a new `HermesRunner` that depends only on `internal/hermes.Client`, translates stream tokens into `SubagentEvent`s, loops on `finish_reason=="tool_calls"`, and returns a typed `SubagentResult`. Wire `buildDefaultRegistry()` to construct this runner from the existing Hermes endpoint/model config, while leaving `NewManager()` defaults on `StubRunner` so current isolated manager tests stay stable.

**Tech Stack:** Go stdlib (`context`, `errors`, `fmt`, `io`, `strings`, `time`), existing `internal/hermes`, existing `internal/subagent`, existing `internal/tools`, Cobra command bootstraps, progress/docs generator.

---

## File Map

- Create: `gormes/internal/subagent/hermes_runner.go`
  Child-specific LLM loop. Owns prompt assembly, stream consumption, `tool_calls` turn-chaining, and terminal result shaping.
- Create: `gormes/internal/subagent/hermes_runner_test.go`
  Deterministic tests for stop, cancel, `tool_calls`, and error paths using `hermes.MockClient` and small local fake streams.
- Modify: `gormes/internal/subagent/runner.go`
  Keep `Runner` / `StubRunner` / `executeChildTool()` in place and add any small shared helpers needed by both runners.
- Modify: `gormes/cmd/gormes/registry.go`
  Build the production `delegate_task` manager with a real `HermesRunner` factory and existing delegation config.
- Modify: `gormes/cmd/gormes/registry_test.go`
  Replace stub-only end-to-end assertions with a real SSE-backed registry test using `httptest`.
- Modify: `gormes/cmd/gormes/main.go`
- Modify: `gormes/cmd/gormes/gateway.go`
- Modify: `gormes/cmd/gormes/telegram.go`
- Modify: `gormes/cmd/gormes/doctor.go`
  Update `buildDefaultRegistry(...)` call sites to pass `config.HermesCfg`.
- Modify: `gormes/docs/content/building-gormes/architecture_plan/progress.json`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/why-go.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/_index.md`
- Modify: `README.md`
  Mark `2.E.1` complete and regenerate progress-driven rollups once the slice is green.

## Task 1: HermesRunner stop and cancellation core

**Files:**
- Create: `gormes/internal/subagent/hermes_runner.go`
- Create: `gormes/internal/subagent/hermes_runner_test.go`
- Modify: `gormes/internal/subagent/runner.go`

- [ ] **Step 1: Write the failing tests**

```go
package subagent

import (
	"context"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
)

type blockingClient struct{}

func (blockingClient) Health(context.Context) error { return nil }
func (blockingClient) OpenRunEvents(context.Context, string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}
func (blockingClient) OpenStream(context.Context, hermes.ChatRequest) (hermes.Stream, error) {
	return blockingStream{}, nil
}

type blockingStream struct{}

func (blockingStream) SessionID() string { return "sess-child-blocked" }
func (blockingStream) Close() error      { return nil }
func (blockingStream) Recv(ctx context.Context) (hermes.Event, error) {
	<-ctx.Done()
	return hermes.Event{}, ctx.Err()
}

func TestHermesRunner_StopCompletesAndStreamsOutput(t *testing.T) {
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "child "},
		{Kind: hermes.EventToken, Token: "done"},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-child-1")

	runner := NewHermesRunner(HermesRunnerConfig{
		Client: mc,
		Model:  "hermes-agent",
	})
	events := make(chan SubagentEvent, 8)

	result := runner.Run(context.Background(), SubagentConfig{
		Goal:    "Summarize the repo state.",
		Context: "Mention only modified files.",
	}, events)
	close(events)

	if result == nil {
		t.Fatal("Run returned nil result")
	}
	if result.Status != StatusCompleted {
		t.Fatalf("Status = %q, want %q", result.Status, StatusCompleted)
	}
	if result.Summary != "child done" {
		t.Fatalf("Summary = %q, want %q", result.Summary, "child done")
	}
	if result.ExitReason != "model_stop" {
		t.Fatalf("ExitReason = %q, want %q", result.ExitReason, "model_stop")
	}
	if result.Iterations != 1 {
		t.Fatalf("Iterations = %d, want 1", result.Iterations)
	}

	got := drain(events)
	if len(got) != 4 {
		t.Fatalf("event count = %d, want 4", len(got))
	}
	if got[0].Type != EventStarted {
		t.Fatalf("first event = %q, want %q", got[0].Type, EventStarted)
	}
	if got[1].Type != EventOutput || got[1].Message != "child " {
		t.Fatalf("second event = %+v, want output chunk", got[1])
	}
	if got[2].Type != EventOutput || got[2].Message != "done" {
		t.Fatalf("third event = %+v, want output chunk", got[2])
	}
	if got[3].Type != EventCompleted || got[3].Message != "child done" {
		t.Fatalf("fourth event = %+v, want completed summary", got[3])
	}
}

func TestHermesRunner_CancelledDuringStreamReturnsInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan SubagentEvent, 4)
	runner := NewHermesRunner(HermesRunnerConfig{
		Client: blockingClient{},
		Model:  "hermes-agent",
	})

	done := make(chan *SubagentResult, 1)
	go func() {
		done <- runner.Run(ctx, SubagentConfig{Goal: "long child"}, events)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case result := <-done:
		if result.Status != StatusInterrupted {
			t.Fatalf("Status = %q, want %q", result.Status, StatusInterrupted)
		}
		if result.ExitReason != "cancelled" {
			t.Fatalf("ExitReason = %q, want %q", result.ExitReason, "cancelled")
		}
		if result.Error != context.Canceled.Error() {
			t.Fatalf("Error = %q, want %q", result.Error, context.Canceled.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HermesRunner did not stop within 2s after ctx cancellation")
	}
}
```

- [ ] **Step 2: Run the targeted tests and verify they fail**

Run:

```bash
cd <repo>/gormes
go test ./internal/subagent -run 'TestHermesRunner_(StopCompletesAndStreamsOutput|CancelledDuringStreamReturnsInterrupted)' -count=1
```

Expected: FAIL with `undefined: NewHermesRunner` and `undefined: HermesRunnerConfig`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/hermes_runner.go
package subagent

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
)

type HermesRunnerConfig struct {
	Client hermes.Client
	Model  string
}

type HermesRunner struct {
	client hermes.Client
	model  string
}

func NewHermesRunner(cfg HermesRunnerConfig) Runner {
	return &HermesRunner{
		client: cfg.Client,
		model:  cfg.Model,
	}
}

func (r *HermesRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	start := time.Now()

	if blocked := blockedToolRequest(cfg.EnabledTools); blocked != "" {
		msg := "blocked tool requested for child run: " + blocked
		emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
		return &SubagentResult{
			Status:     StatusFailed,
			ExitReason: "blocked_tool_request",
			Duration:   time.Since(start),
			Error:      msg,
		}
	}
	if r.client == nil {
		msg := "no Hermes client configured for child run"
		emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
		return &SubagentResult{
			Status:     StatusFailed,
			ExitReason: "missing_client",
			Duration:   time.Since(start),
			Error:      msg,
		}
	}

	emitSubagentEvent(ctx, events, SubagentEvent{Type: EventStarted, Message: cfg.Goal})

	stream, err := r.client.OpenStream(ctx, hermes.ChatRequest{
		Model: pickChildModel(cfg.Model, r.model),
		Stream: true,
		Messages: []hermes.Message{{
			Role:    "user",
			Content: buildChildPrompt(cfg),
		}},
	})
	if err != nil {
		emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: err.Error()})
		return &SubagentResult{
			Status:     StatusFailed,
			ExitReason: "open_stream_failed",
			Duration:   time.Since(start),
			Error:      err.Error(),
		}
	}
	defer stream.Close()

	var draft strings.Builder
	for {
		ev, err := stream.Recv(ctx)
		if err == io.EOF {
			msg := "child stream closed without finish_reason"
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
			return &SubagentResult{
				Status:     StatusFailed,
				ExitReason: "stream_closed_without_finish_reason",
				Duration:   time.Since(start),
				Error:      msg,
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return &SubagentResult{
					Status:     StatusInterrupted,
					ExitReason: classifyContextErr(ctx.Err()),
					Duration:   time.Since(start),
					Error:      ctx.Err().Error(),
				}
			}
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: err.Error()})
			return &SubagentResult{
				Status:     StatusFailed,
				ExitReason: "stream_recv_failed",
				Duration:   time.Since(start),
				Error:      err.Error(),
			}
		}

		switch ev.Kind {
		case hermes.EventToken:
			draft.WriteString(ev.Token)
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventOutput, Message: ev.Token})
		case hermes.EventDone:
			summary := strings.TrimSpace(draft.String())
			if ev.FinishReason != "stop" {
				emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: "tool_calls not implemented yet"})
				return &SubagentResult{
					Status:     StatusFailed,
					ExitReason: "tool_calls_not_implemented",
					Duration:   time.Since(start),
					Iterations: 1,
					Error:      "tool_calls not implemented yet",
				}
			}
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventCompleted, Message: summary})
			return &SubagentResult{
				Status:     StatusCompleted,
				Summary:    summary,
				ExitReason: "model_stop",
				Duration:   time.Since(start),
				Iterations: 1,
			}
		}
	}
}

func buildChildPrompt(cfg SubagentConfig) string {
	if strings.TrimSpace(cfg.Context) == "" {
		return strings.TrimSpace(cfg.Goal)
	}
	return strings.TrimSpace(cfg.Goal) + "\n\nContext:\n" + strings.TrimSpace(cfg.Context)
}

func pickChildModel(override, fallback string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return strings.TrimSpace(fallback)
}

func emitSubagentEvent(ctx context.Context, events chan<- SubagentEvent, ev SubagentEvent) {
	if events == nil {
		return
	}
	select {
	case events <- ev:
	case <-ctx.Done():
	}
}
```

- [ ] **Step 4: Run the targeted tests and verify they pass**

Run:

```bash
cd <repo>/gormes
go test ./internal/subagent -run 'TestHermesRunner_(StopCompletesAndStreamsOutput|CancelledDuringStreamReturnsInterrupted)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd <repo>
git add gormes/internal/subagent/hermes_runner.go gormes/internal/subagent/hermes_runner_test.go
git commit -m "feat(subagent): add hermes-backed child runner core"
```

## Task 2: Tool-calls loop and typed child audit

**Files:**
- Modify: `gormes/internal/subagent/hermes_runner.go`
- Modify: `gormes/internal/subagent/hermes_runner_test.go`
- Modify: `gormes/internal/subagent/runner.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestHermesRunner_ToolCallsRoundTripThroughExecuteChildTool(t *testing.T) {
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{
			Kind:         hermes.EventDone,
			FinishReason: "tool_calls",
			ToolCalls: []hermes.ToolCall{{
				ID:        "call_1",
				Name:      "echo",
				Arguments: json.RawMessage(`{"text":"hello from child"}`),
			}},
		},
	}, "sess-child-tools")
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "tool result applied"},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-child-tools")

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})

	runner := NewHermesRunner(HermesRunnerConfig{
		Client: mc,
		Model:  "hermes-agent",
	})
	events := make(chan SubagentEvent, 16)

	result := runner.Run(context.Background(), SubagentConfig{
		Goal:         "Use echo, then summarize.",
		EnabledTools: []string{"echo"},
		toolExecutor: tools.NewInProcessToolExecutor(reg),
	}, events)
	close(events)

	if result.Status != StatusCompleted {
		t.Fatalf("Status = %q, want %q", result.Status, StatusCompleted)
	}
	if result.ExitReason != "model_stop" {
		t.Fatalf("ExitReason = %q, want %q", result.ExitReason, "model_stop")
	}
	if result.Iterations != 2 {
		t.Fatalf("Iterations = %d, want 2", result.Iterations)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "echo" || result.ToolCalls[0].Status != "completed" {
		t.Fatalf("ToolCalls[0] = %+v, want completed echo call", result.ToolCalls[0])
	}

	reqs := mc.Requests()
	if len(reqs) != 2 {
		t.Fatalf("request count = %d, want 2", len(reqs))
	}
	if len(reqs[1].Messages) != 3 {
		t.Fatalf("second request message count = %d, want 3", len(reqs[1].Messages))
	}
	if reqs[1].Messages[1].Role != "assistant" || len(reqs[1].Messages[1].ToolCalls) != 1 {
		t.Fatalf("assistant tool-call message missing: %+v", reqs[1].Messages[1])
	}
	if reqs[1].Messages[2].Role != "tool" || reqs[1].Messages[2].Name != "echo" {
		t.Fatalf("tool response message missing: %+v", reqs[1].Messages[2])
	}
}

type recvErrClient struct{ err error }

func (recvErrClient) Health(context.Context) error { return nil }
func (recvErrClient) OpenRunEvents(context.Context, string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}
func (c recvErrClient) OpenStream(context.Context, hermes.ChatRequest) (hermes.Stream, error) {
	return recvErrStream{err: c.err}, nil
}

type recvErrStream struct{ err error }

func (recvErrStream) SessionID() string { return "sess-child-error" }
func (recvErrStream) Close() error      { return nil }
func (s recvErrStream) Recv(context.Context) (hermes.Event, error) {
	return hermes.Event{}, s.err
}

func TestHermesRunner_StreamErrorReturnsFailed(t *testing.T) {
	runner := NewHermesRunner(HermesRunnerConfig{
		Client: recvErrClient{err: errors.New("boom")},
		Model:  "hermes-agent",
	})

	result := runner.Run(context.Background(), SubagentConfig{Goal: "error path"}, make(chan SubagentEvent, 4))
	if result.Status != StatusFailed {
		t.Fatalf("Status = %q, want %q", result.Status, StatusFailed)
	}
	if result.ExitReason != "stream_recv_failed" {
		t.Fatalf("ExitReason = %q, want %q", result.ExitReason, "stream_recv_failed")
	}
	if result.Error != "boom" {
		t.Fatalf("Error = %q, want %q", result.Error, "boom")
	}
}
```

- [ ] **Step 2: Run the targeted tests and verify they fail**

Run:

```bash
cd <repo>/gormes
go test ./internal/subagent -run 'TestHermesRunner_(ToolCallsRoundTripThroughExecuteChildTool|StreamErrorReturnsFailed)' -count=1
```

Expected: FAIL because `tool_calls_not_implemented` is still returned and the second scripted request is never sent.

- [ ] **Step 3: Write the minimal implementation**

```go
// Replace the EventDone branch in HermesRunner.Run with a real loop.
messages := []hermes.Message{{
	Role:    "user",
	Content: buildChildPrompt(cfg),
}}
iterations := 0

for {
	iterations++
	if cfg.MaxIterations > 0 && iterations > cfg.MaxIterations {
		msg := "child iteration limit exceeded"
		emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
		return &SubagentResult{
			Status:     StatusFailed,
			ExitReason: "iteration_limit_exceeded",
			Duration:   time.Since(start),
			Iterations: iterations - 1,
			Error:      msg,
		}
	}

	stream, err := r.client.OpenStream(ctx, hermes.ChatRequest{
		Model:    pickChildModel(cfg.Model, r.model),
		Stream:   true,
		Messages: messages,
	})
	if err != nil {
		emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: err.Error()})
		return &SubagentResult{
			Status:     StatusFailed,
			ExitReason: "open_stream_failed",
			Duration:   time.Since(start),
			Iterations: iterations,
			Error:      err.Error(),
		}
	}

	var draft strings.Builder
	var final hermes.Event
	for {
		ev, err := stream.Recv(ctx)
		if err == io.EOF {
			_ = stream.Close()
			msg := "child stream closed without finish_reason"
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
			return &SubagentResult{
				Status:     StatusFailed,
				ExitReason: "stream_closed_without_finish_reason",
				Duration:   time.Since(start),
				Iterations: iterations,
				Error:      msg,
			}
		}
		if err != nil {
			_ = stream.Close()
			if ctx.Err() != nil {
				return &SubagentResult{
					Status:     StatusInterrupted,
					ExitReason: classifyContextErr(ctx.Err()),
					Duration:   time.Since(start),
					Iterations: iterations,
					Error:      ctx.Err().Error(),
				}
			}
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: err.Error()})
			return &SubagentResult{
				Status:     StatusFailed,
				ExitReason: "stream_recv_failed",
				Duration:   time.Since(start),
				Iterations: iterations,
				Error:      err.Error(),
			}
		}

		switch ev.Kind {
		case hermes.EventToken:
			draft.WriteString(ev.Token)
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventOutput, Message: ev.Token})
		case hermes.EventDone:
			final = ev
			goto haveFinal
		}
	}

haveFinal:
	_ = stream.Close()
	summary := strings.TrimSpace(draft.String())
	if final.FinishReason != "tool_calls" {
		emitSubagentEvent(ctx, events, SubagentEvent{Type: EventCompleted, Message: summary})
		return &SubagentResult{
			Status:     StatusCompleted,
			Summary:    summary,
			ExitReason: "model_stop",
			Duration:   time.Since(start),
			Iterations: iterations,
		}
	}

	assistantMsg := hermes.Message{
		Role:      "assistant",
		Content:   summary,
		ToolCalls: final.ToolCalls,
	}
	messages = append(messages, assistantMsg)

	for _, call := range final.ToolCalls {
		out, _, err := executeChildTool(ctx, cfg, events, tools.ToolRequest{
			ToolName: call.Name,
			Input:    call.Arguments,
		})
		if err != nil {
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: err.Error()})
			return &SubagentResult{
				Status:     StatusFailed,
				ExitReason: "tool_call_failed",
				Duration:   time.Since(start),
				Iterations: iterations,
				Error:      err.Error(),
			}
		}
		messages = append(messages, hermes.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    string(out),
		})
	}
}
```

- [ ] **Step 4: Run the targeted tests and verify they pass**

Run:

```bash
cd <repo>/gormes
go test ./internal/subagent -run 'TestHermesRunner_(ToolCallsRoundTripThroughExecuteChildTool|StreamErrorReturnsFailed|StopCompletesAndStreamsOutput|CancelledDuringStreamReturnsInterrupted)' -count=1
go test ./internal/subagent ./internal/hermes ./internal/kernel -count=1 -race
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd <repo>
git add gormes/internal/subagent/hermes_runner.go gormes/internal/subagent/hermes_runner_test.go gormes/internal/subagent/runner.go
git commit -m "feat(subagent): add real child stream loop with tool calls"
```

## Task 3: Wire the production registry to the real runner

**Files:**
- Modify: `gormes/cmd/gormes/registry.go`
- Modify: `gormes/cmd/gormes/registry_test.go`
- Modify: `gormes/cmd/gormes/main.go`
- Modify: `gormes/cmd/gormes/gateway.go`
- Modify: `gormes/cmd/gormes/telegram.go`
- Modify: `gormes/cmd/gormes/doctor.go`

- [ ] **Step 1: Write the failing regression test**

```go
func TestBuildDefaultRegistryDelegationToolExecutesRealChildLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"real child summary\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	reg := buildDefaultRegistry(
		context.Background(),
		config.HermesCfg{Endpoint: srv.URL, Model: "hermes-agent"},
		config.DelegationCfg{
			Enabled:               true,
			MaxDepth:              2,
			MaxConcurrentChildren: 3,
			DefaultMaxIterations:  4,
			DefaultTimeout:        time.Second,
		},
		"",
	)

	tool, ok := reg.Get("delegate_task")
	if !ok {
		t.Fatal("delegate_task not registered")
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"audit runtime"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if bytes.Contains(out, []byte(`"exit_reason":"stub_runner_no_llm_yet"`)) {
		t.Fatalf("output = %s, want real child runner rather than stub", out)
	}
	if !bytes.Contains(out, []byte(`"summary":"real child summary"`)) {
		t.Fatalf("output = %s, want streamed child summary", out)
	}
}
```

- [ ] **Step 2: Run the targeted tests and verify they fail**

Run:

```bash
cd <repo>/gormes
go test ./cmd/gormes -run 'TestBuildDefaultRegistryDelegationToolExecutesRealChildLoop' -count=1
```

Expected: FAIL because `buildDefaultRegistry(...)` still constructs a manager with the default `StubRunner`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/cmd/gormes/registry.go
func buildDefaultRegistry(
	parentCtx context.Context,
	hermesCfg config.HermesCfg,
	delegation config.DelegationCfg,
	skillsRoot string,
) *tools.Registry {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	reg.MustRegister(&tools.NowTool{})
	reg.MustRegister(&tools.RandIntTool{})

	if delegation.Enabled {
		var drafter subagent.CandidateDrafter
		if skillsRoot != "" {
			drafter = skillsCandidateDrafter{store: skills.NewStore(skillsRoot, 0)}
		}

		childClient := hermes.NewHTTPClient(hermesCfg.Endpoint, hermesCfg.APIKey)
		reg.MustRegister(subagent.NewDelegateTool(subagent.NewManager(subagent.ManagerOpts{
			ParentCtx:            parentCtx,
			ParentID:             "root",
			Depth:                0,
			Registry:             subagent.NewRegistry(),
			ToolExecutor:         tools.NewInProcessToolExecutor(reg),
			NewRunner: func() subagent.Runner {
				return subagent.NewHermesRunner(subagent.HermesRunnerConfig{
					Client: childClient,
					Model:  hermesCfg.Model,
				})
			},
			MaxDepth:             delegation.MaxDepth,
			DefaultMaxIterations: delegation.DefaultMaxIterations,
			DefaultMaxConcurrent: delegation.MaxConcurrentChildren,
			DefaultTimeout:       delegation.DefaultTimeout,
			RunLogPath:           delegation.ResolvedRunLogPath(),
		}), drafter))
	}
	return reg
}

// Update every call site:
// buildDefaultRegistry(rootCtx, cfg.Hermes, cfg.Delegation, cfg.SkillsRoot())
```

- [ ] **Step 4: Run the targeted tests and verify they pass**

Run:

```bash
cd <repo>/gormes
go test ./cmd/gormes ./internal/subagent -run 'TestBuildDefaultRegistryDelegationToolExecutesRealChildLoop|TestDelegateTool' -count=1
go test ./cmd/gormes ./internal/subagent ./internal/hermes ./internal/kernel -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd <repo>
git add gormes/cmd/gormes/registry.go gormes/cmd/gormes/registry_test.go gormes/cmd/gormes/main.go gormes/cmd/gormes/gateway.go gormes/cmd/gormes/telegram.go gormes/cmd/gormes/doctor.go
git commit -m "feat(subagent): wire delegate_task to real hermes child runner"
```

## Task 4: Close Queue 1 in the roadmap and generated docs

**Files:**
- Modify: `gormes/docs/content/building-gormes/architecture_plan/progress.json`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/why-go.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/_index.md`
- Modify: `README.md`

- [ ] **Step 1: Update the source-of-truth roadmap files**

```json
{
  "name": "Real child Hermes stream loop",
  "status": "complete",
  "note": "TDD landed: hermes.MockClient, fake-stream, and registry-backed SSE tests now lock stop, tool_calls, error, and cancellation behavior for the real child runner."
}
```

```md
| **Phase 2.E.1 — Delegation Policy + Child Execution** | ✅ complete | **P0** | Runner-enforced blocked-tool/allowlist policy, typed child tool-call audit, and the real child Hermes stream loop are now landed |

| Subagent delegation | `tools/delegate_tool.py` | 2.E | ✅ deterministic runtime, `delegate_task`, runner policy, typed child tool-call audit, append-only run logging, and real child stream execution landed |

**Status:** ✅ Runtime core implemented in `internal/subagent` and exposed through Go-native `delegate_task`. Runner-enforced tool policy, append-only run logging, typed child tool-call audit, and real child LLM execution are landed.
```

- [ ] **Step 2: Regenerate progress-driven markdown**

Run:

```bash
cd <repo>/gormes
go run ./cmd/progress-gen --write
```

Expected: `_index.md` and `README.md` regenerated.

- [ ] **Step 3: Run the verification gate**

Run:

```bash
cd <repo>/gormes
go test ./internal/subagent ./internal/hermes ./internal/kernel ./cmd/gormes ./internal/progress ./docs -count=1
go test ./internal/subagent ./internal/hermes ./internal/kernel -count=1 -race
go run ./cmd/progress-gen --validate
```

Expected: PASS on all commands.

- [ ] **Step 4: Commit**

```bash
cd <repo>
git add gormes/docs/content/building-gormes/architecture_plan/progress.json gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md gormes/docs/content/building-gormes/architecture_plan/why-go.md gormes/docs/content/building-gormes/architecture_plan/_index.md README.md
git commit -m "docs(phase2): mark real child stream loop complete"
```

## Final queue gate

Run before claiming Queue 1 complete:

```bash
cd <repo>/gormes
go test ./internal/subagent ./internal/hermes ./internal/kernel ./cmd/gormes ./internal/progress ./docs -count=1
go test ./internal/subagent ./internal/hermes ./internal/kernel -count=1 -race
go run ./cmd/progress-gen --validate
```

If any command is red, fix that regression before starting Queue 2.
