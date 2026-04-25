package kernel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/internal/subagent"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
)

func TestKernel_CancelDuringToolCallsCancelsSidecarAndDelegatedChild(t *testing.T) {
	started := make(chan string, 2)
	cancelled := make(chan string, 2)

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.ExecuteCodeTool{
		Sandbox:        blockingKernelSandbox{started: started, cancelled: cancelled},
		DefaultTimeout: 5 * time.Second,
	})

	mgr := subagent.NewManager(subagent.ManagerOpts{
		ParentCtx: context.Background(),
		NewRunner: func() subagent.Runner {
			return blockingKernelRunner{started: started, cancelled: cancelled}
		},
	})
	defer mgr.Close()
	reg.MustRegister(subagent.NewDelegateTool(mgr, nil))

	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{{
		Kind:         hermes.EventDone,
		FinishReason: "tool_calls",
		ToolCalls: []hermes.ToolCall{
			{ID: "call_exec", Name: "execute_code", Arguments: json.RawMessage(`{"language":"sh","code":"sleep 5"}`)},
			{ID: "call_delegate", Name: "delegate_task", Arguments: json.RawMessage(`{"goal":"wait for cancel"}`)},
		},
	}}, "sess-tools")

	k := New(Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go k.Run(ctx)

	initial := <-k.Render()
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "run cancellable workers"}); err != nil {
		t.Fatal(err)
	}

	requireNames(t, started, []string{"execute_code", "delegate_task"}, time.Second)

	if err := k.Submit(PlatformEvent{Kind: PlatformEventCancel}); err != nil {
		t.Fatal(err)
	}
	requireNames(t, cancelled, []string{"execute_code", "delegate_task"}, time.Second)

	_, final := drainUntilIdle(t, k.Render(), initial.Seq, time.Second)
	if len(mc.Requests()) != 1 {
		t.Fatalf("OpenStream request count = %d, want 1; cancelled tool batch must not continue to another model call", len(mc.Requests()))
	}
	if len(final.History) != 1 || final.History[0].Role != "user" {
		t.Fatalf("history after cancelled tool batch = %#v, want only the original user turn", final.History)
	}
}

type blockingKernelSandbox struct {
	started   chan<- string
	cancelled chan<- string
}

func (s blockingKernelSandbox) Execute(ctx context.Context, req tools.CodeExecutionRequest) (tools.CodeExecutionResult, error) {
	s.started <- "execute_code"
	<-ctx.Done()
	s.cancelled <- "execute_code"
	return tools.CodeExecutionResult{
		Status:     "interrupted",
		Language:   req.Language,
		ExitCode:   130,
		Error:      ctx.Err().Error(),
		DurationMs: 1,
	}, nil
}

type blockingKernelRunner struct {
	started   chan<- string
	cancelled chan<- string
}

func (r blockingKernelRunner) Run(ctx context.Context, cfg subagent.SubagentConfig, events chan<- subagent.SubagentEvent) *subagent.SubagentResult {
	r.started <- "delegate_task"
	events <- subagent.SubagentEvent{Type: subagent.EventStarted, Message: cfg.Goal}
	<-ctx.Done()
	r.cancelled <- "delegate_task"
	return &subagent.SubagentResult{
		Status:     subagent.StatusInterrupted,
		ExitReason: "cancelled",
		Error:      ctx.Err().Error(),
	}
}
