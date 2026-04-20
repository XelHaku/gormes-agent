package kernel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

// TestKernel_ChatKeyPropagatesToStorePayload proves that setting
// kernel.Config.ChatKey makes every outbound store.Command payload
// contain {"chat_id": "<that key>"} so Phase-3.C's per-chat scoping
// has data to filter against.
func TestKernel_ChatKeyPropagatesToStorePayload(t *testing.T) {
	rec := store.NewRecording()
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-chat-key-test")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
		ChatKey:   "telegram:12345",
	}, mc, rec, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	_ = k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"})

	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID != ""
	}, 2*time.Second)

	cmds := rec.Commands()
	if len(cmds) == 0 {
		t.Fatal("no commands captured")
	}
	var p struct {
		ChatID string `json:"chat_id"`
	}
	if err := json.Unmarshal(cmds[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.ChatID != "telegram:12345" {
		t.Errorf("chat_id in payload = %q, want telegram:12345", p.ChatID)
	}
}
