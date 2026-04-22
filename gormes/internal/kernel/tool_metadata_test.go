package kernel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

func TestKernel_FinalizeAssistantTurnCarriesToolCallMetadata(t *testing.T) {
	rec := store.NewRecording()
	mc := hermes.NewMockClient()

	mc.Script([]hermes.Event{{
		Kind:         hermes.EventDone,
		FinishReason: "tool_calls",
		ToolCalls: []hermes.ToolCall{{
			ID:        "call_echo_1",
			Name:      "echo",
			Arguments: json.RawMessage(`{"text":"tool payload"}`),
		}},
	}}, "sess-tools-meta")

	reply := "Echo complete."
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"})
	mc.Script(events, "sess-tools-meta")

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})

	k := New(Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, mc, rec, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "echo tool payload"}); err != nil {
		t.Fatal(err)
	}

	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-tools-meta"
	}, 5*time.Second)

	var finalizePayload map[string]any
	for _, cmd := range rec.Commands() {
		if cmd.Kind != store.FinalizeAssistantTurn {
			continue
		}
		if err := json.Unmarshal(cmd.Payload, &finalizePayload); err != nil {
			t.Fatalf("FinalizeAssistantTurn payload: %v", err)
		}
		break
	}
	if finalizePayload == nil {
		t.Fatal("missing FinalizeAssistantTurn payload")
	}

	rawMeta, ok := finalizePayload["meta_json"].(string)
	if !ok || rawMeta == "" {
		t.Fatalf("meta_json = %v, want populated JSON string", finalizePayload["meta_json"])
	}

	var meta struct {
		ToolCalls []struct {
			Name string `json:"name"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(rawMeta), &meta); err != nil {
		t.Fatalf("Unmarshal(meta_json): %v", err)
	}
	if len(meta.ToolCalls) != 1 || meta.ToolCalls[0].Name != "echo" {
		t.Fatalf("tool_calls = %+v, want single echo call", meta.ToolCalls)
	}
}
