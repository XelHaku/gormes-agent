package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/learning"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

func TestKernel_RecordsLearningSignalAfterSuccessfulToolTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learning", "complexity.jsonl")
	recorder := learning.NewRuntime(path, learning.Config{})

	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{{
		Kind:         hermes.EventDone,
		FinishReason: "tool_calls",
		ToolCalls: []hermes.ToolCall{
			{
				ID:        "call_echo_1",
				Name:      "echo",
				Arguments: json.RawMessage(`{"text":"first trace"}`),
			},
			{
				ID:        "call_echo_2",
				Name:      "echo",
				Arguments: json.RawMessage(`{"text":"second trace"}`),
			},
		},
	}}, "sess-learning")

	finalAnswer := "Traced the failure, used both tool calls, and confirmed the fix."
	events := make([]hermes.Event, 0, len(finalAnswer)+1)
	for _, ch := range finalAnswer {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{
		Kind:         hermes.EventDone,
		FinishReason: "stop",
		TokensIn:     180,
		TokensOut:    len(finalAnswer),
	})
	mc.Script(events, "sess-learning")

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})

	k := New(Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
		Learning:          recorder,
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go k.Run(ctx)

	<-k.Render()
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "trace the failing restart path and fix it"}); err != nil {
		t.Fatal(err)
	}

	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		if f.Phase != PhaseIdle {
			return false
		}
		a := lastAssistantMessage(f.History)
		return a != nil && a.Content == finalAnswer
	}, 5*time.Second)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(lines))
	}

	var signal learning.Signal
	if err := json.Unmarshal(lines[0], &signal); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if !signal.WorthLearning {
		t.Fatal("WorthLearning = false, want true")
	}
	if signal.SessionID != "sess-learning" {
		t.Fatalf("SessionID = %q, want %q", signal.SessionID, "sess-learning")
	}
	if signal.Metrics.ToolCallCount != 2 {
		t.Fatalf("ToolCallCount = %d, want 2", signal.Metrics.ToolCallCount)
	}
	if !containsLearningReason(signal.Reasons, "tool_calls") {
		t.Fatalf("Reasons = %#v, want tool_calls", signal.Reasons)
	}
	if !containsLearningReason(signal.Reasons, "multi_tool_calls") {
		t.Fatalf("Reasons = %#v, want multi_tool_calls", signal.Reasons)
	}
}

func containsLearningReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
